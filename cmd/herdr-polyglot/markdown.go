package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/draft"
	"github.com/wazum/herdr-polyglot/internal/overlay"
)

// mdStyle is the markdown style the reader renders in: the one S chose last,
// or else the one the settings name. Like the host, the choice is the plugin's,
// not a pane's.
type mdStyle struct {
	current string
	kept    draft.Slot
}

func markdownStyle(settings *config.Settings) mdStyle {
	style := mdStyle{
		current: settings.Read.MarkdownStyle,
		kept:    draft.NewStore(settings.StateDir).Of("markdown-style").For("all"),
	}
	remembered, _ := style.kept.Load()
	if remembered = strings.TrimSpace(remembered); slices.Contains(overlay.MarkdownStyles, remembered) {
		style.current = remembered
	}
	return style
}

// keep makes name the style for the readers after this one. Failing to
// remember it costs only the next reader its style.
func (s mdStyle) keep(name string) {
	if err := s.kept.Save(name); err != nil {
		fmt.Fprintln(os.Stderr, "polyglot: the chosen markdown style could not be remembered:", err)
	}
}
