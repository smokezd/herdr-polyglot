package overlay_test

import (
	"bytes"
	"context"
	"errors"
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
