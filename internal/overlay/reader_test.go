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
