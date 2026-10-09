// Package transcript finds what the agent in a pane has replied. Claude Code
// and Codex keep each session in a file of their own, so their replies come
// apart cleanly, one per prompt; any other agent is read off the terminal.
package transcript

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Reply is what the agent answered to one prompt, without its tool calls or
// its reasoning.
type Reply struct{ Text string }

// Source hands out the agent's latest replies.
type Source interface {
	// Name says where the replies come from, for the header.
	Name() string
	// Latest returns up to n of the most recent replies, oldest first.
	Latest(ctx context.Context, n int) ([]Reply, error)
	// Whole is true when the source cannot tell one reply from the next and
	// returns everything as a single one, so a count of replies means nothing.
	Whole() bool
}

// Runner runs the herdr CLI and hands back what it printed.
type Runner interface {
	Output(ctx context.Context, args ...string) ([]byte, error)
}

// Places are the directories the agents keep their sessions in.
type Places struct {
	Claude string // ~/.claude, or CLAUDE_CONFIG_DIR
	Codex  string // ~/.codex, or CODEX_HOME
}

// scrollbackLines is how much of the terminal stands in for a reply when there
// is no transcript: enough for a long answer, little enough to translate.
const scrollbackLines = 200

// Resolve asks herdr which agent runs in the pane and which session it is in,
// and returns the transcript of that session. When there is none to be found
// the terminal is read instead, so every pane has something to translate.
func Resolve(ctx context.Context, runner Runner, pane string, places Places) Source {
	terminal := Scrollback{runner: runner, pane: pane}

	agent, err := agentIn(ctx, runner, pane)
	if err != nil {
		return terminal
	}

	switch agent.Agent {
	case "claude":
		if path := claudeSession(places.Claude, agent.Session.Value); path != "" {
			return File{name: "Claude Code", path: path, turns: claudeTurns}
		}
	case "codex":
		if path := codexSession(places.Codex, agent.Session.Value, agent.Cwd); path != "" {
			return File{name: "Codex", path: path, turns: codexTurns}
		}
	}
	return terminal
}

type agentInfo struct {
	Agent   string `json:"agent"`
	Cwd     string `json:"cwd"`
	Session struct {
		Value string `json:"value"`
	} `json:"agent_session"`
}

func agentIn(ctx context.Context, runner Runner, pane string) (agentInfo, error) {
	out, err := runner.Output(ctx, "agent", "get", pane)
	if err != nil {
		return agentInfo{}, err
	}
	var reply struct {
		Result struct {
			Agent agentInfo `json:"agent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &reply); err != nil {
		return agentInfo{}, fmt.Errorf("reading what herdr says about pane %s: %w", pane, err)
	}
	return reply.Result.Agent, nil
}

// Scrollback is the recent output of the terminal, as one reply.
type Scrollback struct {
	runner Runner
	pane   string
}

func (Scrollback) Name() string { return "terminal" }

func (Scrollback) Whole() bool { return true }

func (s Scrollback) Latest(ctx context.Context, _ int) ([]Reply, error) {
	out, err := s.runner.Output(ctx, "pane", "read", s.pane,
		"--source", "recent-unwrapped", "--lines", strconv.Itoa(scrollbackLines))
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return nil, nil
	}
	return []Reply{{Text: text}}, nil
}
