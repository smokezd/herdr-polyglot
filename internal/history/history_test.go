package history_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/wazum/herdr-polyglot/internal/history"
)

func entry(original, translation string) history.Entry {
	return history.Entry{Original: original, Translation: translation, Language: "ZH", At: time.Now()}
}

func originals(t *testing.T, store history.Store) []string {
	t.Helper()
	entries, err := store.Entries()
	if err != nil {
		t.Fatalf("Entries returned unexpected error: %v", err)
	}
	var got []string
	for _, kept := range entries {
		got = append(got, kept.Original)
	}
	return got
}

func TestTranslatedRepliesAreKeptOldestFirst(t *testing.T) {
	t.Parallel()
	store := history.New(t.TempDir(), "/repo")

	for _, original := range []string{"first", "second"} {
		if err := store.Remember(entry(original, "译 "+original)); err != nil {
			t.Fatalf("Remember returned unexpected error: %v", err)
		}
	}

	got := originals(t, store)
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("history is %q, want first then second", got)
	}
}

func TestOnlyTheLatestHundredAreKept(t *testing.T) {
	t.Parallel()
	store := history.New(t.TempDir(), "/repo")

	for index := range history.Kept + 5 {
		if err := store.Remember(entry(strconv.Itoa(index), "x")); err != nil {
			t.Fatal(err)
		}
	}

	got := originals(t, store)
	if len(got) != history.Kept || got[0] != "5" || got[len(got)-1] != strconv.Itoa(history.Kept+4) {
		t.Errorf("history holds %d from %q to %q, want the last %d", len(got), got[0], got[len(got)-1], history.Kept)
	}
}

func TestAReplyTranslatedAgainMovesToTheEndAndIsKeptOnce(t *testing.T) {
	t.Parallel()
	store := history.New(t.TempDir(), "/repo")

	_ = store.Remember(entry("same", "old"))
	_ = store.Remember(entry("other", "x"))
	_ = store.Remember(entry("same", "new"))

	got := originals(t, store)
	if len(got) != 2 || got[0] != "other" || got[1] != "same" {
		t.Errorf("history is %q, want other then same", got)
	}
	if found, ok := store.Find("same", "ZH"); !ok || found.Translation != "new" {
		t.Errorf("found %q (%v), want the newer translation", found.Translation, ok)
	}
}

func TestATranslationIsFoundOnlyInItsOwnLanguage(t *testing.T) {
	t.Parallel()
	store := history.New(t.TempDir(), "/repo")
	_ = store.Remember(entry("hello", "你好"))

	if _, ok := store.Find("hello", "DE"); ok {
		t.Error("a Chinese translation was found for German")
	}
	if _, ok := store.Find("bye", "ZH"); ok {
		t.Error("a reply never translated was found")
	}
}

func TestEachProjectHasAHistoryOfItsOwn(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = history.New(dir, "/repo").Remember(entry("in repo", "x"))

	if got := originals(t, history.New(dir, "/repo-worktree")); len(got) != 0 {
		t.Errorf("another project's history holds %q", got)
	}
}

func TestWithoutAPlaceToKeepItNothingIsKept(t *testing.T) {
	t.Parallel()

	for _, store := range []history.Store{history.New("", "/repo"), history.New(t.TempDir(), "")} {
		if err := store.Remember(entry("a", "b")); err != nil {
			t.Errorf("Remember returned unexpected error: %v", err)
		}
		if got := originals(t, store); len(got) != 0 {
			t.Errorf("history is %q, want nothing", got)
		}
	}
}

func TestTheRootIsTheNearestFolderThatHoldsGit(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	worktree := filepath.Join(repo, "worktrees", "feature")
	plain := t.TempDir()
	for _, dir := range []string{filepath.Join(repo, ".git"), filepath.Join(repo, "src", "deep"), filepath.Join(worktree, "pkg")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A worktree has a .git file pointing at the repository it came from.
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		filepath.Join(repo, "src", "deep"): repo,
		filepath.Join(worktree, "pkg"):     worktree,
		plain:                              plain,
		"":                                 "",
	}
	for from, want := range cases {
		if got := history.Root(from); got != want {
			t.Errorf("Root(%q) is %q, want %q", from, got, want)
		}
	}
}

func TestAReplyReadBeforeItWasFinishedGivesWayToTheWholeOfIt(t *testing.T) {
	t.Parallel()
	store := history.New(t.TempDir(), "/repo")

	_ = store.Remember(entry("Let me look.", "让我看看。"))
	_ = store.Remember(entry("Let me look.\n\nFound it.", "让我看看。\n\n找到了。"))

	if got := originals(t, store); len(got) != 1 || got[0] != "Let me look.\n\nFound it." {
		t.Errorf("history is %q, want only the whole reply", got)
	}
}
