package overlay

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Turn is one reply and the prompt it answered.
type Turn struct {
	Prompt string
	Reply  string
}

// Session is a source of replies that can hand out every one of them, oldest
// first, so one can be picked to read. The terminal cannot: it has no turns.
type Session interface {
	All(ctx context.Context) ([]Turn, error)
}

// turnPicker is the list l opens: every reply of the session under the prompt
// it answered, oldest at the top. It takes over the box the way the host list
// does, and leaves what is on show as it was until a reply is picked.
type turnPicker struct {
	open   bool
	turns  []Turn
	cursor int
	// kept says which replies the history has a translation of already, so
	// picking them costs nothing.
	kept []bool
	// previewing shows the turn at the cursor whole, prompt and reply as they
	// were written, scrolled to from.
	previewing bool
	from       int
}

type turnsMsg struct {
	turns []Turn
	err   error
}

// show opens the list on the newest reply.
func (p turnPicker) show(turns []Turn, history History) turnPicker {
	p.open, p.turns, p.cursor = true, turns, len(turns)-1
	p.kept = make([]bool, len(turns))
	if history != nil {
		for index, turn := range turns {
			_, p.kept[index] = history.Find(turn.Reply)
		}
	}
	return p
}

// key moves through the list, or ends it: picked says a reply was chosen, the
// one at the cursor.
func (p turnPicker) key(key tea.KeyMsg, page int) (picker turnPicker, picked bool) {
	last := len(p.turns) - 1
	switch {
	case key.Type == tea.KeyEsc:
		p.open = false
	case key.Type == tea.KeyEnter:
		p.open = false
		return p, true
	case key.Type == tea.KeyRight, key.String() == "p":
		p.previewing, p.from = true, 0
	case key.Type == tea.KeyUp, key.String() == "k":
		p.cursor--
	case key.Type == tea.KeyDown, key.String() == "j":
		p.cursor++
	case key.Type == tea.KeyPgUp, key.Type == tea.KeyCtrlB:
		p.cursor -= page
	case key.Type == tea.KeyPgDown, key.Type == tea.KeySpace:
		p.cursor += page
	case key.Type == tea.KeyHome, key.String() == "g":
		p.cursor = 0
	case key.Type == tea.KeyEnd, key.String() == "G":
		p.cursor = last
	}
	p.cursor = min(max(p.cursor, 0), last)
	return p, false
}

// rowsPerTurn is the prompt, the start of the reply, and a line between turns.
const rowsPerTurn = 3

// fits is how many turns a box of rows has room for.
func fits(rows int) int {
	return max((rows+1)/rowsPerTurn, 1)
}

// rows draws the turns around the cursor that fit the box: the prompt, bright
// at the cursor, and under it how the reply begins.
func (p turnPicker) rows(look styles, width, rows int) string {
	room := fits(rows)
	from := min(max(p.cursor-room/2, 0), max(len(p.turns)-room, 0))

	var lines []string
	for index := from; index < min(from+room, len(p.turns)); index++ {
		if index > from {
			lines = append(lines, "")
		}
		pointer, marker := "  ", "  "
		if index == p.cursor {
			pointer = "› "
		}
		if p.kept[index] {
			marker = "✓ "
		}
		promptStyle := look.text
		if index == p.cursor {
			promptStyle = look.bright
		}
		text := width - lipgloss.Width(pointer+marker)
		lines = append(lines,
			look.accent.Render(pointer)+look.text.Render(marker)+
				promptStyle.Render(cutTo(promptLine(p.turns[index].Prompt, index), max(text, 1))),
			strings.Repeat(" ", lipgloss.Width(pointer+marker))+
				look.faded.Render(cutTo(firstLine(p.turns[index].Reply), max(text, 1))))
	}
	return strings.Join(lines, "\n")
}

func (p turnPicker) position() string {
	return strconv.Itoa(p.cursor+1) + "/" + strconv.Itoa(len(p.turns))
}

// footer names the keys the list answers to.
func (p turnPicker) footer(look styles, inner int, markdown bool, style string) string {
	key := func(name, does string) string { return look.key.Render(name) + look.hint.Render(" "+does) }
	hints := []string{key("↑↓", "move"), key("p", "preview"), key("enter", "translate"), key("esc", "back")}
	if p.previewing {
		hints = []string{key("↑↓", "scroll"), key("M", markdownHint(markdown))}
		if markdown {
			hints = append(hints, key("S", style))
		}
		hints = append(hints, key("enter", "translate"), key("esc", "list"))
	}
	return spread(" "+strings.Join(hints, look.hint.Render(" · ")), "", inner)
}

var (
	tag = regexp.MustCompile(`<[^>]*>`)
	// echoed is the name of a slash command said a second time.
	echoed = regexp.MustCompile(`(?s)<command-message>.*?</command-message>`)
	// summary is the line Claude Code gives a task it ran in the background
	// when it tells the agent the task is over, and outcome what the task said.
	summary = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)
	outcome = regexp.MustCompile(`(?s)<result>(.*?)</result>`)
)

// promptLine is a prompt as one line, without the tags a slash command is
// recorded in, so /plan reads as it was typed. A background task that ended
// goes by its summary, and a prompt with nothing left to read, an image alone,
// by its number.
func promptLine(prompt string, index int) string {
	if said := summary.FindStringSubmatch(prompt); said != nil {
		prompt = said[1]
	}
	prompt = tag.ReplaceAllString(echoed.ReplaceAllString(prompt, " "), " ")
	if line := strings.Join(strings.Fields(prompt), " "); line != "" {
		return line
	}
	return "#" + strconv.Itoa(index+1)
}

// promptText is a prompt whole, as the preview shows it: without the tags it
// was recorded in, and a background task that ended as its summary and what it
// said, not the bookkeeping around them.
func promptText(prompt string, index int) string {
	if said := summary.FindStringSubmatch(prompt); said != nil {
		task := said[1]
		if result := outcome.FindStringSubmatch(prompt); result != nil {
			task += "\n\n" + result[1]
		}
		prompt = task
	}
	var lines []string
	for _, line := range strings.Split(prompt, "\n") {
		bare := strings.TrimRight(tag.ReplaceAllString(echoed.ReplaceAllString(line, ""), ""), " \t")
		switch {
		case bare == "" && strings.TrimSpace(line) != "":
			// A line that held only tags held nothing to read.
			continue
		case bare == "" && (len(lines) == 0 || lines[len(lines)-1] == ""):
			continue
		}
		lines = append(lines, bare)
	}
	if text := strings.TrimSpace(strings.Join(lines, "\n")); text != "" {
		return text
	}
	return "#" + strconv.Itoa(index+1)
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// session is the source of the replies when it can hand out every one of
// them, and there are turns to tell apart.
func (r Reader) session() (Session, bool) {
	session, ok := r.replies.(Session)
	return session, ok && !r.options.Whole
}

// listTurns reads every reply of the session, for l.
func (r Reader) listTurns() tea.Cmd {
	session, ok := r.session()
	if !ok {
		return nil
	}
	ctx := r.ctx
	return func() tea.Msg {
		turns, err := session.All(ctx)
		return turnsMsg{turns: turns, err: err}
	}
}

func (r Reader) turnsListed(msg turnsMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		r.failure = msg.err
	case len(msg.turns) == 0:
		r.hint = "the agent has not said anything yet"
	default:
		r.turns = r.turns.show(msg.turns, r.options.History)
	}
	return r, nil
}

func (r Reader) pickTurn(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if r.turns.previewing {
		return r.previewKey(key)
	}
	var picked bool
	r.turns, picked = r.turns.key(key, fits(r.rows()))
	if !picked {
		return r, nil
	}
	return r.translatePicked(r.turns.cursor)
}

// translatePicked shows the reply picked in place of the latest, translated
// knowing the replies before it as the latest would be. One translated before
// comes from the history; one being translated now is given up on.
func (r Reader) translatePicked(index int) (tea.Model, tea.Cmd) {
	turns := r.turns.turns
	before := make([]string, 0, index)
	for _, turn := range turns[:index] {
		before = append(before, turn.Reply)
	}
	// A reading of the replies still on its way would put the latest back.
	r.fetch++
	r.loaded = true
	r.shown = []string{turns[index].Reply}
	r.context = before[max(len(before)-r.options.Context, 0):]
	r.results, r.next = make([]result, 1), 0
	r.held, r.browse, r.fresh, r.original = false, nil, false, false
	r.toTop()
	r.picked = "reply " + r.turns.position()
	if r.options.Trouble != nil {
		return r, nil
	}
	started := r.startTranslating()
	return r, started
}

// previewKey scrolls the turn shown whole, translates it, or goes back to the
// list where it was.
func (r Reader) previewKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	var kept tea.Cmd
	switch key.String() {
	case "M":
		r.markdown = !r.markdown
	case "S":
		kept = r.nextStyle()
	}
	rows := r.rows()
	bottom := max(len(r.previewLines())-rows, 0)
	from := r.turns.from
	switch {
	case key.Type == tea.KeyEsc, key.Type == tea.KeyLeft, key.String() == "p":
		r.turns.previewing = false
		return r, nil
	case key.Type == tea.KeyEnter:
		r.turns.previewing, r.turns.open = false, false
		return r.translatePicked(r.turns.cursor)
	case key.Type == tea.KeyUp, key.String() == "k":
		from--
	case key.Type == tea.KeyDown, key.String() == "j":
		from++
	case key.Type == tea.KeyPgUp, key.Type == tea.KeyCtrlB:
		from -= rows
	case key.Type == tea.KeyPgDown, key.Type == tea.KeySpace:
		from += rows
	case key.Type == tea.KeyHome, key.String() == "g":
		from = 0
	case key.Type == tea.KeyEnd, key.String() == "G":
		from = bottom
	}
	r.turns.from = min(max(from, 0), bottom)
	return r, kept
}

// scrollPreview moves the turn shown whole to from, as far as there is text.
func (r *Reader) scrollPreview(from int) {
	bottom := max(len(r.previewLines())-r.rows(), 0)
	r.turns.from = min(max(from, 0), bottom)
}

// previewLines are the prompt at the cursor and its reply, whole and as
// written, wrapped to the box and styled line by line.
func (r Reader) previewLines() []string {
	look := r.canvas.styles
	width := r.canvas.contentWidth()
	turn := r.turns.turns[r.turns.cursor]
	var lines []string
	add := func(text string, style lipgloss.Style) {
		for _, line := range strings.Split(wrapped(text, width), "\n") {
			lines = append(lines, style.Render(line))
		}
	}
	add(promptText(turn.Prompt, r.turns.cursor), look.bright)
	add("", look.text)
	add(strings.Repeat("─", min(width, 24)), look.mark)
	add("", look.text)
	if r.markdown {
		lines = append(lines, r.render.lines(turn.Reply, width)...)
	} else {
		add(turn.Reply, look.text)
	}
	return lines
}

func (r Reader) previewView() string {
	rows := r.rows()
	lines := r.previewLines()
	total := len(lines)
	from := min(r.turns.from, max(total-rows, 0))
	shown := strings.Join(lines[from:min(from+rows, total)], "\n")
	box := r.canvas.labelled(
		r.canvas.box(true).Height(rows).Render(r.canvas.scrolled(shown, from, rows, total)),
		strings.TrimSpace("replies "+howFarThrough(from, rows, total)), true)
	line := lipgloss.Width(box)
	return strings.Join([]string{r.header(line), box, r.turns.footer(r.canvas.styles, line-1, r.markdown, r.render.Style())}, "\n")
}

func (r Reader) turnsView() string {
	if r.turns.previewing {
		return r.previewView()
	}
	rows := r.rows()
	look := r.canvas.styles
	body := r.turns.rows(look, r.canvas.contentWidth(), rows)
	box := r.canvas.labelled(r.canvas.box(true).Height(rows).Render(body), "replies", true)
	line := lipgloss.Width(box)
	return strings.Join([]string{r.header(line), box, r.turns.footer(look, line-1, r.markdown, r.render.Style())}, "\n")
}
