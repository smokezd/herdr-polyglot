package transcript_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/transcript"
)

// fakeHerdr answers `agent get` with the agent it was given and `pane read`
// with the terminal it was given.
type fakeHerdr struct {
	agent    string
	terminal string
	calls    [][]string
}

func (f *fakeHerdr) Output(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch {
	case args[0] == "agent" && args[1] == "get":
		if f.agent == "" {
			return nil, errors.New("pane is not an agent")
		}
		return []byte(`{"result":{"agent":` + f.agent + `}}`), nil
	case args[0] == "pane" && args[1] == "read":
		return []byte(f.terminal), nil
	}
	return nil, errors.New("unexpected call")
}

func writeFile(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func texts(t *testing.T, source transcript.Source, n int) []string {
	t.Helper()
	replies, err := source.Latest(context.Background(), n)
	if err != nil {
		t.Fatalf("Latest returned unexpected error: %v", err)
	}
	got := make([]string, len(replies))
	for i, reply := range replies {
		got[i] = reply.Text
	}
	return got
}

var claudeSession = []string{
	`{"type":"permission-mode","permissionMode":"default"}`,
	`{"type":"user","message":{"role":"user","content":"erste Frage"}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"}]}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Let me look."}]}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read"}]}}`,
	`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","content":"file body"}]}}`,
	`{"type":"assistant","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"a subagent aside"}]}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The first answer."}]}}`,
	`{"type":"attachment","attachment":{}}`,
	`{"type":"user","isMeta":true,"message":{"role":"user","content":"a reminder"}}`,
	`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"zweite Frage"}]}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`,
	`{"type":"user","message":{"role":"user","content":"dritte Frage"}}`,
	`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"The third answer."}]}}`,
	`{"type":"assistant","message":{"role":"assist`,
}

func claudePane(t *testing.T) (*fakeHerdr, transcript.Places) {
	t.Helper()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "projects", "-some-project", "abc-123.jsonl"), claudeSession...)
	herdr := &fakeHerdr{agent: `{"agent":"claude","cwd":"/some/project","agent_session":{"value":"abc-123"}}`}
	return herdr, transcript.Places{Claude: home}
}

func TestAClaudeReplyIsWhatTheAgentSaidBetweenTwoPrompts(t *testing.T) {
	t.Parallel()
	herdr, places := claudePane(t)

	source := transcript.Resolve(context.Background(), herdr, "w1:p1", places)

	if source.Name() != "Claude Code" || source.Whole() {
		t.Fatalf("source is %q (whole: %v), want the Claude Code session", source.Name(), source.Whole())
	}
	got := texts(t, source, 5)
	want := []string{"Let me look.\n\nThe first answer.", "The third answer."}
	if !slices.Equal(got, want) {
		t.Errorf("replies are %q, want %q", got, want)
	}
}

func TestOnlyTheLatestRepliesAreHandedOut(t *testing.T) {
	t.Parallel()
	herdr, places := claudePane(t)

	got := texts(t, transcript.Resolve(context.Background(), herdr, "w1:p1", places), 1)

	if !slices.Equal(got, []string{"The third answer."}) {
		t.Errorf("replies are %q, want the last one only", got)
	}
}

func TestACodexReplyComesFromTheAgentMessagesOfOneTurn(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "sessions", "2026", "06", "26", "rollout-2026-06-26T14-37-27-019f.jsonl"),
		`{"type":"session_meta","payload":{"id":"019f","cwd":"/work"}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"erste Frage"}}`,
		`{"type":"response_item","payload":{"type":"function_call"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"Looking."}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"Done."}}`,
		`{"type":"event_msg","payload":{"type":"user_message","message":"zweite Frage"}}`,
		`{"type":"event_msg","payload":{"type":"agent_message","message":"Second."}}`,
	)

	for name, agent := range map[string]string{
		"by its id":        `{"agent":"codex","cwd":"/elsewhere","agent_session":{"value":"019f"}}`,
		"by its directory": `{"agent":"codex","cwd":"/work","agent_session":{"value":""}}`,
	} {
		source := transcript.Resolve(context.Background(), &fakeHerdr{agent: agent}, "w1:p1",
			transcript.Places{Codex: home})
		if source.Name() != "Codex" {
			t.Fatalf("found %s: source is %q, want the Codex session", name, source.Name())
		}
		got := texts(t, source, 2)
		if want := []string{"Looking.\n\nDone.", "Second."}; !slices.Equal(got, want) {
			t.Errorf("found %s: replies are %q, want %q", name, got, want)
		}
	}
}

func TestAPaneWithoutATranscriptIsReadOffTheTerminal(t *testing.T) {
	t.Parallel()

	for name, agent := range map[string]string{
		"no agent":          "",
		"another agent":     `{"agent":"opencode","cwd":"/work","agent_session":{"value":"x"}}`,
		"a missing session": `{"agent":"claude","cwd":"/work","agent_session":{"value":"gone"}}`,
		"a path for an id":  `{"agent":"claude","cwd":"/work","agent_session":{"value":"../../secret"}}`,
	} {
		herdr := &fakeHerdr{agent: agent, terminal: "  $ make\n  ok\n"}
		source := transcript.Resolve(context.Background(), herdr, "w1:p1",
			transcript.Places{Claude: t.TempDir(), Codex: t.TempDir()})

		if source.Name() != "terminal" || !source.Whole() {
			t.Errorf("%s: source is %q, want the terminal", name, source.Name())
			continue
		}
		if got := texts(t, source, 1); !slices.Equal(got, []string{"$ make\n  ok"}) {
			t.Errorf("%s: replies are %q, want the scrollback as one", name, got)
		}
		read := herdr.calls[len(herdr.calls)-1]
		if !slices.Equal(read[:3], []string{"pane", "read", "w1:p1"}) {
			t.Errorf("%s: read the terminal with %q", name, read)
		}
	}
}

func TestAQuestionOrAPlanPutToTheReaderIsPartOfTheReply(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "projects", "-p", "q-1.jsonl"),
		`{"type":"user","message":{"role":"user","content":"make the popup resizable"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"One thing decides it:"}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"AskUserQuestion","input":{"questions":[{"question":"Which popup?","header":"Popup","options":[{"label":"Both","description":"Each its own size."},{"label":"Only the reader"}]}]}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"ExitPlanMode","input":{"plan":"# Plan\n\nDo it."}}]}}`,
	)
	herdr := &fakeHerdr{agent: `{"agent":"claude","cwd":"/p","agent_session":{"value":"q-1"}}`}

	got := texts(t, transcript.Resolve(context.Background(), herdr, "w1:p1", transcript.Places{Claude: home}), 1)

	want := "One thing decides it:\n\n**Popup:** Which popup?\n1. Both — Each its own size.\n2. Only the reader\n\n# Plan\n\nDo it."
	if len(got) != 1 || got[0] != want {
		t.Errorf("reply is %q, want %q", got, want)
	}
}

func TestEveryQuestionOfAGroupIsInTheReplyNotOnlyTheTabOnScreen(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeFile(t, filepath.Join(home, "projects", "-p", "q-2.jsonl"),
		`{"type":"user","message":{"role":"user","content":"plan it"}}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"AskUserQuestion","input":{"questions":[`+
			`{"question":"How many?","header":"Counts","options":[{"label":"One"},{"label":"Two"}]},`+
			`{"question":"From where?","header":"Source","options":[{"label":"Files"},{"label":"Screen"}]},`+
			`{"question":"Which language?","header":"Language","options":[{"label":"German"}]}]}}]}}`,
	)
	herdr := &fakeHerdr{agent: `{"agent":"claude","cwd":"/p","agent_session":{"value":"q-2"}}`}

	got := texts(t, transcript.Resolve(context.Background(), herdr, "w1:p1", transcript.Places{Claude: home}), 1)

	want := "**Counts:** How many?\n1. One\n2. Two\n\n" +
		"**Source:** From where?\n1. Files\n2. Screen\n\n" +
		"**Language:** Which language?\n1. German"
	if len(got) != 1 || got[0] != want {
		t.Errorf("reply is %q, want every question in turn", got)
	}
}
