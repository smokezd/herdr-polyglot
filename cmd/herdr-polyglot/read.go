package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wazum/herdr-polyglot/internal/clipboard"
	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/draft"
	"github.com/wazum/herdr-polyglot/internal/herdr"
	"github.com/wazum/herdr-polyglot/internal/history"
	"github.com/wazum/herdr-polyglot/internal/lmstudio"
	"github.com/wazum/herdr-polyglot/internal/overlay"
	"github.com/wazum/herdr-polyglot/internal/readstate"
	"github.com/wazum/herdr-polyglot/internal/transcript"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

// runReader shows the agent's latest replies, translated into the language
// they are read in, in a popup over the agent or a pane beside it. It keeps no
// draft and delivers nothing. A reader hidden or moved hands on what it shows,
// and the one that takes over shows it again without asking the service twice.
func runReader(ctx context.Context, settings *config.Settings, hosts *lmHosts, runner herdr.ExecRunner) error {
	chosen := chooseReplyService(hosts.applied())
	store := readstate.New(settings.StateDir)
	agent := settings.Target
	place := readstate.Popup
	if os.Getenv(placeEnv) == string(readstate.Pane) {
		place = readstate.Pane
	}
	resumed := takeOver(store, agent, place, os.Getenv(resumeEnv))
	if place == readstate.Popup {
		// A popup open now replaces whatever an earlier one hid: closing this
		// one must leave nothing to come back to.
		_ = store.Hidden(agent).Clear()
	}

	// The replies are only looked for when there is no earlier reader's to
	// show, or once they are asked for again.
	found := &replies{find: func() transcript.Source {
		return transcript.Resolve(ctx, runner, agent, agentHomes())
	}}
	options := overlay.ReaderOptions{
		Service:       hosts.label(chosen.name),
		Language:      orDefault(settings.Read.Language, "?"),
		Replies:       settings.Read.Replies,
		Context:       settings.Read.Context,
		Markdown:      settings.Read.Markdown,
		MarkdownStyle: settings.Read.MarkdownStyle,
		Trouble:       chosen.trouble,
		Pulse:         settings.Pulse,
		InPane:        place == readstate.Pane,
		Resumed:       resumed,
		Copy: func(text string) error {
			tools := clipboard.Tools(runtime.GOOS, os.Getenv("WAYLAND_DISPLAY") != "")
			return clipboard.Write(ctx, tools, text)
		},
		Move: func(saved overlay.Snapshot) error {
			if err := keep(store.Handoff(agent, place.Other()), &saved); err != nil {
				return err
			}
			return herdrReaders{store: store}.open(ctx, agent, place.Other(), resumeHandoff)
		},
	}
	if chosen.name == (lmstudio.Provider{}).Name() {
		options.Hosts = hosts
		options.ChooseHost = func(name string) (overlay.ReplyTranslator, string, error) {
			hosts.choose(name)
			chosen := chooseReplyService(hosts.applied())
			return replyTranslator(chosen.translator), hosts.label(chosen.name), chosen.trouble
		}
	}
	options.History = projectHistory{
		store:    history.New(settings.StateDir, history.Root(transcript.Dir(ctx, runner, agent))),
		language: options.Language,
	}
	if place == readstate.Popup {
		options.Hide = func(saved overlay.Snapshot) error { return keep(store.Hidden(agent), &saved) }
	}
	if resumed == nil {
		source := found.source()
		options.Source, options.Whole = source.Name(), source.Whole()
	}

	program := tea.NewProgram(
		overlay.NewReader(ctx, found, replyTranslator(chosen.translator), options), tea.WithContext(ctx))

	// A key pressed in herdr reaches the reader through its line: translate
	// again, hide, move.
	if listener, err := store.Listen(agent, place); err == nil {
		defer listener.Close()
		go readstate.Serve(listener, func(word string) {
			switch word {
			case readstate.Refresh:
				program.Send(overlay.RefreshAsked())
			case readstate.Hide:
				program.Send(overlay.HideAsked())
			case readstate.Move:
				program.Send(overlay.MoveAsked())
			}
		})
	}

	if _, err := program.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("running reader: %w", err)
	}
	return nil
}

// takeOver reads what an earlier reader handed on, and clears it: it is shown
// once, by this reader.
func takeOver(store readstate.Store, agent string, place readstate.Place, resume string) *overlay.Snapshot {
	var slot draft.Slot
	switch resume {
	case resumeHidden:
		slot = store.Hidden(agent)
	case resumeHandoff:
		slot = store.Handoff(agent, place)
	default:
		return nil
	}
	kept, err := slot.Load()
	_ = slot.Clear()
	if err != nil || kept == "" {
		return nil
	}
	var saved overlay.Snapshot
	if json.Unmarshal([]byte(kept), &saved) != nil {
		return nil
	}
	return &saved
}

func keep(slot draft.Slot, saved *overlay.Snapshot) error {
	encoded, err := json.Marshal(saved)
	if err != nil {
		return fmt.Errorf("keeping what is on show: %w", err)
	}
	return slot.Save(string(encoded))
}

// projectHistory hands the overlay the translations kept for the project the
// agent works in, in the language read in now.
type projectHistory struct {
	store    history.Store
	language string
}

func (p projectHistory) Entries() []overlay.HistoryEntry {
	kept, _ := p.store.Entries()
	entries := make([]overlay.HistoryEntry, len(kept))
	for i, entry := range kept {
		entries[i] = overlay.HistoryEntry{Original: entry.Original, Translation: entry.Translation, At: entry.At}
	}
	return entries
}

func (p projectHistory) Find(original string) (string, bool) {
	entry, found := p.store.Find(original, p.language)
	return entry.Translation, found
}

// Remember keeps what it can: a history that cannot be written costs only a
// translation next time.
func (p projectHistory) Remember(original, translated string) {
	_ = p.store.Remember(history.Entry{
		Original: original, Translation: translated,
		Language: p.language, At: time.Now(),
	})
}

// replies hands the overlay the text of each reply and nothing of where it
// came from.
type replies struct {
	find  func() transcript.Source
	once  sync.Once
	found transcript.Source
}

func (r *replies) source() transcript.Source {
	r.once.Do(func() { r.found = r.find() })
	return r.found
}

func (r *replies) Latest(ctx context.Context, n int) ([]string, error) {
	found, err := r.source().Latest(ctx, n)
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(found))
	for i, reply := range found {
		texts[i] = reply.Text
	}
	return texts, nil
}

// All is every reply of the session with the prompt it answered, for the list
// a reply is picked from. A prompt the agent has said nothing to yet has no
// reply to pick.
func (r *replies) All(ctx context.Context) ([]overlay.Turn, error) {
	found, err := r.source().Latest(ctx, math.MaxInt)
	if err != nil {
		return nil, err
	}
	turns := make([]overlay.Turn, 0, len(found))
	for _, reply := range found {
		if reply.Text != "" {
			turns = append(turns, overlay.Turn{Prompt: reply.Prompt, Reply: reply.Text})
		}
	}
	return turns, nil
}

// agentHomes are where Claude Code and Codex keep their sessions, moved where
// each agent's own setting says.
func agentHomes() transcript.Places {
	home, _ := os.UserHomeDir()
	in := func(variable, fallback string) string {
		if dir := os.Getenv(variable); dir != "" {
			return dir
		}
		if home == "" {
			return ""
		}
		return filepath.Join(home, fallback)
	}
	return transcript.Places{
		Claude: in("CLAUDE_CONFIG_DIR", ".claude"),
		Codex:  in("CODEX_HOME", ".codex"),
	}
}

// replyTranslator puts translator in the overlay's words. Protecting is what
// passes the replies before along to a service that can be told about them,
// with their code left out like the reply's own, and what streams the answer
// from a service that writes it bit by bit.
func replyTranslator(translator translation.Translator) overlay.ReplyTranslator {
	return streamingReplies{translation.Protecting(translator).(translation.StreamingTranslator)}
}

// streamingReplies hands the overlay a translation as it is written, in the
// overlay's own words.
type streamingReplies struct {
	translator translation.StreamingTranslator
}

func (s streamingReplies) TranslateReply(
	ctx context.Context, text, preceding string, progress func(overlay.Progress),
) (string, error) {
	return s.translator.TranslateStreaming(ctx, text, preceding, func(so translation.Progress) {
		progress(overlay.Progress{Thinking: so.Thinking, Text: so.Text, Thought: so.Thought})
	})
}
