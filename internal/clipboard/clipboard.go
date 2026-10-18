// Package clipboard puts text on the system clipboard through the tool the
// system has for it. The reader runs in a terminal inside herdr, where an
// escape sequence asking the terminal to copy may never reach the real one.
package clipboard

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// writeTimeout is how long a clipboard tool has to take the text.
const writeTimeout = 2 * time.Second

// ErrNoTool is a system with none of the tools there are for the clipboard.
var ErrNoTool = errors.New("no clipboard tool found (pbcopy, wl-copy, xclip or xsel)")

// Tools are the commands that write the clipboard, in the order they are tried:
// macOS has one, and Linux has one for Wayland and two for X.
func Tools(goos string, wayland bool) [][]string {
	if goos == "darwin" {
		return [][]string{{"pbcopy"}}
	}
	tools := [][]string{{"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	if wayland {
		tools = append([][]string{{"wl-copy"}}, tools...)
	}
	return tools
}

// Write gives text to the first of tools that is installed.
func Write(ctx context.Context, tools [][]string, text string) error {
	for _, tool := range tools {
		path, err := exec.LookPath(tool[0])
		if err != nil {
			continue
		}
		return run(ctx, tool[0], path, tool[1:], text)
	}
	return ErrNoTool
}

func run(ctx context.Context, name, path string, args []string, text string) error {
	writing, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	// The command is one of Tools, found on the PATH, and the text goes in on
	// stdin, never as an argument.
	command := exec.CommandContext(writing, path, args...) //nolint:gosec // see above
	command.Stdin = strings.NewReader(text)
	if out, err := command.CombinedOutput(); err != nil {
		if said := strings.TrimSpace(string(out)); said != "" {
			return errors.New(name + ": " + said)
		}
		return errors.New(name + ": " + err.Error())
	}
	return nil
}
