package overlay_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/wazum/herdr-polyglot/internal/overlay"
)

type fixedReplies struct {
	replies []string
	err     error
	asked   int
}

func (f *fixedReplies) Latest(_ context.Context, n int) ([]string, error) {
	f.asked = n
	if len(f.replies) > n {
		return f.replies[len(f.replies)-n:], f.err
	}
	return f.replies, f.err
}

// upperTranslator shouts each reply and remembers what it was told came before.
type upperTranslator struct {
	mu       sync.Mutex
	contexts []string
	err      error
}

func (u *upperTranslator) TranslateReply(_ context.Context, text, preceding string, _ func(overlay.Progress)) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.contexts = append(u.contexts, preceding)
	if u.err != nil {
		return "", u.err
	}
	return strings.ToUpper(text), nil
}

func (u *upperTranslator) told() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.contexts...)
}

func newReader(t *testing.T, replies overlay.Replies, translator overlay.ReplyTranslator,
	options overlay.ReaderOptions,
) *teatest.TestModel {
	t.Helper()
	if options.Source == "" {
		options.Source, options.Service, options.Language = "Claude Code", "deepl", "DE"
	}
	return teatest.NewTestModel(t,
		overlay.NewReader(context.Background(), replies, translator, options),
		teatest.WithInitialTermSize(80, 24))
}

func waitForText(t *testing.T, reader *teatest.TestModel, texts ...string) {
	t.Helper()
	teatest.WaitFor(t, reader.Output(), func(out []byte) bool {
		for _, text := range texts {
			if !bytes.Contains(out, []byte(text)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(3*time.Second))
}

func TestTheLatestReplyIsShownTranslatedAndTheOnesBeforeGoAlongUnseen(t *testing.T) {
	t.Parallel()
	replies := &fixedReplies{replies: []string{"older", "old", "newest answer"}}
	translator := &upperTranslator{}

	reader := newReader(t, replies, translator, overlay.ReaderOptions{
		Source: "Claude Code", Service: "deepl", Language: "DE", Replies: 1, Context: 2,
	})
	waitForText(t, reader, "NEWEST ANSWER", "Claude Code → DE", "1 reply, 2 before as context")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if replies.asked != 3 {
		t.Errorf("asked for %d replies, want the one shown and two before it", replies.asked)
	}
	if told := translator.told(); len(told) != 1 || told[0] != "older\n\nold" {
		t.Errorf("the service was told %q came before, want the two earlier replies", told)
	}
	final := reader.FinalModel(t).View()
	if strings.Contains(final, "OLDER") {
		t.Errorf("a reply sent only as context was shown:\n%s", final)
	}
}

func TestEachShownReplyIsTranslatedKnowingTheOnesBeforeIt(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{replies: []string{"first", "second"}}, translator,
		overlay.ReaderOptions{Source: "Codex", Service: "lmstudio", Language: "ZH", Replies: 2})
	waitForText(t, reader, "FIRST", "SECOND", "2 replies")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 2 || told[0] != "" || told[1] != "first" {
		t.Errorf("the service was told %q came before, want nothing and then the first reply", told)
	}
}

func TestTabShowsTheReplyAsTheAgentWroteIt(t *testing.T) {
	t.Parallel()

	reader := newReader(t, &fixedReplies{replies: []string{"as written"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "AS WRITTEN")
	reader.Send(tea.KeyMsg{Type: tea.KeyTab})
	waitForText(t, reader, "as written", "original")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

func TestAFailedTranslationSaysWhyAndCanBeTriedAgain(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{err: errors.New("LM Studio could not be reached")}

	reader := newReader(t, &fixedReplies{replies: []string{"try me"}}, translator,
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "LM Studio could not be reached", "ctrl+t")

	translator.mu.Lock()
	translator.err = nil
	translator.mu.Unlock()
	reader.Send(tea.KeyMsg{Type: tea.KeyCtrlT})
	waitForText(t, reader, "TRY ME")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

func TestWithoutALanguageTheRepliesAreShownAsWrittenAndThePopupSaysWhatIsMissing(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{replies: []string{"untouched"}}, translator, overlay.ReaderOptions{
		Source: "Claude Code", Service: "deepl", Language: "?", Replies: 1,
		Trouble: errors.New("HERDR_POLYGLOT_READ_LANGUAGE is not set"),
	})
	waitForText(t, reader, "untouched", "HERDR_POLYGLOT_READ_LANGUAGE is not set")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 0 {
		t.Errorf("the service was asked %d times, want never", len(told))
	}
}

func TestAnAgentThatHasSaidNothingYetIsSaidSo(t *testing.T) {
	t.Parallel()

	reader := newReader(t, &fixedReplies{}, &upperTranslator{}, overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "has not said anything yet")
	reader.Send(tea.KeyMsg{Runes: []rune("q"), Type: tea.KeyRunes})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

func TestALongReplyScrolls(t *testing.T) {
	t.Parallel()
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "line"
	}
	lines[59] = "the very end"

	reader := newReader(t, &fixedReplies{replies: []string{strings.Join(lines, "\n")}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "THE VERY END", "100%", overlay.ScrollThumb)
	reader.Send(tea.KeyMsg{Runes: []rune("g"), Type: tea.KeyRunes})
	teatest.WaitFor(t, reader.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("LINE")) && !bytes.Contains(out, []byte("THE VERY END"))
	}, teatest.WithDuration(3*time.Second))
	reader.Send(tea.KeyMsg{Runes: []rune("G"), Type: tea.KeyRunes})
	waitForText(t, reader, "THE VERY END", "100%")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

// thinkingTranslator thinks out loud, waits until the test lets it go on, and
// then writes its answer in two pieces.
type thinkingTranslator struct {
	thinking string
	answer   []string
	goOn     chan struct{}
}

func (k *thinkingTranslator) TranslateReply(
	_ context.Context, _, _ string, progress func(overlay.Progress),
) (string, error) {
	progress(overlay.Progress{Thinking: k.thinking})
	<-k.goOn
	so := ""
	for _, piece := range k.answer {
		so += piece
		progress(overlay.Progress{Thinking: k.thinking, Text: so, Thought: true})
	}
	return so, nil
}

func TestThinkingIsShownWhileItGoesOnAndFoldsAwayOnceTheAnswerBegins(t *testing.T) {
	t.Parallel()
	translator := &thinkingTranslator{
		thinking: "Considering the word for flaky",
		answer:   []string{"不稳定的测试", "已修复。"},
		goOn:     make(chan struct{}),
	}

	reader := newReader(t, &fixedReplies{replies: []string{"The flaky test is fixed."}}, translator,
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "thinking …", "Considering the word for flaky")

	close(translator.goOn)
	waitForText(t, reader, "不稳定的测试已修复。", "▸ thought for")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	final := reader.FinalModel(t).View()
	if strings.Contains(final, "Considering") {
		t.Errorf("the thinking is still spread out after the answer arrived:\n%s", final)
	}
}

// streamingTranslator writes its answer one line at a time.
type streamingTranslator struct{ lines []string }

func (s streamingTranslator) TranslateReply(
	_ context.Context, _, _ string, progress func(overlay.Progress),
) (string, error) {
	so := ""
	for _, line := range s.lines {
		so += line + "\n"
		progress(overlay.Progress{Text: so})
		time.Sleep(time.Millisecond)
	}
	return so, nil
}

func TestAStreamedAnswerKeepsItsEndInView(t *testing.T) {
	t.Parallel()
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "zeile " + strconv.Itoa(i)
	}

	reader := newReader(t, &fixedReplies{replies: []string{"long"}}, streamingTranslator{lines: lines},
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "zeile 59", "100%")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if final := reader.FinalModel(t).View(); strings.Contains(final, "zeile 0\n") {
		t.Errorf("the view stayed at the top while the answer grew:\n%s", final)
	}
}

func TestAHiddenPopupComesBackAsItWasWithoutAskingTheServiceAgain(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{}
	var kept overlay.Snapshot

	first := newReader(t, &fixedReplies{replies: []string{"first answer"}}, translator, overlay.ReaderOptions{
		Replies: 1,
		Hide:    func(saved overlay.Snapshot) error { kept = saved; return nil },
	})
	waitForText(t, first, "FIRST ANSWER", "h hide")
	first.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	first.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if len(kept.Results) != 1 || kept.Results[0].Text != "FIRST ANSWER" || kept.Next != 1 {
		t.Fatalf("kept %+v, want the finished translation", kept)
	}

	asked := len(translator.told())
	replies := &fixedReplies{replies: []string{"a newer answer"}}
	again := newReader(t, replies, translator, overlay.ReaderOptions{Replies: 1, Resumed: &kept})
	waitForText(t, again, "FIRST ANSWER")
	again.Send(tea.KeyMsg{Type: tea.KeyEsc})
	again.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if now := len(translator.told()); now != asked {
		t.Errorf("the service was asked %d more times, want none", now-asked)
	}
	if replies.asked != 0 {
		t.Error("the agent's replies were read again for a popup that came back")
	}
}

func TestAReplyLeftUnfinishedIsTranslatedByTheReaderThatTakesOver(t *testing.T) {
	t.Parallel()
	kept := overlay.Snapshot{
		Source: "Claude Code", Shown: []string{"done", "half"},
		Results: []overlay.SnapshotReply{{Text: "FERTIG"}, {}}, Next: 1,
	}
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{}, translator, overlay.ReaderOptions{Replies: 2, Resumed: &kept})
	waitForText(t, reader, "FERTIG", "HALF")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 1 || told[0] != "done" {
		t.Errorf("the service was told %q, want only the unfinished reply, after the finished one", told)
	}
}

func TestARefreshDropsWhatWasShownAndTranslatesTheLatestReplies(t *testing.T) {
	t.Parallel()
	replies := &fixedReplies{replies: []string{"old answer"}}

	reader := newReader(t, replies, &upperTranslator{}, overlay.ReaderOptions{Replies: 1, InPane: true})
	waitForText(t, reader, "OLD ANSWER")
	replies.replies = []string{"new answer"}
	reader.Send(overlay.RefreshAsked())
	waitForText(t, reader, "NEW ANSWER")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if final := reader.FinalModel(t).View(); strings.Contains(final, "OLD ANSWER") {
		t.Errorf("the old translation is still on show:\n%s", final)
	}
}

func TestMovingHandsWhatIsShownToTheOtherPlace(t *testing.T) {
	t.Parallel()
	moved := make(chan overlay.Snapshot, 1)

	reader := newReader(t, &fixedReplies{replies: []string{"answer"}}, &upperTranslator{}, overlay.ReaderOptions{
		Replies: 1, InPane: true,
		Move: func(saved overlay.Snapshot) error { moved <- saved; return nil },
	})
	waitForText(t, reader, "ANSWER", "m → popup")
	reader.Send(overlay.MoveAsked())
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if saved := <-moved; len(saved.Results) != 1 || saved.Results[0].Text != "ANSWER" {
		t.Errorf("moved %+v, want the translation", saved)
	}
}

func TestAPaneCannotBeHidden(t *testing.T) {
	t.Parallel()
	hidden := false

	reader := newReader(t, &fixedReplies{replies: []string{"answer"}}, &upperTranslator{}, overlay.ReaderOptions{
		Replies: 1, InPane: true,
		Hide: func(overlay.Snapshot) error { hidden = true; return nil },
	})
	waitForText(t, reader, "ANSWER")
	reader.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if hidden {
		t.Error("a pane was hidden")
	}
}

// numbered is a reply of that many lines, each saying which it is.
func numbered(lines int) string {
	text := make([]string, lines)
	for i := range text {
		text[i] = "zeile " + strconv.Itoa(i)
	}
	return strings.Join(text, "\n")
}

func readerAt(t *testing.T, width int, options overlay.ReaderOptions) *teatest.TestModel {
	t.Helper()
	options.Source, options.Service, options.Language, options.Replies = "Claude Code", "deepl", "DE", 1
	return teatest.NewTestModel(t,
		overlay.NewReader(context.Background(), &fixedReplies{replies: []string{numbered(100)}},
			&upperTranslator{}, options),
		teatest.WithInitialTermSize(width, 24))
}

func TestAHiddenPopupComesBackScrolledWhereItWas(t *testing.T) {
	t.Parallel()
	var kept overlay.Snapshot

	first := readerAt(t, 80, overlay.ReaderOptions{
		Hide: func(saved overlay.Snapshot) error { kept = saved; return nil },
	})
	waitForText(t, first, "ZEILE 99")
	first.Send(tea.KeyMsg{Runes: []rune("g"), Type: tea.KeyRunes})
	for range 40 {
		first.Send(tea.KeyMsg{Type: tea.KeyDown})
	}
	waitForText(t, first, "ZEILE 40")
	first.Send(tea.KeyMsg{Runes: []rune("h"), Type: tea.KeyRunes})
	first.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	if kept.From != 40 || kept.AtEnd {
		t.Fatalf("kept the view at row %d (at the end: %v), want row 40", kept.From, kept.AtEnd)
	}

	again := readerAt(t, 80, overlay.ReaderOptions{Resumed: &kept})
	again.Send(tea.KeyMsg{Type: tea.KeyEsc})
	again.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	view := again.FinalModel(t).View()
	if !strings.Contains(view, "ZEILE 40 ") || strings.Contains(view, "ZEILE 39 ") {
		t.Errorf("the popup came back elsewhere:\n%s", view)
	}
}

func TestAMovedReaderKeepsTheSameShareOfTheTextInView(t *testing.T) {
	t.Parallel()
	kept := overlay.Snapshot{
		Source: "Claude Code", Shown: []string{numbered(100)},
		Results: []overlay.SnapshotReply{{Text: strings.ToUpper(numbered(100))}}, Next: 1,
		From: 100, Rows: 200,
	}

	moved := readerAt(t, 80, overlay.ReaderOptions{Resumed: &kept})
	moved.Send(tea.KeyMsg{Type: tea.KeyEsc})
	moved.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	if view := moved.FinalModel(t).View(); !strings.Contains(view, "ZEILE 50 ") {
		t.Errorf("halfway through the text before is not halfway through it now:\n%s", view)
	}
}

func TestAReaderThatFollowedTheEndStillDoesWhereverItGoes(t *testing.T) {
	t.Parallel()
	kept := overlay.Snapshot{
		Source: "Claude Code", Shown: []string{numbered(100)},
		Results: []overlay.SnapshotReply{{Text: strings.ToUpper(numbered(100))}}, Next: 1,
		From: 3, Rows: 50, AtEnd: true,
	}

	moved := readerAt(t, 120, overlay.ReaderOptions{Resumed: &kept})
	moved.Send(tea.KeyMsg{Type: tea.KeyEsc})
	moved.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	if view := moved.FinalModel(t).View(); !strings.Contains(view, "ZEILE 99") {
		t.Errorf("the end is not in view:\n%s", view)
	}
}

// laterReplies hands out one set of replies until it is told the agent has
// answered, and the next set after.
type laterReplies struct {
	mu       sync.Mutex
	replies  []string
	answered []string
	reads    int
}

func (l *laterReplies) Latest(_ context.Context, n int) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reads++
	found := l.replies
	if len(found) > n {
		found = found[len(found)-n:]
	}
	return found, nil
}

func (l *laterReplies) answer() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.replies = l.answered
}

func TestAPromptStillBeingAnsweredIsWaitedForNotTakenForTheOneBefore(t *testing.T) {
	t.Parallel()
	replies := &laterReplies{replies: []string{"old answer", ""}, answered: []string{"old answer", "new answer"}}
	translator := &upperTranslator{}

	reader := newReader(t, replies, translator, overlay.ReaderOptions{Replies: 1, Context: 1})
	waitForText(t, reader, "waiting for the agent to answer")
	replies.answer()
	waitForText(t, reader, "NEW ANSWER")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 1 || told[0] != "old answer" {
		t.Errorf("the service was told %q, want the new answer only, after the old one", told)
	}
	if final := reader.FinalModel(t).View(); strings.Contains(final, "OLD ANSWER") {
		t.Errorf("the reply before was shown as the answer:\n%s", final)
	}
}

func TestCtrlBPagesUpAndSpacePagesDown(t *testing.T) {
	t.Parallel()
	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "line"
	}
	lines[59] = "the very end"

	reader := newReader(t, &fixedReplies{replies: []string{strings.Join(lines, "\n")}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "THE VERY END", "100%")
	reader.Send(tea.KeyMsg{Type: tea.KeyCtrlB})
	teatest.WaitFor(t, reader.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("LINE")) && !bytes.Contains(out, []byte("THE VERY END"))
	}, teatest.WithDuration(3*time.Second))
	reader.Send(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	waitForText(t, reader, "THE VERY END", "100%")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

// clipboard keeps what was copied to it.
type clipboard struct {
	mu   sync.Mutex
	kept []string
}

func (c *clipboard) copy(text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kept = append(c.kept, text)
	return nil
}

func (c *clipboard) copied() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.kept...)
}

func TestYCopiesWhatIsOnShowWholeTheTranslationOrTheOriginal(t *testing.T) {
	t.Parallel()
	board := &clipboard{}

	reader := newReader(t, &fixedReplies{replies: []string{"first answer", "second answer"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 2, Copy: board.copy})
	waitForText(t, reader, "FIRST ANSWER", "SECOND ANSWER", "y copy")
	reader.Send(tea.KeyMsg{Runes: []rune("y"), Type: tea.KeyRunes})
	waitForText(t, reader, "copied")
	reader.Send(tea.KeyMsg{Type: tea.KeyTab})
	reader.Send(tea.KeyMsg{Runes: []rune("y"), Type: tea.KeyRunes})
	teatest.WaitFor(t, reader.Output(), func([]byte) bool { return len(board.copied()) == 2 },
		teatest.WithDuration(3*time.Second))
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	want := []string{"FIRST ANSWER\n\nSECOND ANSWER", "first answer\n\nsecond answer"}
	if got := board.copied(); !slices.Equal(got, want) {
		t.Errorf("copied %q, want %q", got, want)
	}
}

func TestACopyThatFailsSaysWhy(t *testing.T) {
	t.Parallel()

	reader := newReader(t, &fixedReplies{replies: []string{"answer"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1, Copy: func(string) error { return errors.New("no clipboard tool found") }})
	waitForText(t, reader, "ANSWER")
	reader.Send(tea.KeyMsg{Runes: []rune("y"), Type: tea.KeyRunes})
	waitForText(t, reader, "no clipboard tool found")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

// keptHistory is a history in memory, in the reader's own words.
type keptHistory struct {
	mu      sync.Mutex
	entries []overlay.HistoryEntry
}

func (k *keptHistory) Entries() []overlay.HistoryEntry {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]overlay.HistoryEntry(nil), k.entries...)
}

func (k *keptHistory) Find(original string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	for index := len(k.entries) - 1; index >= 0; index-- {
		if k.entries[index].Original == original {
			return k.entries[index].Translation, true
		}
	}
	return "", false
}

func (k *keptHistory) Remember(original, translation string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.entries = append(k.entries, overlay.HistoryEntry{Original: original, Translation: translation, At: time.Now()})
}

func (k *keptHistory) originals() []string {
	var got []string
	for _, kept := range k.Entries() {
		got = append(got, kept.Original)
	}
	return got
}

func TestAReplyAlreadyTranslatedIsShownFromTheHistoryWithoutAskingTheService(t *testing.T) {
	t.Parallel()
	kept := &keptHistory{entries: []overlay.HistoryEntry{{Original: "known answer", Translation: "已知的回答"}}}
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{replies: []string{"known answer"}}, translator,
		overlay.ReaderOptions{Replies: 1, History: kept})
	waitForText(t, reader, "已知的回答")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 0 {
		t.Errorf("the service was asked %d times, want not at all", len(told))
	}
}

func TestTryingAgainTranslatesAReplyTheHistoryHasAndKeepsTheNewTranslation(t *testing.T) {
	t.Parallel()
	kept := &keptHistory{entries: []overlay.HistoryEntry{{Original: "known answer", Translation: "stale"}}}
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{replies: []string{"known answer"}}, translator,
		overlay.ReaderOptions{Replies: 1, History: kept, InPane: true})
	waitForText(t, reader, "stale")
	reader.Send(overlay.RefreshAsked())
	waitForText(t, reader, "KNOWN ANSWER")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if found, _ := kept.Find("known answer"); found != "KNOWN ANSWER" {
		t.Errorf("the history holds %q, want the new translation", found)
	}
}

func TestATranslatedReplyIsKeptButTheTerminalIsNot(t *testing.T) {
	t.Parallel()

	kept := &keptHistory{}
	reader := newReader(t, &fixedReplies{replies: []string{"an answer"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1, History: kept})
	waitForText(t, reader, "t translate")
	reader.Send(translateKey)
	waitForText(t, reader, "AN ANSWER")
	teatest.WaitFor(t, reader.Output(), func([]byte) bool { return len(kept.Entries()) == 1 },
		teatest.WithDuration(3*time.Second))
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	if got := kept.originals(); !slices.Equal(got, []string{"an answer"}) {
		t.Errorf("the history holds %q, want the reply", got)
	}

	terminal := &keptHistory{}
	reader = newReader(t, &fixedReplies{replies: []string{"$ ls"}}, &upperTranslator{},
		overlay.ReaderOptions{Source: "terminal", Service: "deepl", Language: "DE", Whole: true, History: terminal})
	waitForText(t, reader, "t translate")
	reader.Send(translateKey)
	waitForText(t, reader, "$ LS")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	if got := terminal.originals(); len(got) != 0 {
		t.Errorf("the terminal's output was kept: %q", got)
	}
}

func TestBracketsStepThroughTheHistoryAndBackToTheLatest(t *testing.T) {
	t.Parallel()
	kept := &keptHistory{entries: []overlay.HistoryEntry{
		{Original: "first", Translation: "第一", At: time.Now()},
		{Original: "second", Translation: "第二", At: time.Now()},
	}}
	board := &clipboard{}
	older := tea.KeyMsg{Runes: []rune("["), Type: tea.KeyRunes}
	newer := tea.KeyMsg{Runes: []rune("]"), Type: tea.KeyRunes}

	reader := newReader(t, &fixedReplies{replies: []string{"latest"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1, History: kept, Copy: board.copy})
	waitForText(t, reader, "t translate")
	reader.Send(translateKey)
	waitForText(t, reader, "LATEST", "[ ] history")
	reader.Send(older)
	// The latest reply is kept too, but it is on show already: the history
	// starts before it.
	waitForText(t, reader, "第二", "history 2/2")
	reader.Send(older)
	waitForText(t, reader, "第一", "history 1/2")
	reader.Send(older)
	reader.Send(tea.KeyMsg{Type: tea.KeyTab})
	waitForText(t, reader, "first", "original")
	reader.Send(tea.KeyMsg{Runes: []rune("y"), Type: tea.KeyRunes})
	teatest.WaitFor(t, reader.Output(), func([]byte) bool { return len(board.copied()) == 1 },
		teatest.WithDuration(3*time.Second))
	reader.Send(tea.KeyMsg{Type: tea.KeyTab})
	reader.Send(newer)
	reader.Send(newer)
	reader.Send(newer)
	waitForText(t, reader, "1 reply")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if got := board.copied(); !slices.Equal(got, []string{"first"}) {
		t.Errorf("copied %q, want the oldest reply as written", got)
	}
	final := reader.FinalModel(t).View()
	if !strings.Contains(final, "LATEST") || strings.Contains(final, "/2") {
		t.Errorf("] past the newest did not come back to the latest reply:\n%s", final)
	}
}

func TestWithoutAHistoryTheBracketsDoNothing(t *testing.T) {
	t.Parallel()

	reader := newReader(t, &fixedReplies{replies: []string{"answer"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1})
	waitForText(t, reader, "ANSWER")
	reader.Send(tea.KeyMsg{Runes: []rune("["), Type: tea.KeyRunes})
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	final := reader.FinalModel(t).View()
	if strings.Contains(final, "history") || !strings.Contains(final, "ANSWER") {
		t.Errorf("a reader with no history moved away from the reply:\n%s", final)
	}
}

var translateKey = tea.KeyMsg{Runes: []rune("t"), Type: tea.KeyRunes}

func TestANewReplyWaitsForTOnTheLatestTranslationKept(t *testing.T) {
	t.Parallel()
	kept := &keptHistory{entries: []overlay.HistoryEntry{
		{Original: "older", Translation: "较早的", At: time.Now()},
		{Original: "last", Translation: "上一个", At: time.Now()},
	}}
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{replies: []string{"new answer"}}, translator,
		overlay.ReaderOptions{Replies: 1, History: kept})
	waitForText(t, reader, "上一个", "history 2/2", "new reply", "t translate")
	if told := translator.told(); len(told) != 0 {
		t.Fatalf("the service was asked %d times before t, want not at all", len(told))
	}
	reader.Send(translateKey)
	waitForText(t, reader, "NEW ANSWER", "1 reply")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 1 {
		t.Errorf("the service was asked %d times, want once", len(told))
	}
}

func TestWithNothingKeptYetANewReplyStillWaitsForT(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{}

	reader := newReader(t, &fixedReplies{replies: []string{"first ever"}}, translator,
		overlay.ReaderOptions{Replies: 1, History: &keptHistory{}})
	waitForText(t, reader, "a new reply is waiting", "t translate")
	reader.Send(tea.KeyMsg{Type: tea.KeyTab})
	waitForText(t, reader, "first ever")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 0 {
		t.Errorf("the service was asked %d times, want not at all", len(told))
	}
}

func TestAWaitingReplyStillWaitsWhenThePopupComesBack(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{}
	var saved overlay.Snapshot

	first := newReader(t, &fixedReplies{replies: []string{"not yet"}}, translator, overlay.ReaderOptions{
		Replies: 1, History: &keptHistory{},
		Hide: func(snapshot overlay.Snapshot) error { saved = snapshot; return nil },
	})
	waitForText(t, first, "t translate")
	first.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	first.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	again := newReader(t, &fixedReplies{}, translator,
		overlay.ReaderOptions{Replies: 1, History: &keptHistory{}, Resumed: &saved})
	waitForText(t, again, "a new reply is waiting")
	again.Send(tea.KeyMsg{Type: tea.KeyEsc})
	again.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 0 {
		t.Errorf("the service was asked %d times, want not at all", len(told))
	}
}

func TestThePaneKeyTranslatesAWaitingReplyWithoutT(t *testing.T) {
	t.Parallel()

	reader := newReader(t, &fixedReplies{replies: []string{"waiting"}}, &upperTranslator{},
		overlay.ReaderOptions{Replies: 1, History: &keptHistory{}, InPane: true})
	waitForText(t, reader, "t translate")
	reader.Send(overlay.RefreshAsked())
	waitForText(t, reader, "WAITING")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
}

// wholeSession is a session that can also hand out every reply it holds.
type wholeSession struct {
	fixedReplies
	turns []overlay.Turn
}

func (w *wholeSession) All(context.Context) ([]overlay.Turn, error) {
	return w.turns, nil
}

func sessionOf(turns ...overlay.Turn) *wholeSession {
	session := &wholeSession{turns: turns}
	for _, turn := range turns {
		session.replies = append(session.replies, turn.Reply)
	}
	return session
}

var listKey = tea.KeyMsg{Runes: []rune("l"), Type: tea.KeyRunes}

func threeTurns() *wholeSession {
	return sessionOf(
		overlay.Turn{Prompt: "<command-name>/plan</command-name>\n<command-args>next step</command-args>", Reply: "plan reply"},
		overlay.Turn{Prompt: "fix it", Reply: "fixed it\nin two places"},
		overlay.Turn{Prompt: "thanks", Reply: "welcome"},
	)
}

func TestLListsEveryReplyOfTheSessionWithThePromptItAnswered(t *testing.T) {
	t.Parallel()
	kept := &keptHistory{entries: []overlay.HistoryEntry{{Original: "fixed it\nin two places", Translation: "修好了"}}}

	reader := newReader(t, threeTurns(), &upperTranslator{}, overlay.ReaderOptions{Replies: 1, History: kept})
	waitForText(t, reader, "l replies")
	reader.Send(listKey)
	waitForText(t, reader, "pick a reply · 3/3", "/plan next step", "plan reply", "✓ fix it", "fixed it",
		"› ", "enter translate")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if final := reader.FinalModel(t).View(); strings.Contains(final, "pick a reply") {
		t.Errorf("esc left the list open:\n%s", final)
	}
}

func TestAPickedReplyIsTranslatedKnowingTheRepliesBeforeIt(t *testing.T) {
	t.Parallel()
	translator := &upperTranslator{}

	reader := newReader(t, threeTurns(), translator, overlay.ReaderOptions{Replies: 1, Context: 2})
	waitForText(t, reader, "WELCOME")
	reader.Send(listKey)
	waitForText(t, reader, "pick a reply · 3/3")
	reader.Send(tea.KeyMsg{Type: tea.KeyUp})
	reader.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForText(t, reader, "FIXED IT", "reply 2/3")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	told := translator.told()
	if len(told) != 2 || told[1] != "plan reply" {
		t.Errorf("the service was told %q came before, want only the reply before the picked one", told)
	}
}

func TestPickingAReplyTranslatedBeforeAsksTheServiceNothing(t *testing.T) {
	t.Parallel()
	kept := &keptHistory{entries: []overlay.HistoryEntry{
		{Original: "plan reply", Translation: "计划"}, {Original: "welcome", Translation: "不客气"},
	}}
	translator := &upperTranslator{}

	reader := newReader(t, threeTurns(), translator, overlay.ReaderOptions{Replies: 1, History: kept})
	waitForText(t, reader, "不客气")
	reader.Send(listKey)
	waitForText(t, reader, "pick a reply")
	reader.Send(tea.KeyMsg{Runes: []rune("g"), Type: tea.KeyRunes})
	reader.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitForText(t, reader, "计划", "reply 1/3")
	reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
	reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

	if told := translator.told(); len(told) != 0 {
		t.Errorf("the service was asked %d times, want not at all", len(told))
	}
}

func TestWithoutAWholeSessionLDoesNothing(t *testing.T) {
	t.Parallel()

	for name, options := range map[string]overlay.ReaderOptions{
		"a session that cannot list its replies": {Replies: 1},
		"the terminal":                           {Source: "terminal", Service: "deepl", Language: "DE", Whole: true},
	} {
		replies := overlay.Replies(&fixedReplies{replies: []string{"answer"}})
		if options.Whole {
			replies = sessionOf(overlay.Turn{Reply: "answer"})
		}
		reader := newReader(t, replies, &upperTranslator{}, options)
		waitForText(t, reader, "ANSWER")
		reader.Send(listKey)
		reader.Send(tea.KeyMsg{Type: tea.KeyEsc})
		reader.WaitFinished(t, teatest.WithFinalTimeout(time.Second))

		if final := reader.FinalModel(t).View(); strings.Contains(final, "pick a reply") || strings.Contains(final, "l replies") {
			t.Errorf("%s offered the list:\n%s", name, final)
		}
	}
}

func TestAPromptReadsInTheListAsItWasTyped(t *testing.T) {
	t.Parallel()

	cases := []struct{ prompt, want string }{
		{"fix the\n  login bug", "fix the login bug"},
		{
			"<command-name>/plan</command-name>\n<command-message>plan</command-message>\n<command-args>next step</command-args>",
			"/plan next step",
		},
		{
			"<task-notification>\n<task-id>ab24f</task-id>\n<status>completed</status>\n" +
				"<summary>Agent \"Implement fn-148.1\" finished</summary>\n<result>I finished…</result>\n</task-notification>",
			"Agent \"Implement fn-148.1\" finished",
		},
		{"", "#4"},
	}
	for _, c := range cases {
		if got := overlay.PromptLine(c.prompt, 3); got != c.want {
			t.Errorf("PromptLine(%q) is %q, want %q", c.prompt, got, c.want)
		}
	}
}
