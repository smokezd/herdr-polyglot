package transcript

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
	// waiting is a prompt the agent is still working on and has said nothing
	// to yet. It is handed out as an empty reply, so the one before it is not
	// taken for the answer.
	waiting bool
}

// prompt starts a turn. awaited says an answer is sure to follow, which is not
// so for a command Claude Code runs itself.
func (t *turnsOf) prompt(awaited bool) {
	t.close()
	t.started = true
	t.waiting = awaited
}

func (t *turnsOf) say(text string) {
	if text = strings.TrimSpace(text); text != "" && t.started {
		t.current = append(t.current, text)
		t.waiting = false
	}
}

// end is the agent done with the turn, whether or not it said anything.
func (t *turnsOf) end() { t.waiting = false }

func (t *turnsOf) close() {
	if len(t.current) > 0 {
		t.replies = append(t.replies, strings.Join(t.current, "\n\n"))
	}
	t.current = nil
}

func (t *turnsOf) done() []string {
	t.close()
	if t.waiting {
		t.replies = append(t.replies, "")
	}
	return t.replies
}

// claudeTurns reads a Claude Code session. A prompt is a user message with
// text in it; a user message holding only tool results is the agent's own
// work going on. What the agent says is the text of its messages, not its
// thinking or its tool calls, and not what a subagent said on the side, nor an
// error Claude Code wrote in its place. The summary Claude Code writes as the
// user's when it compacts the session is no prompt either. A turn is over once
// Claude Code says how long it took.
func claudeTurns(records io.Reader) ([]string, error) {
	var turns turnsOf
	err := eachRecord(records, func(line []byte) {
		var record struct {
			Type      string `json:"type"`
			Sidechain bool   `json:"isSidechain"`
			Meta      bool   `json:"isMeta"`
			Subtype   string `json:"subtype"`
			APIError  bool   `json:"isApiErrorMessage"`
			Summary   bool   `json:"isCompactSummary"`
			Message   struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &record) != nil || record.Sidechain || record.Meta || record.APIError || record.Summary {
			return
		}
		if record.Type == "system" && record.Subtype == "turn_duration" {
			turns.end()
			return
		}

		var said string
		var blocks []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(record.Message.Content, &said) != nil {
			if json.Unmarshal(record.Message.Content, &blocks) != nil {
				return
			}
		}

		switch record.Type {
		case "user":
			if said != "" {
				turns.prompt(awaited(said))
				return
			}
			for _, block := range blocks {
				if block.Type == "text" {
					turns.prompt(awaited(block.Text))
					return
				}
			}
		case "assistant":
			turns.say(said)
			for _, block := range blocks {
				switch block.Type {
				case "text":
					turns.say(block.Text)
				case "tool_use":
					turns.say(addressedToTheReader(block.Name, block.Input))
				}
			}
		}
	})
	return turns.done(), err
}

// awaited says whether a user message is one the agent answers. A slash
// command, what it printed and a note that the agent was stopped are recorded
// as the user's too, and some of them are never answered.
func awaited(prompt string) bool {
	for _, own := range []string{"<command-name>", "<command-message>", "<local-command-", "[Request interrupted"} {
		if strings.HasPrefix(strings.TrimSpace(prompt), own) {
			return false
		}
	}
	return true
}

// addressedToTheReader is what a tool call says to the person at the
// terminal rather than to the machine: the questions Claude Code asks, with
// their options numbered as the terminal numbers them, and the plan it asks to
// have approved. Every other tool call is the agent's own work.
func addressedToTheReader(tool string, input json.RawMessage) string {
	switch tool {
	case "AskUserQuestion":
		var asked struct {
			Questions []struct {
				Question string `json:"question"`
				Header   string `json:"header"`
				Options  []struct {
					Label       string `json:"label"`
					Description string `json:"description"`
				} `json:"options"`
			} `json:"questions"`
		}
		if json.Unmarshal(input, &asked) != nil {
			return ""
		}
		var questions []string
		for _, question := range asked.Questions {
			lines := []string{question.Question}
			if question.Header != "" {
				lines[0] = "**" + question.Header + ":** " + question.Question
			}
			for index, option := range question.Options {
				line := strconv.Itoa(index+1) + ". " + option.Label
				if option.Description != "" {
					line += " — " + option.Description
				}
				lines = append(lines, line)
			}
			questions = append(questions, strings.Join(lines, "\n"))
		}
		return strings.Join(questions, "\n\n")

	case "ExitPlanMode":
		var plan struct {
			Plan string `json:"plan"`
		}
		if json.Unmarshal(input, &plan) != nil {
			return ""
		}
		return plan.Plan
	}
	return ""
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
			// Not read for when a turn ends, so no turn is waited on.
			turns.prompt(false)
		case "agent_message":
			turns.say(record.Payload.Message)
		}
	})
	return turns.done(), err
}

// claudeSession finds the session by its id alone. The folder it is in is
// named after the working directory, but how Claude Code spells that name is
// its own business. A session Claude Code went on with under another id ends
// by naming it, and herdr may not have noticed, so the reading goes where the
// session went.
func claudeSession(home, session string) string {
	if home == "" {
		return ""
	}
	path := ""
	for seen := map[string]bool{}; safeID(session) && !seen[session]; {
		seen[session] = true
		matches, _ := filepath.Glob(filepath.Join(home, "projects", "*", session+".jsonl"))
		found := newest(matches)
		if found == "" {
			break
		}
		path, session = found, continuedIn(found)
	}
	return path
}

// continuedIn is the id of the session a Claude Code session went on in, if it
// did.
func continuedIn(path string) string {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return ""
	}
	defer file.Close()

	next := ""
	_ = eachRecord(file, func(line []byte) {
		if !bytes.Contains(line, []byte(`"continued-in"`)) {
			return
		}
		var record struct {
			Type string `json:"type"`
			Next string `json:"continuedInSessionId"`
		}
		if json.Unmarshal(line, &record) == nil && record.Type == "continued-in" {
			next = record.Next
		}
	})
	return next
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
