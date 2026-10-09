package main

import (
	"context"
	"errors"
	"os"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/herdr"
	"github.com/wazum/herdr-polyglot/internal/readstate"
)

// How a key asks for the agent's replies.
type readHow int

const (
	// readInPopup opens them over the agent, or brings back a hidden popup.
	readInPopup readHow = iota
	// readInPane opens them beside the agent, or translates the latest again
	// in the pane already open.
	readInPane
	// readElsewhere moves what is open to the other place.
	readElsewhere
)

// What a reader is told, through the environment, when it opens.
const (
	placeEnv  = "HERDR_POLYGLOT_PLACE"
	resumeEnv = "HERDR_POLYGLOT_RESUME"

	// resumeHidden takes what a hidden popup was showing, resumeHandoff what a
	// reader moving here brought along.
	resumeHidden  = "hidden"
	resumeHandoff = "handoff"
)

// readers are the agent's readers as a key pressed in herdr finds them.
type readers interface {
	// hidden says a popup for the agent was hidden and can come back.
	hidden(agent string) bool
	// tell passes a word to a reader that is open, or fails when none is.
	tell(ctx context.Context, agent string, place readstate.Place, word string) error
	// open starts a reader, with what it should take over, if anything.
	open(ctx context.Context, agent string, place readstate.Place, resume string) error
	// notify says something in herdr without opening anything.
	notify(ctx context.Context, title, body string)
}

// readAction decides what a key asking for the agent's replies does. A hidden
// popup comes back as it was; a pane already open translates again where it
// is; moving takes whatever is open, or hidden, to the other place.
func readAction(ctx context.Context, how readHow, agent string, open readers) error {
	switch how {
	case readInPane:
		if open.tell(ctx, agent, readstate.Pane, readstate.Refresh) == nil {
			return nil
		}
		return open.open(ctx, agent, readstate.Pane, "")

	case readElsewhere:
		for _, place := range []readstate.Place{readstate.Pane, readstate.Popup} {
			if open.tell(ctx, agent, place, readstate.Move) == nil {
				return nil
			}
		}
		if open.hidden(agent) {
			return open.open(ctx, agent, readstate.Pane, resumeHidden)
		}
		open.notify(ctx, "polyglot: nothing to move",
			"Open the translation in a popup or a pane first.")
		return nil

	default:
		if open.hidden(agent) {
			return open.open(ctx, agent, readstate.Popup, resumeHidden)
		}
		return open.open(ctx, agent, readstate.Popup, "")
	}
}

// herdrReaders finds and opens readers through herdr and the plugin's state.
type herdrReaders struct{ store readstate.Store }

func (h herdrReaders) hidden(agent string) bool {
	kept, err := h.store.Hidden(agent).Load()
	return err == nil && kept != ""
}

func (h herdrReaders) tell(ctx context.Context, agent string, place readstate.Place, word string) error {
	return h.store.Tell(ctx, agent, place, word)
}

// open asks herdr for a popup over the active pane, or a pane beside the agent
// that leaves the keyboard where it is. A pane is remembered as the agent's, so
// a key pressed in it acts for the agent.
func (h herdrReaders) open(ctx context.Context, agent string, place readstate.Place, resume string) error {
	env := map[string]string{
		"HERDR_POLYGLOT_TARGET": agent,
		"HERDR_POLYGLOT_MODE":   config.ReadMode,
		placeEnv:                string(place),
	}
	if resume != "" {
		env[resumeEnv] = resume
	}
	socket := herdr.NewSocket(orDefault(os.Getenv(socketEnv), defaultSocketPath()))
	plugin := os.Getenv(pluginIDEnv)

	if place == readstate.Popup {
		width, height := popupSize(ctx, agent, true)
		_, err := socket.OpenPopup(ctx, herdr.Popup{
			PluginID: plugin, Entrypoint: "overlay", Width: width, Height: height, Env: env,
		})
		return err
	}

	pane, err := socket.OpenSplit(ctx, &herdr.Split{
		PluginID: plugin, Entrypoint: "overlay", Beside: agent, Direction: "right", Env: env,
	})
	if err != nil {
		return err
	}
	if pane == "" {
		return errors.New("herdr opened the pane without saying which it is")
	}
	return h.store.Link(pane, agent)
}

func (h herdrReaders) notify(ctx context.Context, title, body string) {
	_ = herdr.NewExecRunner(orDefault(os.Getenv("HERDR_BIN_PATH"), "herdr")).
		Run(ctx, "notification", "show", title, "--body", body)
}
