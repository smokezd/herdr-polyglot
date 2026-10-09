package transcript

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is a session an agent writes as one JSON record per line.
type File struct {
	name  string
	path  string
	turns func(records io.Reader) ([]string, error)
}

func (f File) Name() string { return f.name }

func (File) Whole() bool { return false }

func (f File) Latest(_ context.Context, n int) ([]Reply, error) {
	file, err := os.Open(f.path)
	if err != nil {
		return nil, fmt.Errorf("reading the %s session: %w", f.name, err)
	}
	defer file.Close()

	turns, err := f.turns(file)
	if err != nil {
		return nil, fmt.Errorf("reading the %s session: %w", f.name, err)
	}
	if len(turns) > n {
		turns = turns[len(turns)-n:]
	}
	replies := make([]Reply, len(turns))
	for i, text := range turns {
		replies[i] = Reply{Text: text}
	}
	return replies, nil
}

// maxRecord is the longest line a session file may hold. A tool result can be
// a whole file, so the scanner's own limit of 64 KiB is not enough.
const maxRecord = 64 << 20

// eachRecord calls visit with every line that is a JSON object, skipping any
// it cannot read: an agent still writing its last line is not an error.
func eachRecord(records io.Reader, visit func(line []byte)) error {
	lines := bufio.NewScanner(records)
	lines.Buffer(make([]byte, 0, 64<<10), maxRecord)
	for lines.Scan() {
		if line := lines.Bytes(); len(line) > 0 && line[0] == '{' {
			visit(line)
		}
	}
	return lines.Err()
}

// turnsOf collects the replies of a session as it is read: a prompt starts a
// new turn and everything the agent says until the next one belongs to it.
type turnsOf struct {
	replies []string
	current []string
	started bool
}

func (t *turnsOf) prompt() {
	t.close()
	t.started = true
}

func (t *turnsOf) say(text string) {
	if text = strings.TrimSpace(text); text != "" && t.started {
		t.current = append(t.current, text)
	}
}

func (t *turnsOf) close() {
	if len(t.current) > 0 {
		t.replies = append(t.replies, strings.Join(t.current, "\n\n"))
	}
	t.current = nil
}

func (t *turnsOf) done() []string {
	t.close()
	return t.replies
}

// claudeTurns reads a Claude Code session. A prompt is a user message with
// text in it; a user message holding only tool results is the agent's own
// work going on. What the agent says is the text of its messages, not its
// thinking or its tool calls, and not what a subagent said on the side.
func claudeTurns(records io.Reader) ([]string, error) {
	var turns turnsOf
	err := eachRecord(records, func(line []byte) {
		var record struct {
			Type      string `json:"type"`
			Sidechain bool   `json:"isSidechain"`
			Meta      bool   `json:"isMeta"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &record) != nil || record.Sidechain || record.Meta {
			return
		}

		var said string
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(record.Message.Content, &said) != nil {
			if json.Unmarshal(record.Message.Content, &blocks) != nil {
				return
			}
		}

		switch record.Type {
		case "user":
			if said != "" {
				turns.prompt()
				return
			}
			for _, block := range blocks {
				if block.Type == "text" {
					turns.prompt()
					return
				}
			}
		case "assistant":
			turns.say(said)
			for _, block := range blocks {
				if block.Type == "text" {
					turns.say(block.Text)
				}
			}
		}
	})
	return turns.done(), err
}

// codexTurns reads a Codex session, which records each prompt and each thing
// the agent says as an event of its own.
func codexTurns(records io.Reader) ([]string, error) {
	var turns turnsOf
	err := eachRecord(records, func(line []byte) {
		var record struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &record) != nil || record.Type != "event_msg" {
			return
		}
		switch record.Payload.Type {
		case "user_message":
			turns.prompt()
		case "agent_message":
			turns.say(record.Payload.Message)
		}
	})
	return turns.done(), err
}

// claudeSession finds the session by its id alone. The folder it is in is
// named after the working directory, but how Claude Code spells that name is
// its own business.
func claudeSession(home, session string) string {
	if home == "" || !safeID(session) {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(home, "projects", "*", session+".jsonl"))
	return newest(matches)
}

// codexSession finds the rollout named after the session, or else the latest
// one started in the pane's directory.
func codexSession(home, session, cwd string) string {
	if home == "" {
		return ""
	}
	var rollouts []string
	_ = filepath.WalkDir(filepath.Join(home, "sessions"), func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasPrefix(entry.Name(), "rollout-") &&
			strings.HasSuffix(entry.Name(), ".jsonl") {
			rollouts = append(rollouts, path)
		}
		return nil
	})

	if safeID(session) {
		var named []string
		for _, path := range rollouts {
			if strings.HasSuffix(filepath.Base(path), "-"+session+".jsonl") {
				named = append(named, path)
			}
		}
		if path := newest(named); path != "" {
			return path
		}
	}

	if cwd == "" {
		return ""
	}
	sort.Slice(rollouts, func(i, j int) bool { return modified(rollouts[i]) > modified(rollouts[j]) })
	for _, path := range rollouts {
		if codexStartedIn(path) == cwd {
			return path
		}
	}
	return ""
}

// codexStartedIn reads the working directory from the first record of a
// rollout, which describes the session.
func codexStartedIn(path string) string {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return ""
	}
	defer file.Close()

	first, err := bufio.NewReaderSize(file, 64<<10).ReadBytes('\n')
	if err != nil && len(first) == 0 {
		return ""
	}
	var record struct {
		Type    string `json:"type"`
		Payload struct {
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(first, &record) != nil || record.Type != "session_meta" {
		return ""
	}
	return record.Payload.Cwd
}

// safeID keeps a session id from reaching outside the directory it names a
// file in.
func safeID(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\*?[`) && id != "." && id != ".."
}

func newest(paths []string) string {
	best, bestTime := "", int64(-1)
	for _, path := range paths {
		if at := modified(path); at > bestTime {
			best, bestTime = path, at
		}
	}
	return best
}

func modified(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.ModTime().UnixNano()
}
