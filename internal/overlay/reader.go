package overlay

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Replies are what the agent answered, oldest first. The last one is empty
// while the agent is still working on the latest prompt and has said nothing.
type Replies interface {
	Latest(ctx context.Context, n int) ([]string, error)
}

// History is what was translated before in the same project, oldest first.
// A reply found there is not translated again, and the reader can step back
// through the rest.
type History interface {
	Entries() []HistoryEntry
	// Find is the translation of a reply into the language read in.
	Find(original string) (string, bool)
	Remember(original, translation string)
}

// HistoryEntry is one reply translated before.
type HistoryEntry struct {
	Original    string
	Translation string
	At          time.Time
}

// Progress is a translation as far as it has come: what the service has
// thought so far, if it thinks out loud, and the translation so far.
type Progress struct {
	Thinking string
	Text     string
	// Thought says the thinking is over and the translation has begun.
	Thought bool
}

// ReplyTranslator translates one reply, told what came before it so the same
// thing keeps the same word from one reply to the next. A service that writes
// its answer bit by bit reports each bit to progress; one that answers all at
// once never calls it.
type ReplyTranslator interface {
	TranslateReply(ctx context.Context, text, preceding string, progress func(Progress)) (string, error)
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
	// InPane says the reader is a pane beside the agent rather than a popup
	// over it.
	InPane bool
	// Resumed is what a reader was showing before it moved here. It
	// is shown again as it was, and only a reply left unfinished is translated.
	Resumed *Snapshot
	// Move hands what is on show to a reader opened in the other place.
	// Without it m does nothing.
	Move func(Snapshot) error
	// Hosts and ChooseHost are as for the prompt popup: ctrl+o picks the machine
	// the replies are translated on, and they are translated again there.
	Hosts      Hosts
	ChooseHost func(name string) (ReplyTranslator, string, error)
	// Copy puts text on the clipboard. Without it y does nothing.
	Copy func(text string) error
	// Markdown renders what is shown, the translation or the reply as written,
	// as markdown in MarkdownStyle, one of glamour's; M turns it on and off.
	Markdown      bool
	MarkdownStyle string
	// KeepStyle remembers the style S chose, for the readers after this one.
	KeepStyle func(style string)
	// History keeps each reply translated, and [ and ] step through it.
	// Without it every reply is translated and the keys do nothing.
	History History
}

// Snapshot is what a reader shows, with nothing that ties it to the process
// showing it, so another can take over.
type Snapshot struct {
	Source  string          `json:"source"`
	Whole   bool            `json:"whole"`
	Shown   []string        `json:"shown"`
	Context []string        `json:"context"`
	Results []SnapshotReply `json:"results"`
	// Next is the first reply not translated whole yet.
	Next int `json:"next"`
	// Where the text was scrolled to: From of Rows rows, wrapped at the width
	// it was shown at, or the end, which keeps following what is written.
	From     int  `json:"from"`
	Rows     int  `json:"rows"`
	AtEnd    bool `json:"at_end"`
	Original bool `json:"original"`
	// Held says the replies wait for t to be translated.
	Held bool `json:"held"`
	// Picked names the reply picked from the list, when it is not the latest.
	Picked string `json:"picked,omitempty"`
	// Markdown says what is shown is rendered as markdown, in MarkdownStyle.
	Markdown      bool   `json:"markdown,omitempty"`
	MarkdownStyle string `json:"markdown_style,omitempty"`
}

// SnapshotReply is one reply's translation as far as it had come.
type SnapshotReply struct {
	Text     string        `json:"text"`
	Thinking string        `json:"thinking"`
	Took     time.Duration `json:"took"`
}

// maxContext is how much of what came before travels with a reply. The end of
// it is what matters, and a service reads a long one slowly or not at all.
const maxContext = 4_000

// answerPoll is how often the agent's replies are read again while it has yet
// to answer the latest prompt.
const answerPoll = time.Second

// ReaderHeight is the height to ask herdr for: a reply wants more rows than a
// prompt does.
func ReaderHeight() int {
	return 30 + PopupBorder
}

// Reader shows the agent's latest replies, translated, one after another and
// each as it is written. Nothing here writes or sends anything.
type Reader struct {
	ctx        context.Context
	replies    Replies
	translator ReplyTranslator
	options    ReaderOptions
	// canvas draws the boxes, the scroll bar and the pulse the way the prompt
	// box does. Only its styles and width are ever set.
	canvas Model

	loaded  bool
	shown   []string
	context []string
	results []result
	// next is the reply being translated, or len(shown) once all are done.
	next int
	// attempt tells a try that was given up on from the one that replaced it,
	// and fetch a reading of the replies from the one that replaced it.
	attempt int
	fetch   int
	// stop gives up on the translation under way. It is a closure, so every
	// copy of the model stops the same one.
	stop     context.CancelFunc
	failure  error
	original bool
	from     int
	// follow keeps the end of the text in view while it is being written, once
	// the reader has scrolled down to it. A reply opens at its top: it is read
	// from the start, not from wherever the writing has got to.
	follow bool
	// returnTo is where a reader coming back was scrolled to, kept until the
	// pane's size is known: only then does the text have rows to count.
	returnTo *Snapshot
	pane     tea.WindowSizeMsg
	picker   hostPicker
	// hint is something worth knowing, in the footer until the next key.
	hint string
	// fresh says a translation was asked for again, so the history is not
	// looked in: what it holds is what is being replaced.
	fresh bool
	// held says the replies on show wait for t: a reader opens on what was
	// translated before rather than asking the service unbidden. asked says t
	// or the pane key asked for the latest, so they do not wait.
	held  bool
	asked bool
	// browse is the history being read instead of the replies, and at the
	// entry on show. Without it the replies are.
	browse []HistoryEntry
	at     int
	// turns is the list of the session's replies l opens, and picked names
	// the one picked from it while that is on show rather than the latest.
	turns  turnPicker
	picked string
	// markdown renders what is shown with render, which keeps its renderings
	// for every copy of the model.
	markdown bool
	render   *markdown
}

// result is one reply's translation as far as it has come.
type result struct {
	text     string
	thinking string
	thought  bool
	// began times the thinking, and took is how long it went on, for the line
	// it collapses into.
	began time.Time
	took  time.Duration
}

func NewReader(ctx context.Context, replies Replies, translator ReplyTranslator, options ReaderOptions) Reader {
	if options.Replies < 1 {
		options.Replies = 1
	}
	canvas := Model{styles: newStyles(), options: Options{Live: true, Pulse: options.Pulse}}
	canvas.width = maxContentWidth
	reader := Reader{
		ctx:        ctx,
		replies:    replies,
		translator: translator,
		options:    options,
		canvas:     canvas,
		failure:    options.Trouble,
		stop:       func() {},
		markdown:   options.Markdown,
		render:     newMarkdown(options.MarkdownStyle),
	}
	if saved := options.Resumed; saved != nil {
		reader.loaded = true
		reader.options.Source, reader.options.Whole = saved.Source, saved.Whole
		reader.shown, reader.context = saved.Shown, saved.Context
		reader.results = make([]result, len(saved.Shown))
		for index, kept := range saved.Results {
			if index < len(reader.results) {
				reader.results[index] = result{
					text: kept.Text, thinking: kept.Thinking, thought: true, took: kept.Took,
				}
			}
		}
		reader.next = min(max(saved.Next, 0), len(saved.Shown))
		reader.original, reader.held, reader.picked = saved.Original, saved.Held, saved.Picked
		reader.markdown = saved.Markdown
		if saved.MarkdownStyle != "" {
			reader.render = newMarkdown(saved.MarkdownStyle)
		}
		// Until the pane's size says where that is, the reader comes back
		// where it was rather than at the end.
		reader.follow = false
		reader.returnTo = saved
	}
	return reader
}

// Snapshot is what the reader shows now. A reply still being translated is
// left out, and translated again by whichever reader takes over.
func (r Reader) Snapshot() Snapshot {
	saved := Snapshot{
		Source: r.options.Source, Whole: r.options.Whole,
		Shown: r.shown, Context: r.context, Next: r.next,
		From: r.from, Rows: r.total(), AtEnd: r.follow, Original: r.original, Held: r.held,
		Picked: r.picked, Markdown: r.markdown, MarkdownStyle: r.render.Style(),
	}
	for index := range r.shown {
		kept := SnapshotReply{}
		if index < r.next {
			result := r.results[index]
			kept = SnapshotReply{Text: result.text, Thinking: result.thinking, Took: result.took}
		}
		saved.Results = append(saved.Results, kept)
	}
	return saved
}

type (
	repliesMsg struct {
		fetch   int
		replies []string
		err     error
	}
	// What a reader is told from outside, by a key pressed elsewhere.
	moveAskedMsg    struct{}
	refreshAskedMsg struct{}
	// waitedMsg is time to look again for an answer the agent has yet to give.
	waitedMsg struct{ fetch int }
	// copiedMsg is the clipboard's answer to y.
	copiedMsg struct {
		lines int
		err   error
	}
	// resumeMsg goes on with a reply a reader before this one left unfinished.
	resumeMsg struct{}
	// replyProgressMsg carries the channels it came from, so the next piece is
	// listened for where this one arrived.
	replyProgressMsg struct {
		attempt, index int
		so             Progress
		pieces, done   <-chan tea.Msg
	}
	replyTranslatedMsg struct {
		attempt, index int
		text           string
		err            error
		// found says it came from the history, which need not keep it again.
		found bool
	}
)

// MoveAsked and RefreshAsked are what a running reader is told to
// do by a key pressed outside it.
func MoveAsked() tea.Msg    { return moveAskedMsg{} }
func RefreshAsked() tea.Msg { return refreshAskedMsg{} }

func (r Reader) Init() tea.Cmd {
	if r.options.Resumed != nil {
		if r.waiting() {
			return r.fetchReplies()
		}
		if r.next < len(r.shown) && r.options.Trouble == nil && !r.held {
			// Starting changes the model, which Init cannot keep.
			return func() tea.Msg { return resumeMsg{} }
		}
		return nil
	}
	return r.fetchReplies()
}

func (r Reader) fetchReplies() tea.Cmd {
	ctx, replies, fetch := r.ctx, r.replies, r.fetch
	wanted := r.options.Replies + r.options.Context
	return func() tea.Msg {
		found, err := replies.Latest(ctx, wanted)
		return repliesMsg{fetch: fetch, replies: found, err: err}
	}
}

// waiting says the agent has yet to answer the latest prompt. The reply before
// it is not the answer, so nothing is translated until there is one.
func (r Reader) waiting() bool {
	return r.loaded && len(r.shown) > 0 && r.shown[len(r.shown)-1] == ""
}

func (r Reader) waitForAnswer() tea.Cmd {
	fetch := r.fetch
	return tea.Tick(answerPoll, func(time.Time) tea.Msg { return waitedMsg{fetch: fetch} })
}

// refresh drops what is on show and reads the agent's latest replies again,
// each translated anew.
func (r Reader) refresh() (tea.Model, tea.Cmd) {
	return r.reread(true)
}

// latest is t: the agent's latest replies, read again so a reader left open
// has the ones written since, and translated. One kept already is shown from
// the history; only the new ones go to the service.
func (r Reader) latest() (tea.Model, tea.Cmd) {
	return r.reread(false)
}

// reread drops what is on show and reads the agent's latest replies again, to
// be translated as soon as they are read: fresh asks for every one of them
// anew. Either way it was asked for, so nothing waits for t.
func (r Reader) reread(fresh bool) (tea.Model, tea.Cmd) {
	r.stop()
	r.attempt++
	r.fetch++
	r.loaded, r.shown, r.context, r.results, r.next = false, nil, nil, nil, 0
	r.failure, r.original = r.options.Trouble, false
	r.toTop()
	r.fresh, r.asked, r.held, r.browse, r.picked = fresh, true, false, nil, ""
	r.canvas.endPulse()
	fetching := r.fetchReplies()
	return r, fetching
}

// handOn gives what is on show to whoever keeps it, and closes this reader.
// If it cannot be kept the reader stays, and says why.
func (r Reader) handOn(keep func(Snapshot) error) (tea.Model, tea.Cmd) {
	if keep == nil {
		return r, nil
	}
	if err := keep(r.Snapshot()); err != nil {
		r.failure = err
		return r, nil
	}
	r.stop()
	return r, tea.Quit
}

func (r Reader) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		r.pane = msg
		r.canvas.width = max(msg.Width-draftFrame, 1)
		if saved := r.returnTo; saved != nil {
			r.returnTo = nil
			r.scrollBack(saved)
		}
		r.keepInView()
		return r, nil

	case tea.KeyMsg:
		return r.handleKey(msg)

	case hostProbedMsg:
		r.picker = r.picker.probed(msg)
		return r, nil
	case turnsMsg:
		return r.turnsListed(msg)

	case moveAskedMsg:
		return r.handOn(r.options.Move)
	case refreshAskedMsg:
		return r.refresh()
	case copiedMsg:
		if msg.err != nil {
			r.hint = "✗ not copied: " + msg.err.Error()
			return r, nil
		}
		r.hint = "copied " + plural(msg.lines, "line", "lines")
		return r, nil

	case waitedMsg:
		if msg.fetch != r.fetch || !r.waiting() {
			return r, nil
		}
		return r, r.fetchReplies()
	case resumeMsg:
		started := r.startTranslating()
		return r, started

	case repliesMsg:
		if msg.fetch != r.fetch {
			return r, nil
		}
		r.loaded = true
		if msg.err != nil {
			r.failure = msg.err
			return r, nil
		}
		r.shown, r.context = r.split(msg.replies)
		r.results = make([]result, len(r.shown))
		if r.waiting() {
			return r, r.waitForAnswer()
		}
		if len(r.shown) == 0 || r.options.Trouble != nil {
			return r, nil
		}
		if r.held = r.holds(); r.held {
			if !r.browsing() {
				r.step(-1)
			}
			return r, nil
		}
		started := r.startTranslating()
		return r, started

	case replyProgressMsg:
		if msg.attempt != r.attempt {
			return r, nil
		}
		r.results[msg.index] = r.results[msg.index].advanced(msg.so, time.Now())
		r.keepInView()
		return r, listen(msg.pieces, msg.done)

	case replyTranslatedMsg:
		if msg.attempt != r.attempt || errors.Is(msg.err, context.Canceled) {
			return r, nil
		}
		if msg.err != nil {
			r.failure = msg.err
			r.canvas.endPulse()
			r.keepInView()
			return r, nil
		}
		r.results[msg.index] = r.results[msg.index].finished(msg.text, time.Now())
		r.next++
		r.keepInView()
		kept := r.remember(msg)
		if r.next == len(r.shown) {
			r.canvas.endPulse()
			return r, kept
		}
		next := r.translate(r.next)
		return r, tea.Batch(kept, next)

	case pulseMsg:
		if !r.canvas.pulsing || msg.beat <= r.canvas.beat {
			return r, nil
		}
		beating := r.canvas.keepBreathing(msg.beat)
		return r, beating
	}
	return r, nil
}

func (s result) advanced(so Progress, now time.Time) result {
	if so.Thinking != "" && s.began.IsZero() {
		s.began = now
	}
	if so.Thought && !s.thought && !s.began.IsZero() {
		s.took = now.Sub(s.began)
	}
	s.thinking, s.text, s.thought = so.Thinking, so.Text, s.thought || so.Thought
	return s
}

// finished takes the whole translation, which has had its code put back and
// was checked; what streamed in was only ever a preview of it.
func (s result) finished(text string, now time.Time) result {
	if !s.began.IsZero() && !s.thought {
		s.took = now.Sub(s.began)
	}
	s.text, s.thought = text, true
	return s
}

// holds says the replies on show wait for t: some of them would go to the
// service, and nobody asked for that yet. A reader with no history, or one
// told to translate again, does not wait.
func (r Reader) holds() bool {
	if r.options.History == nil || r.fresh || r.asked {
		return false
	}
	if r.options.Whole {
		return true
	}
	for _, reply := range r.shown {
		if _, found := r.options.History.Find(reply); !found {
			return true
		}
	}
	return false
}

// remember keeps a reply's translation for the next reader in the project.
// The terminal's output is not a reply, and is never the same twice.
func (r Reader) remember(msg replyTranslatedMsg) tea.Cmd {
	if r.options.History == nil || r.options.Whole || msg.found {
		return nil
	}
	history, original, translation := r.options.History, r.shown[msg.index], msg.text
	return func() tea.Msg {
		history.Remember(original, translation)
		return nil
	}
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
	r.results[r.next] = result{}
	return tea.Batch(r.translate(r.next), r.canvas.beginPulse())
}

// translate starts a reply off and listens for it. Progress keeps only the
// newest piece: each one holds everything before it, so one the screen had no
// time for is not missed.
func (r *Reader) translate(index int) tea.Cmd {
	r.stop()
	r.attempt++
	attempt := r.attempt
	ctx, stop := context.WithCancel(r.ctx)
	r.stop = stop
	text := r.shown[index]
	if r.options.History != nil && !r.fresh && !r.options.Whole {
		if found, ok := r.options.History.Find(text); ok {
			return func() tea.Msg {
				return replyTranslatedMsg{attempt: attempt, index: index, text: found, found: true}
			}
		}
	}
	preceding := lastOf(strings.Join(append(append([]string(nil), r.context...), r.shown[:index]...), "\n\n"),
		maxContext)

	pieces := make(chan tea.Msg, 1)
	done := make(chan tea.Msg, 1)
	newest := func(piece tea.Msg) {
		for {
			select {
			case pieces <- piece:
				return
			default:
				select {
				case <-pieces:
				default:
				}
			}
		}
	}

	translator := r.translator
	go func() {
		translated, err := translator.TranslateReply(ctx, text, preceding, func(so Progress) {
			newest(replyProgressMsg{attempt: attempt, index: index, so: so, pieces: pieces, done: done})
		})
		done <- replyTranslatedMsg{attempt: attempt, index: index, text: translated, err: err}
	}()
	return listen(pieces, done)
}

// listen waits for the next piece of a translation, or for its end. The end
// wins a tie: it holds everything the piece would have said.
func listen(pieces, done <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case end := <-done:
			return end
		default:
		}
		select {
		case end := <-done:
			return end
		case piece := <-pieces:
			return piece
		}
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
	r.hint = ""
	switch {
	case key.Type == tea.KeyCtrlC:
		return r, tea.Quit

	case r.picker.open:
		return r.pickKey(key)
	case r.turns.open:
		return r.pickTurn(key)

	case key.Type == tea.KeyCtrlO && r.options.Hosts != nil:
		return r.openPicker()

	case key.Type == tea.KeyEsc, key.String() == "q":
		return r, tea.Quit

	case key.String() == "m":
		return r.handOn(r.options.Move)

	case key.String() == "y":
		return r, r.copyShown()

	case key.Type == tea.KeyTab:
		r.original = !r.original
		r.from = r.clamp(r.from)
		return r, nil

	case key.String() == "M":
		r.markdown = !r.markdown
		r.from = r.clamp(r.from)
		return r, nil

	case key.String() == "S":
		kept := r.nextStyle()
		r.from = r.clamp(r.from)
		return r, kept

	case key.Type == tea.KeyCtrlT:
		if r.failure == nil || !r.loaded || r.next >= len(r.shown) || r.options.Trouble != nil {
			return r, nil
		}
		r.fresh = true
		r.toTop()
		started := r.startTranslating()
		return r, started

	case key.String() == "t":
		return r.latest()

	case key.String() == "l":
		return r, r.listTurns()

	case key.String() == "[":
		r.step(-1)
	case key.String() == "]":
		r.step(1)

	case key.Type == tea.KeyUp, key.String() == "k":
		r.scrollTo(r.from - 1)
	case key.Type == tea.KeyDown, key.String() == "j":
		r.scrollTo(r.from + 1)
	case key.Type == tea.KeyPgUp, key.Type == tea.KeyCtrlB:
		r.scrollTo(r.from - rows)
	case key.Type == tea.KeyPgDown, key.Type == tea.KeySpace:
		r.scrollTo(r.from + rows)
	case key.Type == tea.KeyHome, key.String() == "g":
		r.scrollTo(0)
	case key.Type == tea.KeyEnd, key.String() == "G":
		r.scrollTo(r.total())
	}
	return r, nil
}

// step moves through the history: back to an older entry, or forward to a
// newer one and past the newest back to the replies. The replies on show are
// left out of it, being on show already.
func (r *Reader) step(by int) {
	if r.options.History == nil {
		return
	}
	if r.browse == nil {
		if by > 0 {
			return
		}
		for _, entry := range r.options.History.Entries() {
			if !slices.Contains(r.shown, entry.Original) {
				r.browse = append(r.browse, entry)
			}
		}
		if len(r.browse) == 0 {
			r.browse = nil
			return
		}
		r.at = len(r.browse)
	}
	r.at = max(r.at+by, 0)
	if r.at >= len(r.browse) {
		r.browse = nil
	}
	r.toTop()
}

// nextStyle renders markdown in the style after the one in use, and renders
// it at all if it was not, and keeps the choice for the readers after this one.
func (r *Reader) nextStyle() tea.Cmd {
	style := nextStyle(r.render.Style())
	r.render.use(style)
	r.markdown = true
	keep := r.options.KeepStyle
	if keep == nil {
		return nil
	}
	return func() tea.Msg {
		keep(style)
		return nil
	}
}

// toTop shows what opens now from its start.
func (r *Reader) toTop() {
	r.from, r.follow = 0, false
}

func (r Reader) browsing() bool { return r.browse != nil }

// copyShown copies what is on show, every reply of it whole however far it is
// scrolled: the translation as far as it has come, or the replies as the agent
// wrote them.
func (r Reader) copyShown() tea.Cmd {
	if r.options.Copy == nil {
		return nil
	}
	texts := r.onShow()
	if len(texts) == 0 {
		return nil
	}
	whole, copyTo := strings.Join(texts, "\n\n"), r.options.Copy
	return func() tea.Msg {
		return copiedMsg{lines: strings.Count(whole, "\n") + 1, err: copyTo(whole)}
	}
}

// onShow is the text of each reply on show, or of the history entry read
// instead, as translated or as written, leaving out any with nothing in it.
func (r Reader) onShow() []string {
	var texts []string
	keep := func(text string) {
		if text = strings.TrimSpace(text); text != "" {
			texts = append(texts, text)
		}
	}
	if r.browsing() {
		keep(r.entryText())
		return texts
	}
	if !r.loaded || r.waiting() {
		return nil
	}
	for index, reply := range r.shown {
		if !r.original && r.options.Trouble == nil {
			reply = r.results[index].text
		}
		keep(reply)
	}
	return texts
}

// scrollBack puts the view where an earlier reader had it. The same share of
// the text is the same place even when it wraps at another width, as it does
// when it moves between a popup and a pane.
func (r *Reader) scrollBack(saved *Snapshot) {
	if saved.AtEnd {
		r.follow = true
		return
	}
	row := saved.From
	if saved.Rows > 0 {
		row = (saved.From*r.total() + saved.Rows/2) / saved.Rows
	}
	r.scrollTo(row)
}

// scrollTo moves the view, and keeps following the text only if the move left
// its end in view.
func (r *Reader) scrollTo(row int) {
	r.from = r.clamp(row)
	r.follow = r.from >= r.bottom()
}

// keepInView brings the end of the text into view while it is followed, and
// otherwise only keeps the view inside a text that may have got shorter: the
// thinking folds away once the answer begins.
func (r *Reader) keepInView() {
	if r.follow {
		r.from = r.bottom()
		return
	}
	r.from = r.clamp(r.from)
}

func (r Reader) bottom() int {
	return max(r.total()-r.rows(), 0)
}

func (r Reader) clamp(row int) int {
	return min(max(row, 0), r.bottom())
}

func (r Reader) rows() int {
	if r.pane.Height <= 0 {
		return ReaderHeight() - PopupBorder - headerRows - footerRows - draftFrame
	}
	return max(r.pane.Height-headerRows-footerRows-draftFrame, 1)
}

func (r Reader) total() int {
	return len(r.lines())
}

// lines is every reply on show, wrapped to the box and styled line by line:
// a style opened on one line and closed on another would leak into the border.
func (r Reader) lines() []string {
	look := r.canvas.styles
	width := r.canvas.contentWidth()
	var lines []string
	add := func(text string, style lipgloss.Style) {
		for _, line := range strings.Split(wrapped(text, width), "\n") {
			lines = append(lines, style.Render(line))
		}
	}
	// body is a reply or its translation, rendered when markdown is on.
	body := func(text string) {
		if r.markdown {
			lines = append(lines, r.render.lines(text, width)...)
			return
		}
		add(text, look.text)
	}

	switch {
	case r.browsing():
		body(r.entryText())
		return lines
	case !r.loaded:
		add("reading what the agent said …", look.placeholder)
		return lines
	case len(r.shown) == 0:
		add("the agent has not said anything yet", look.placeholder)
		return lines
	case r.waiting():
		add("waiting for the agent to answer …", look.placeholder)
		return lines
	case r.held && !r.original:
		add("a new reply is waiting · t translates it", look.placeholder)
		return lines
	}

	rule := strings.Repeat("─", min(width, 24))
	for index, reply := range r.shown {
		if index > 0 {
			add("", look.text)
			add(rule, look.mark)
			add("", look.text)
		}
		if r.original || r.options.Trouble != nil {
			body(reply)
			continue
		}
		r.addResult(index, reply, add, body)
	}
	return lines
}

// entryText is the history entry on show, as translated or as written.
func (r Reader) entryText() string {
	entry := r.browse[r.at]
	if r.original {
		return entry.Original
	}
	return entry.Translation
}

// addResult shows the thinking while it goes on and a line for it once it is
// over, then the translation as far as it has come.
func (r Reader) addResult(index int, reply string, add func(string, lipgloss.Style), body func(string)) {
	look := r.canvas.styles
	result := r.results[index]
	current := index == r.next

	if result.thinking != "" {
		if result.thought {
			add("▸ "+thoughtFor(result.took), look.faded)
		} else {
			add("▾ thinking …", look.accent)
			add(result.thinking, look.faded)
		}
		if result.text != "" || (current && r.failure == nil && !result.thought) {
			add("", look.text)
		}
	}

	switch {
	case result.text != "" && (index < r.next || r.failure == nil):
		body(result.text)
	case current && r.failure != nil:
		// What did not translate is still worth reading as it was written.
		body(reply)
	case current && result.thinking == "":
		add("translating …", look.placeholder)
	case !current:
		add("…", look.placeholder)
	}
}

func thoughtFor(took time.Duration) string {
	if took < time.Second {
		return "thought for a moment"
	}
	return "thought for " + took.Round(time.Second).String()
}

func (r Reader) View() string {
	if r.picker.open {
		return r.pickerView()
	}
	if r.turns.open {
		return r.turnsView()
	}
	rows := r.rows()
	lines := r.lines()
	total := len(lines)
	from := min(r.from, max(total-rows, 0))

	shown := strings.Join(lines[from:min(from+rows, total)], "\n")
	label := howFarThrough(from, rows, total)
	if r.original {
		label = strings.TrimSpace("original " + label)
	}
	box := r.canvas.labelled(
		r.canvas.box(true).Height(rows).Render(r.canvas.scrolled(shown, from, rows, total)),
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
	said := r.counted()
	switch {
	case r.turns.open && r.turns.previewing:
		said = "preview " + r.turns.position()
	case r.turns.open:
		said = "pick a reply · " + r.turns.position()
	case r.picked != "" && !r.browsing():
		said = r.picked
	}
	if r.browsing() && !r.turns.open {
		said = "history " + strconv.Itoa(r.at+1) + "/" + strconv.Itoa(len(r.browse)) + " · " +
			when(r.browse[r.at].At, time.Now())
		if r.held {
			said += " · new reply"
		}
	}
	pieces = append(pieces, look.badge.Render("· "+said+" · "+r.options.Service))
	return lipgloss.NewStyle().MaxWidth(line - 1).Render(" " + strings.Join(pieces, look.badge.Render(" ")))
}

// when says when an entry was translated: the time on the day it was, and
// the date as well on any other.
func when(at, now time.Time) string {
	if at.IsZero() {
		return "earlier"
	}
	at = at.Local()
	if y, m, d := at.Date(); y == now.Year() && m == now.Month() && d == now.Day() {
		return at.Format("15:04")
	}
	return at.Format("Jan 2 15:04")
}

func (r Reader) working() bool {
	return r.loaded && r.failure == nil && r.next < len(r.shown) && r.options.Trouble == nil && !r.waiting() &&
		!r.held
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
		if choosable(r.options.Hosts) {
			keys = key("ctrl+o", "host") + separator + keys
		}
		if r.options.Trouble == nil && r.loaded && r.next < len(r.shown) {
			keys = key("ctrl+t", "try again") + separator + keys
		}
		said := cutTo("✗ "+r.failure.Error(), max(inner-lipgloss.Width(keys)-2, 1))
		return spread(" "+look.danger.Render(said), keys, inner)
	}
	if r.hint != "" {
		return spread(" "+look.badge.Render(cutTo(r.hint, max(inner-2, 1))), "", inner)
	}
	hints := []string{key("↑↓", "read"), key("tab", r.otherView()), key("M", markdownHint(r.markdown))}
	if r.markdown {
		hints = append(hints, key("S", r.render.Style()))
	}
	if r.held {
		hints = append([]string{key("t", "translate")}, hints...)
	} else {
		hints = append([]string{key("t", "latest")}, hints...)
	}
	if r.options.History != nil {
		hints = append(hints, key("[ ]", "history"))
	}
	if _, ok := r.session(); ok {
		hints = append(hints, key("l", "replies"))
	}
	if r.options.Move != nil {
		hints = append(hints, key("m", "→ "+r.elsewhere()))
	}
	if r.options.Copy != nil {
		hints = append(hints, key("y", "copy"))
	}
	if choosable(r.options.Hosts) {
		hints = append(hints, key("ctrl+o", "host"))
	}
	hints = append(hints, key("esc", "close"))
	return spread(" "+strings.Join(hints, separator), "", inner)
}

func (r Reader) elsewhere() string {
	if r.options.InPane {
		return "popup"
	}
	return "pane"
}

func (r Reader) otherView() string {
	if r.original {
		return "translation"
	}
	return "original"
}
