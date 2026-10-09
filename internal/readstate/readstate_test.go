package readstate_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/wazum/herdr-polyglot/internal/readstate"
)

// shortDir keeps a socket path under the hundred or so characters allowed,
// which a per-test TempDir does not.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "rs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestARunningReaderHearsWhatItIsTold(t *testing.T) {
	t.Parallel()
	store := readstate.New(shortDir(t))

	listener, err := store.Listen("w1:p1", readstate.Pane)
	if err != nil {
		t.Fatalf("Listen returned unexpected error: %v", err)
	}
	heard := make(chan string, 1)
	go readstate.Serve(listener, func(word string) { heard <- word })
	defer listener.Close()

	if err := store.Tell(context.Background(), "w1:p1", readstate.Pane, readstate.Refresh); err != nil {
		t.Fatalf("Tell returned unexpected error: %v", err)
	}
	select {
	case word := <-heard:
		if word != readstate.Refresh {
			t.Errorf("heard %q, want refresh", word)
		}
	case <-time.After(time.Second):
		t.Fatal("the reader heard nothing")
	}

	if err := store.Tell(context.Background(), "w1:p1", readstate.Popup, readstate.Hide); !errors.Is(err, readstate.ErrNotOpen) {
		t.Errorf("telling a popup nobody opened returned %v, want ErrNotOpen", err)
	}
	if err := store.Tell(context.Background(), "w1:p2", readstate.Pane, readstate.Hide); !errors.Is(err, readstate.ErrNotOpen) {
		t.Errorf("telling another agent's reader returned %v, want ErrNotOpen", err)
	}
}

func TestALineLeftBehindByAReaderThatIsGoneIsTakenOver(t *testing.T) {
	t.Parallel()
	store := readstate.New(shortDir(t))

	first, err := store.Listen("w1:p1", readstate.Popup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Listen("w1:p1", readstate.Popup); err == nil {
		t.Error("a second reader took the line of one still running")
	}
	_ = first.Close()

	second, err := store.Listen("w1:p1", readstate.Popup)
	if err != nil {
		t.Fatalf("the line of a closed reader could not be taken over: %v", err)
	}
	_ = second.Close()
}

func TestAReaderPaneActsForTheAgentBesideIt(t *testing.T) {
	t.Parallel()
	store := readstate.New(shortDir(t))

	if err := store.Link("w1:p9", "w1:p1"); err != nil {
		t.Fatal(err)
	}
	if owner := store.Owner("w1:p9"); owner != "w1:p1" {
		t.Errorf("the reader pane belongs to %q, want w1:p1", owner)
	}
	if owner := store.Owner("w1:p1"); owner != "w1:p1" {
		t.Errorf("the agent pane belongs to %q, want itself", owner)
	}
}

func TestHiddenAndHandedOnContentAreKeptApart(t *testing.T) {
	t.Parallel()
	store := readstate.New(shortDir(t))

	if err := store.Hidden("w1:p1").Save("hidden"); err != nil {
		t.Fatal(err)
	}
	if err := store.Handoff("w1:p1", readstate.Pane).Save("moving"); err != nil {
		t.Fatal(err)
	}
	if kept, _ := store.Hidden("w1:p1").Load(); kept != "hidden" {
		t.Errorf("hidden is %q", kept)
	}
	if kept, _ := store.Handoff("w1:p1", readstate.Pane).Load(); kept != "moving" {
		t.Errorf("handoff is %q", kept)
	}
	if kept, _ := store.Hidden("w1:p2").Load(); kept != "" {
		t.Errorf("another agent's hidden popup is %q, want nothing", kept)
	}
}
