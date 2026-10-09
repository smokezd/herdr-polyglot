package main

import (
	"context"
	"slices"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/readstate"
)

// fakeReaders is what is open and hidden, and records what was done.
type fakeReaders struct {
	open      map[readstate.Place]bool
	hiddenNow bool
	did       []string
}

func (f *fakeReaders) hidden(string) bool { return f.hiddenNow }

func (f *fakeReaders) tell(_ context.Context, _ string, place readstate.Place, word string) error {
	if !f.open[place] {
		return readstate.ErrNotOpen
	}
	f.did = append(f.did, "tell "+string(place)+" "+word)
	return nil
}

func (f *fakeReaders) notify(context.Context, string, string) { f.did = append(f.did, "notify") }

type recordingReaders struct{ *fakeReaders }

func (r recordingReaders) open(_ context.Context, _ string, place readstate.Place, resume string) error {
	r.did = append(r.did, "open "+string(place)+" "+resume)
	return nil
}

func TestWhatEachKeyDoesDependsOnWhatIsOpenOrHidden(t *testing.T) {
	t.Parallel()

	for _, case_ := range []struct {
		name   string
		how    readHow
		open   map[readstate.Place]bool
		hidden bool
		want   []string
	}{
		{
			"a popup with nothing hidden translates afresh", readInPopup, nil, false,
			[]string{"open popup "},
		},
		{
			"a hidden popup comes back as it was", readInPopup, nil, true,
			[]string{"open popup hidden"},
		},
		{
			"a pane where there is none opens one", readInPane, nil, false,
			[]string{"open pane "},
		},
		{
			"a pane already open translates again", readInPane,
			map[readstate.Place]bool{readstate.Pane: true},
			false,
			[]string{"tell pane refresh"},
		},
		{
			"moving an open pane makes it a popup", readElsewhere,
			map[readstate.Place]bool{readstate.Pane: true},
			false,
			[]string{"tell pane move"},
		},
		{
			"moving a hidden popup opens it as a pane", readElsewhere, nil, true,
			[]string{"open pane hidden"},
		},
		{
			"moving nothing says so", readElsewhere, nil, false,
			[]string{"notify"},
		},
	} {
		fake := &fakeReaders{open: case_.open, hiddenNow: case_.hidden}
		if err := readAction(context.Background(), case_.how, "w1:p1", recordingReaders{fake}); err != nil {
			t.Errorf("%s: unexpected error: %v", case_.name, err)
		}
		if !slices.Equal(fake.did, case_.want) {
			t.Errorf("%s: did %q, want %q", case_.name, fake.did, case_.want)
		}
	}
}
