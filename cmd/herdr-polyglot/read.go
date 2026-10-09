package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/herdr"
	"github.com/wazum/herdr-polyglot/internal/overlay"
	"github.com/wazum/herdr-polyglot/internal/transcript"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

// runReader shows the agent's latest replies in the popup, translated into
// the language they are read in. It keeps no draft and delivers nothing.
func runReader(ctx context.Context, settings *config.Settings, runner herdr.ExecRunner) error {
	chosen := chooseReplyService(settings)
	source := transcript.Resolve(ctx, runner, settings.Target, agentHomes())

	// Protecting is what passes the replies before along to a service that can
	// be told about them, with their code left out like the reply's own, and
	// what streams the answer from a service that writes it bit by bit.
	translator := streamingReplies{translation.Protecting(chosen.translator).(translation.StreamingTranslator)}

	language := settings.Read.Language
	if language == "" {
		language = "?"
	}
	program := tea.NewProgram(
		overlay.NewReader(ctx, replies{source}, translator, overlay.ReaderOptions{
			Source:   source.Name(),
			Service:  chosen.name,
			Language: language,
			Replies:  settings.Read.Replies,
			Context:  settings.Read.Context,
			Whole:    source.Whole(),
			Trouble:  chosen.trouble,
			Pulse:    settings.Pulse,
		}),
		tea.WithContext(ctx),
	)
	if _, err := program.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return fmt.Errorf("running reader: %w", err)
	}
	return nil
}

// replies hands the overlay the text of each reply and nothing of where it
// came from.
type replies struct{ source transcript.Source }

func (r replies) Latest(ctx context.Context, n int) ([]string, error) {
	found, err := r.source.Latest(ctx, n)
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(found))
	for i, reply := range found {
		texts[i] = reply.Text
	}
	return texts, nil
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
