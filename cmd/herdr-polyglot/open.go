package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/herdr"
	"github.com/wazum/herdr-polyglot/internal/overlay"
	"github.com/wazum/herdr-polyglot/internal/readstate"
)

const (
	pluginIDEnv = "HERDR_PLUGIN_ID"
	socketEnv   = "HERDR_SOCKET_PATH"
	paneEnv     = "HERDR_PANE_ID"
	// configDirEnv is where herdr says the plugin's own settings live.
	configDirEnv = "HERDR_PLUGIN_CONFIG_DIR"
	stateDirEnv  = "HERDR_PLUGIN_STATE_DIR"
)

// runOpen puts the overlay in a floating popup over the pane the keybinding was
// pressed in, so the agent's output stays readable around it. Only the socket
// API can size a popup, which is why this does not go through the CLI.
func runOpen(ctx context.Context, args []string) error {
	pane := os.Getenv(paneEnv)
	if pane == "" {
		return fmt.Errorf("no invoking pane: %s is not set; open this from an agent pane", paneEnv)
	}

	env := map[string]string{
		"HERDR_POLYGLOT_TARGET": pane,
		"HERDR_POLYGLOT_SUBMIT": "1",
	}
	reading, how := false, readInPopup
	for _, arg := range args {
		switch arg {
		case "--review":
			env["HERDR_POLYGLOT_SUBMIT"] = "0"
		case "--read":
			reading = true
		case "--pane":
			how = readInPane
		case "--toggle":
			how = readElsewhere
		}
	}
	if reading {
		store := readstate.New(os.Getenv(stateDirEnv))
		// A key pressed in a reader pane acts for the agent beside it, while
		// that reader is still there: herdr may give the id to another pane.
		agent := store.Owner(pane)
		if agent != pane && store.Tell(ctx, agent, readstate.Pane, "") != nil {
			agent = pane
		}
		return readAction(ctx, how, agent, herdrReaders{store: store})
	}
	width, height := popupSize(ctx, pane, false)

	socket := herdr.NewSocket(orDefault(os.Getenv(socketEnv), defaultSocketPath()))

	_, err := socket.OpenPopup(ctx, herdr.Popup{
		PluginID:   os.Getenv(pluginIDEnv),
		Entrypoint: "overlay",
		Width:      width,
		Height:     height,
		Env:        env,
	})
	return err
}

// popupSize is the size the settings ask for, side by side, and the plugin's
// own where they do not. The agent's replies are read in a taller popup than a
// prompt is written in: a reply is longer. Settings that cannot be read still
// open the popup at its own size, which is where the trouble is said.
func popupSize(ctx context.Context, pane string, reading bool) (width, height any) {
	width, height = overlay.PopupWidth, overlay.PopupHeight()
	if reading {
		height = overlay.ReaderHeight()
	}

	settings, err := config.Load(func(variable string) string {
		switch variable {
		case "HERDR_POLYGLOT_TARGET":
			return pane
		case configDirEnv:
			return configDir(ctx)
		}
		return os.Getenv(variable)
	})
	if err != nil {
		return width, height
	}

	asked := settings.Prompt
	if reading {
		asked = settings.Read.Size
	}
	if asked.Width != nil {
		width = asked.Width
	}
	if asked.Height != nil {
		height = asked.Height
	}
	return width, height
}

// configDir is where the plugin's .env is. Herdr says so to the popup; an
// action may have to ask.
func configDir(ctx context.Context) string {
	if dir := os.Getenv(configDirEnv); dir != "" {
		return dir
	}
	plugin := os.Getenv(pluginIDEnv)
	if plugin == "" {
		return ""
	}
	out, err := herdr.NewExecRunner(orDefault(os.Getenv("HERDR_BIN_PATH"), "herdr")).
		Output(ctx, "plugin", "config-dir", plugin)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func defaultSocketPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home + "/.config/herdr/herdr.sock"
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
