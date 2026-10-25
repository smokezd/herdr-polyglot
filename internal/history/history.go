// Package history keeps the replies a reader translated, per project, so one
// already translated is shown again without asking the service, and earlier
// ones can be read back.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wazum/herdr-polyglot/internal/draft"
)

// Kept is how many translated replies a project keeps. The oldest goes first.
const Kept = 100

// Entry is one reply and its translation.
type Entry struct {
	Original    string    `json:"original"`
	Translation string    `json:"translation"`
	Language    string    `json:"language"`
	At          time.Time `json:"at"`
}

// Store is the history of one project, kept in one file beside the drafts.
type Store struct{ slot draft.Slot }

// New takes the directory herdr set aside for the plugin's state and the root
// of the project. Without either nothing is kept.
func New(stateDir, root string) Store {
	if root == "" {
		return Store{}
	}
	return Store{slot: draft.NewStore(stateDir).Of("history").For(root)}
}

// Entries are the translated replies, oldest first.
func (s Store) Entries() ([]Entry, error) {
	kept, err := s.slot.Load()
	if err != nil || kept == "" {
		return nil, err
	}
	var entries []Entry
	if err := json.Unmarshal([]byte(kept), &entries); err != nil {
		return nil, fmt.Errorf("reading the history: %w", err)
	}
	return entries, nil
}

// Find is the latest translation of original into language.
func (s Store) Find(original, language string) (Entry, bool) {
	entries, _ := s.Entries()
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].Original == original && entries[index].Language == language {
			return entries[index], true
		}
	}
	return Entry{}, false
}

// Remember keeps a translation as the newest. One of the same reply into the
// same language before it is dropped, and so is one of the reply as far as it
// had come when it was read before the agent finished it. The oldest past Kept
// goes too. Two readers of one project writing at once may lose one of the two.
func (s Store) Remember(entry Entry) error {
	entries, err := s.Entries()
	if err != nil {
		// A history that cannot be read is started again, not kept from growing.
		entries = nil
	}
	fresh := make([]Entry, 0, len(entries)+1)
	for _, kept := range entries {
		if kept.Language != entry.Language || !grewInto(kept.Original, entry.Original) {
			fresh = append(fresh, kept)
		}
	}
	fresh = append(fresh, entry)
	if len(fresh) > Kept {
		fresh = fresh[len(fresh)-Kept:]
	}
	encoded, err := json.Marshal(fresh)
	if err != nil {
		return fmt.Errorf("keeping the history: %w", err)
	}
	return s.slot.Save(string(encoded))
}

// grewInto says earlier is the reply whole, or the reply as far as it had come:
// an agent adds to a reply a paragraph at a time.
func grewInto(earlier, whole string) bool {
	return earlier == whole || strings.HasPrefix(whole, earlier+"\n\n")
}

// Root is the project dir is in: the nearest folder up from it that holds a
// .git, a folder in a repository or a file in a worktree, so every worktree is
// a project of its own. Outside any repository it is dir itself.
func Root(dir string) string {
	if dir == "" {
		return ""
	}
	for at := filepath.Clean(dir); ; {
		if _, err := os.Lstat(filepath.Join(at, ".git")); err == nil {
			return at
		}
		up := filepath.Dir(at)
		if up == at {
			return filepath.Clean(dir)
		}
		at = up
	}
}
