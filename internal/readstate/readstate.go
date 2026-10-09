// Package readstate keeps what the reader of an agent's replies has to hand
// on between processes: what a hidden popup was showing, what a reader moving
// from a popup to a pane or back carries with it, which agent a reader pane
// belongs to, and a line to a reader that is running, so a key pressed
// elsewhere can tell it what to do.
package readstate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wazum/herdr-polyglot/internal/draft"
)

// Place is where a reader opens: a popup over the agent or a pane beside it.
type Place string

const (
	Popup Place = "popup"
	Pane  Place = "pane"
)

// Other is where a reader goes when it moves.
func (p Place) Other() Place {
	if p == Pane {
		return Popup
	}
	return Pane
}

// The words a running reader understands.
const (
	Hide    = "hide"
	Refresh = "refresh"
	Move    = "move"
)

// ErrNotOpen is a reader that is not running, or not listening.
var ErrNotOpen = errors.New("no reader is open there")

// tellTimeout is how long a key pressed elsewhere waits for a reader to take
// what it was told.
const tellTimeout = time.Second

// maxSocketPath is a little under what macOS allows for a Unix socket.
const maxSocketPath = 100

// Store is the plugin's state directory, as far as reading goes. An empty one
// keeps nothing and listens for nothing.
type Store struct{ directory string }

func New(directory string) Store {
	return Store{directory: directory}
}

// Hidden is what a popup over the agent was showing when it was hidden, kept
// until the popup is opened again.
func (s Store) Hidden(agent string) draft.Slot {
	return draft.NewStore(s.directory).Of("read-hidden").For(agent)
}

// Handoff is what a reader moving to the other place carries there.
func (s Store) Handoff(agent string, to Place) draft.Slot {
	return draft.NewStore(s.directory).Of("read-handoff-" + string(to)).For(agent)
}

// Link remembers which agent a reader pane belongs to, so a key pressed in the
// reader pane acts for the agent beside it.
func (s Store) Link(readerPane, agent string) error {
	return draft.NewStore(s.directory).Of("read-owner").For(readerPane).Save(agent)
}

// Owner is the agent a pane reads for, or the pane itself when it is not a
// reader.
func (s Store) Owner(pane string) string {
	agent, err := draft.NewStore(s.directory).Of("read-owner").For(pane).Load()
	if err != nil || strings.TrimSpace(agent) == "" {
		return pane
	}
	return strings.TrimSpace(agent)
}

// Listen opens the line to the reader for an agent at a place. A socket left
// behind by a reader that is gone is cleared first; one that still answers
// belongs to a reader that is running.
func (s Store) Listen(agent string, place Place) (net.Listener, error) {
	path, err := s.socketPath(agent, place)
	if err != nil {
		return nil, err
	}
	if s.Tell(context.Background(), agent, place, "") == nil {
		return nil, fmt.Errorf("a reader is already open for %s in a %s", agent, place)
	}
	_ = os.Remove(path)

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening for the reader: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("keeping the reader's line private: %w", err)
	}
	return listener, nil
}

// Serve hands every word that arrives to heard, until the listener closes.
func Serve(listener net.Listener, heard func(word string)) {
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer connection.Close()
			_ = connection.SetDeadline(time.Now().Add(tellTimeout))
			line, err := bufio.NewReader(connection).ReadString('\n')
			if err != nil {
				return
			}
			if word := strings.TrimSpace(line); word != "" {
				heard(word)
			}
			_, _ = connection.Write([]byte("ok\n"))
		}()
	}
}

// Tell passes a word to the reader for an agent at a place. An empty word only
// asks whether it is there.
func (s Store) Tell(ctx context.Context, agent string, place Place, word string) error {
	path, err := s.socketPath(agent, place)
	if err != nil {
		return ErrNotOpen
	}
	ctx, cancel := context.WithTimeout(ctx, tellTimeout)
	defer cancel()

	connection, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return ErrNotOpen
	}
	defer connection.Close()
	if word == "" {
		return nil
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	if _, err := connection.Write([]byte(word + "\n")); err != nil {
		return ErrNotOpen
	}
	if _, err := bufio.NewReader(connection).ReadString('\n'); err != nil {
		return ErrNotOpen
	}
	return nil
}

// socketPath names the line by a digest of the agent pane, so an id with odd
// characters in it cannot point anywhere but the state directory.
func (s Store) socketPath(agent string, place Place) (string, error) {
	if s.directory == "" {
		return "", ErrNotOpen
	}
	digest := sha256.Sum256([]byte(agent))
	path := filepath.Join(s.directory, "read-"+hex.EncodeToString(digest[:6])+"-"+string(place)+".sock")
	if len(path) > maxSocketPath {
		return "", fmt.Errorf("the state directory's path is too long for a socket: %s", path)
	}
	return path, nil
}
