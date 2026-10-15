package overlay_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/wazum/herdr-polyglot/internal/overlay"
	"github.com/wazum/herdr-polyglot/internal/promptflow"
)

// twoHosts is a laptop that is up with nothing named to load, and a GPU box
// that is down with a model of its own.
type twoHosts struct{ current string }

func (twoHosts) List() []overlay.Host {
	return []overlay.Host{
		{Name: "laptop", Address: "http://localhost:1234"},
		{Name: "gpu", Address: "http://192.168.1.20:1234", Model: "qwen3-32b"},
	}
}

func (h twoHosts) Current() string { return h.current }

func (twoHosts) Probe(_ context.Context, name string) ([]string, error) {
	if name == "gpu" {
		return nil, errors.New("connection refused")
	}
	return []string{"gemma-3"}, nil
}

// choices remembers which hosts were chosen, from the program's goroutine.
type choices struct {
	mu   sync.Mutex
	made []string
}

func (c *choices) add(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.made = append(c.made, name)
}

func (c *choices) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.made...)
}

func pickerOverlay(t *testing.T, chosen *choices, live bool) *teatest.TestModel {
	t.Helper()
	target := &recordingTarget{}
	return newOverlayWith(t, stubTranslator{english: english}, target, overlay.Options{
		Service:  "lmstudio · laptop",
		Language: "EN-US",
		Live:     live,
		Debounce: time.Millisecond,
		Hosts:    twoHosts{current: "laptop"},
		ChooseHost: func(name string) (overlay.Prompter, string, error) {
			chosen.add(name)
			return promptflow.New(stubTranslator{english: "Translated on " + name}, target, target),
				"lmstudio · " + name, nil
		},
	})
}

func TestCtrlOListsTheHostsAndWhetherEachIsUp(t *testing.T) {
	t.Parallel()

	popup := pickerOverlay(t, &choices{}, false)
	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlO})
	waitForText(t, popup, "● laptop", "gemma-3 (loaded)", "gpu", "✗ unreachable", "enter")

	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	popup.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestChoosingAHostTranslatesWithItFromThenOn(t *testing.T) {
	t.Parallel()
	chosen := &choices{}

	popup := pickerOverlay(t, chosen, true)
	popup.Type("hallo")
	waitForText(t, popup, english)

	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlO})
	popup.Send(tea.KeyMsg{Type: tea.KeyDown})
	popup.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForText(t, popup, "lmstudio · gpu", "Translated on gpu")

	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	popup.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	if got := chosen.all(); len(got) != 1 || got[0] != "gpu" {
		t.Errorf("chose %v, want gpu, down as it is", got)
	}
}

func TestEscapeLeavesThePickerWithoutChoosing(t *testing.T) {
	t.Parallel()
	chosen := &choices{}

	popup := pickerOverlay(t, chosen, false)
	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlO})
	waitForText(t, popup, "✗ unreachable")
	popup.Send(tea.KeyMsg{Type: tea.KeyDown})
	popup.Send(tea.KeyMsg{Type: tea.KeyEsc})
	popup.Type("hallo")
	waitForText(t, popup, "hallo")

	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	popup.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	if got := chosen.all(); len(got) != 0 {
		t.Errorf("chose %v, want nothing", got)
	}
}

func TestCtrlOWithOneHostSaysHowToAddMore(t *testing.T) {
	t.Parallel()

	popup := newOverlayWith(t, stubTranslator{english: english}, &recordingTarget{}, overlay.Options{
		Service: "lmstudio", Language: "EN-US", Hosts: oneHost{},
		ChooseHost: func(string) (overlay.Prompter, string, error) {
			t.Error("a host was chosen with only one to choose")
			return nil, "", nil
		},
	})
	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlO})
	waitForText(t, popup, "HERDR_POLYGLOT_LMSTUDIO_HOSTS")

	popup.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	popup.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

type oneHost struct{}

func (oneHost) List() []overlay.Host {
	return []overlay.Host{{Name: "", Address: "http://localhost:1234"}}
}
func (oneHost) Current() string                                 { return "" }
func (oneHost) Probe(context.Context, string) ([]string, error) { return nil, nil }

// prefixTranslator marks each reply with the host that translated it.
type prefixTranslator struct{ host string }

func (p prefixTranslator) TranslateReply(_ context.Context, text, _ string, _ func(overlay.Progress)) (string, error) {
	return p.host + ": " + strings.ToUpper(text), nil
}

func TestChoosingAHostInTheReaderTranslatesTheRepliesAgain(t *testing.T) {
	t.Parallel()
	chosen := &choices{}

	reader := newReader(t, &fixedReplies{replies: []string{"all done"}}, prefixTranslator{host: "laptop"},
		overlay.ReaderOptions{
			Source: "Claude Code", Service: "lmstudio · laptop", Language: "DE",
			Hosts: twoHosts{current: "laptop"},
			ChooseHost: func(name string) (overlay.ReplyTranslator, string, error) {
				chosen.add(name)
				return prefixTranslator{host: name}, "lmstudio · " + name, nil
			},
		})
	waitForText(t, reader, "laptop: ALL DONE")

	reader.Send(tea.KeyMsg{Type: tea.KeyCtrlO})
	waitForText(t, reader, "✗ unreachable")
	reader.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	reader.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForText(t, reader, "lmstudio · gpu", "gpu: ALL DONE")

	reader.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	reader.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
	if got := chosen.all(); len(got) != 1 || got[0] != "gpu" {
		t.Errorf("chose %v, want gpu", got)
	}
}
