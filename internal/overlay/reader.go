package overlay

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Replies are what the agent answered, oldest first.
type Replies interface {
	Latest(ctx context.Context, n int) ([]string, error)
}

// ReplyTranslator translates one reply, told what came before it so the same
// thing keeps the same word from one reply to the next.
type ReplyTranslator interface {
	TranslateWithContext(ctx context.Context, text, preceding string) (string, error)
}

type ReaderOptions struct {
	// Source names where the replies come from: an agent, or the terminal.
	Source   string
	Service  string
	Language string
	// Replies is how many of the latest replies are translated and shown,
	// Context how many before them only go along to the service.
	Replies int
	Context int
	// Whole says the source hands out everything as one reply, so there is no
	// counting them.
	Whole bool
	// Trouble is a reason nothing can be translated, said as soon as the popup
	// opens. The replies are still shown, as they were written.
	Trouble error
	Pulse   bool
}

// maxContext is how much of what came before travels with a reply. The end of
// it is what matters, and a service reads a long one slowly or not at all.
const maxContext = 4_000

// ReaderHeight is the height to ask herdr for: a reply wants more rows than a
// prompt does.
func ReaderHeight() int {
	return 30 + PopupBorder
}

// Reader shows the agent's latest replies, translated, one after another as
// each is done. Nothing here writes or sends anything.
type Reader struct {
	ctx        context.Context
	replies    Replies
	translator ReplyTranslator
	options    ReaderOptions
	// canvas draws the boxes, the scroll bar and the pulse the way the prompt
	// box does. Only its styles and width are ever set.
	canvas Model

	loaded     bool
	shown      []string
	context    []string
	translated []string
	// next is the reply being translated, or len(shown) once all are done.
	next     int
	failure  error
	original bool
	from     int
	pane     tea.WindowSizeMsg
}

func NewReader(ctx context.Context, replies Replies, translator ReplyTranslator, options ReaderOptions) Reader {
	if options.Replies < 1 {
		options.Replies = 1
	}
	canvas := Model{styles: newStyles(), options: Options{Live: true, Pulse: options.Pulse}}
	canvas.width = maxContentWidth
	return Reader{
		ctx:        ctx,
		replies:    replies,
		translator: translator,
		options:    options,
		canvas:     canvas,
		failure:    options.Trouble,
	}
}

type (
	repliesMsg struct {
		replies []string
		err     error
	}
	replyTranslatedMsg struct {
		index int
		text  string
		err   error
	}
)

func (r Reader) Init() tea.Cmd {
	return func() tea.Msg {
		replies, err := r.replies.Latest(r.ctx, r.options.Replies+r.options.Context)
		return repliesMsg{replies: replies, err: err}
	}
}

func (r Reader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.pane = msg
		r.canvas.width = min(max(msg.Width-draftFrame, 1), maxContentWidth)
		r.from = r.clamp(r.from)
		return r, nil

	case tea.KeyMsg:
		return r.handleKey(msg)

	case repliesMsg:
		r.loaded = true
		if msg.err != nil {
			r.failure = msg.err
			return r, nil
		}
		r.shown, r.context = r.split(msg.replies)
		r.translated = make([]string, len(r.shown))
		if len(r.shown) == 0 || r.options.Trouble != nil {
			return r, nil
		}
		started := r.startTranslating()
		return r, started

	case replyTranslatedMsg:
		if errors.Is(msg.err, context.Canceled) || msg.index != r.next {
			return r, nil
		}
		if msg.err != nil {
			r.failure = msg.err
			r.canvas.endPulse()
			return r, nil
		}
		r.translated[msg.index] = msg.text
		r.next++
		if r.next == len(r.shown) {
			r.canvas.endPulse()
			return r, nil
		}
		return r, r.translate(r.next)

	case pulseMsg:
		if !r.canvas.pulsing || msg.beat <= r.canvas.beat {
			return r, nil
		}
		beating := r.canvas.keepBreathing(msg.beat)
		return r, beating
	}
	return r, nil
}

// split keeps the latest replies to show and the ones before them to send
// along. A source that cannot count replies has only the one to show.
func (r Reader) split(replies []string) (shown, before []string) {
	if r.options.Whole {
		return replies, nil
	}
	cut := max(len(replies)-r.options.Replies, 0)
	return replies[cut:], replies[:cut]
}

func (r *Reader) startTranslating() tea.Cmd {
	r.failure = nil
	return tea.Batch(r.translate(r.next), r.canvas.beginPulse())
}

func (r Reader) translate(index int) tea.Cmd {
	text := r.shown[index]
	preceding := lastOf(strings.Join(append(append([]string(nil), r.context...), r.shown[:index]...), "\n\n"),
		maxContext)
	return func() tea.Msg {
		translated, err := r.translator.TranslateWithContext(r.ctx, text, preceding)
		return replyTranslatedMsg{index: index, text: translated, err: err}
	}
}

// lastOf keeps the end of text, from a line break where there is one, so the
// context does not start halfway through a word.
func lastOf(text string, most int) string {
	runes := []rune(text)
	if len(runes) <= most {
		return text
	}
	cut := string(runes[len(runes)-most:])
	if _, after, found := strings.Cut(cut, "\n"); found {
		return after
	}
	return cut
}

func (r Reader) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := r.rows()
	switch {
	case key.Type == tea.KeyEsc, key.Type == tea.KeyCtrlC, key.String() == "q":
		return r, tea.Quit

	case key.Type == tea.KeyTab:
		r.original = !r.original
		r.from = r.clamp(r.from)
		return r, nil

	case key.Type == tea.KeyCtrlT:
		if r.failure == nil || !r.loaded || r.next >= len(r.shown) || r.options.Trouble != nil {
			return r, nil
		}
		started := r.startTranslating()
		return r, started

	case key.Type == tea.KeyUp, key.String() == "k":
		r.from = r.clamp(r.from - 1)
	case key.Type == tea.KeyDown, key.String() == "j":
		r.from = r.clamp(r.from + 1)
	case key.Type == tea.KeyPgUp:
		r.from = r.clamp(r.from - rows)
	case key.Type == tea.KeyPgDown, key.Type == tea.KeySpace:
		r.from = r.clamp(r.from + rows)
	case key.Type == tea.KeyHome, key.String() == "g":
		r.from = 0
	case key.Type == tea.KeyEnd, key.String() == "G":
		r.from = r.clamp(r.total())
	}
	return r, nil
}

func (r Reader) clamp(row int) int {
	return min(max(row, 0), max(r.total()-r.rows(), 0))
}

func (r Reader) rows() int {
	if r.pane.Height <= 0 {
		return ReaderHeight() - PopupBorder - headerRows - footerRows - draftFrame
	}
	return max(r.pane.Height-headerRows-footerRows-draftFrame, 1)
}

func (r Reader) total() int {
	return rowsOf(r.text(), r.canvas.contentWidth())
}

// text is every reply on show, one after the other, each in its translation
// once it has one. The text is plain here: it is wrapped before it is styled.
func (r Reader) text() string {
	switch {
	case !r.loaded:
		return "reading what the agent said …"
	case len(r.shown) == 0:
		return "the agent has not said anything yet"
	}

	rule := strings.Repeat("─", min(r.canvas.contentWidth(), 24))
	parts := make([]string, len(r.shown))
	for index, reply := range r.shown {
		switch {
		case r.original, r.options.Trouble != nil:
			parts[index] = reply
		case index < r.next:
			parts[index] = r.translated[index]
		case index == r.next && r.failure == nil:
			parts[index] = "translating …"
		case index == r.next:
			// What did not translate is still worth reading as it was written.
			parts[index] = reply
		default:
			parts[index] = "…"
		}
	}
	return strings.Join(parts, "\n\n"+rule+"\n\n")
}

func (r Reader) View() string {
	rows := r.rows()
	total := r.total()
	width := r.canvas.contentWidth()

	shown := r.canvas.styles.text.Render(rowsFrom(r.text(), width, r.from, rows))
	label := howFarThrough(r.from, rows, total)
	if r.original {
		label = strings.TrimSpace("original " + label)
	}
	box := r.canvas.labelled(
		r.canvas.box(true).Height(rows).Render(r.canvas.scrolled(shown, r.from, rows, total)),
		label, true)
	line := lipgloss.Width(box)

	return strings.Join([]string{r.header(line), box, r.footer(line - 1)}, "\n")
}

func (r Reader) header(line int) string {
	look := r.canvas.styles
	pieces := []string{look.badge.Render(r.options.Source + " → " + r.options.Language)}
	if r.working() {
		pieces = append(pieces, r.canvas.pulseGlyph())
	}
	pieces = append(pieces, look.badge.Render("· "+r.counted()+" · "+r.options.Service))
	return lipgloss.NewStyle().MaxWidth(line - 1).Render(" " + strings.Join(pieces, look.badge.Render(" ")))
}

func (r Reader) working() bool {
	return r.loaded && r.failure == nil && r.next < len(r.shown) && r.options.Trouble == nil
}

// counted says how much is on show and how much went along unseen.
func (r Reader) counted() string {
	if r.options.Whole {
		return "recent output"
	}
	said := plural(r.options.Replies, "reply", "replies")
	if r.options.Context > 0 {
		said += ", " + strconv.Itoa(r.options.Context) + " before as context"
	}
	return said
}

func plural(count int, one, many string) string {
	if count == 1 {
		return "1 " + one
	}
	return strconv.Itoa(count) + " " + many
}

func (r Reader) footer(inner int) string {
	look := r.canvas.styles
	key := func(name, does string) string { return look.key.Render(name) + look.hint.Render(" "+does) }
	separator := look.hint.Render(" · ")

	// Beside a failure there is room only for the keys that answer it.
	if r.failure != nil {
		keys := key("esc", "close")
		if r.options.Trouble == nil && r.loaded && r.next < len(r.shown) {
			keys = key("ctrl+t", "try again") + separator + keys
		}
		said := cutTo("✗ "+r.failure.Error(), max(inner-lipgloss.Width(keys)-2, 1))
		return spread(" "+look.danger.Render(said), keys, inner)
	}
	return spread(" "+strings.Join([]string{
		key("↑↓", "read"), key("tab", r.otherView()), key("esc", "close"),
	}, separator), "", inner)
}

func (r Reader) otherView() string {
	if r.original {
		return "translation"
	}
	return "original"
}
