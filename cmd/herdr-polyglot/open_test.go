package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/overlay"
)

func withDotenv(t *testing.T, dotenv string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)
}

func TestEachPopupTakesTheSizeSetForIt(t *testing.T) {
	withDotenv(t, "HERDR_POLYGLOT_WIDTH=120\nHERDR_POLYGLOT_READ_WIDTH=80%\nHERDR_POLYGLOT_READ_HEIGHT=40\n")

	width, height := popupSize(context.Background(), "w1:p1", false)
	if width != 120 || height != overlay.PopupHeight() {
		t.Errorf("the prompt popup is %v×%v, want 120 wide and its own height", width, height)
	}
	width, height = popupSize(context.Background(), "w1:p1", true)
	if width != "80%" || height != 40 {
		t.Errorf("the reading popup is %v×%v, want 80%% × 40", width, height)
	}
}

func TestASizeThatCannotBeReadOpensThePopupAtItsOwn(t *testing.T) {
	withDotenv(t, "HERDR_POLYGLOT_READ_WIDTH=huge\n")

	width, height := popupSize(context.Background(), "w1:p1", true)
	if width != overlay.PopupWidth || height != overlay.ReaderHeight() {
		t.Errorf("the reading popup is %v×%v, want its own size", width, height)
	}
}
