package main

import (
	"testing"

	"github.com/wazum/herdr-polyglot/internal/config"
)

func TestTheMarkdownStyleChosenLastWinsOverTheSetting(t *testing.T) {
	settings := config.Settings{StateDir: t.TempDir(), Read: config.ReadSettings{MarkdownStyle: "light"}}

	if got := markdownStyle(&settings); got.current != "light" {
		t.Errorf("before any is chosen the style is %q, want the setting's", got.current)
	}
	markdownStyle(&settings).keep("dracula")
	if got := markdownStyle(&settings); got.current != "dracula" {
		t.Errorf("the style is %q, want the one chosen last", got.current)
	}
}

func TestAMarkdownStyleRememberedThatIsNoLongerOneIsPassedBy(t *testing.T) {
	settings := config.Settings{StateDir: t.TempDir(), Read: config.ReadSettings{MarkdownStyle: "light"}}

	markdownStyle(&settings).keep("solarized")
	if got := markdownStyle(&settings); got.current != "light" {
		t.Errorf("the style is %q, want the setting's", got.current)
	}
}
