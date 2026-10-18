package clipboard_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/clipboard"
)

func TestMacOSUsesPbcopyAndWaylandComesBeforeX(t *testing.T) {
	t.Parallel()

	if tools := clipboard.Tools("darwin", false); len(tools) != 1 || tools[0][0] != "pbcopy" {
		t.Errorf("macOS tools are %q, want pbcopy", tools)
	}
	if tools := clipboard.Tools("linux", true); tools[0][0] != "wl-copy" || tools[1][0] != "xclip" {
		t.Errorf("Wayland tools are %q, want wl-copy first", tools)
	}
	if tools := clipboard.Tools("linux", false); tools[0][0] != "xclip" {
		t.Errorf("X tools are %q, want xclip first", tools)
	}
}

func TestTheFirstToolInstalledGetsTheText(t *testing.T) {
	dir := t.TempDir()
	kept := filepath.Join(dir, "kept")
	script := "#!/bin/sh\n/bin/cat > '" + kept + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "fakecopy"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	err := clipboard.Write(context.Background(), [][]string{{"not-installed"}, {"fakecopy"}}, "hallo welt")
	if err != nil {
		t.Fatalf("Write returned %v", err)
	}
	if got, _ := os.ReadFile(kept); string(got) != "hallo welt" {
		t.Errorf("the tool got %q, want the text", got)
	}
}

func TestWithoutAToolItSaysSo(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if err := clipboard.Write(context.Background(), [][]string{{"pbcopy"}}, "x"); !errors.Is(err, clipboard.ErrNoTool) {
		t.Errorf("Write returned %v, want ErrNoTool", err)
	}
}
