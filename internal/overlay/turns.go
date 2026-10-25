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
func (p turnPicker) footer(look styles, inner int) string {
	key := func(name, does string) string { return look.key.Render(name) + look.hint.Render(" "+does) }
	hints := []string{key("↑↓", "move"), key("enter", "translate"), key("esc", "back")}
	return spread(" "+strings.Join(hints, look.hint.Render(" · ")), "", inner)
}

var (
	tag = regexp.MustCompile(`<[^>]*>`)
	// echoed is the name of a slash command said a second time.
	echoed = regexp.MustCompile(`(?s)<command-message>.*?</command-message>`)
	// summary is the line Claude Code gives a task it ran in the background
	// when it tells the agent the task is over.
	summary = regexp.MustCompile(`(?s)<summary>(.*?)</summary>`)
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
	r.held, r.browse, r.fresh, r.original, r.follow = false, nil, false, false, true
	r.picked = "reply " + r.turns.position()
	if r.options.Trouble != nil {
		return r, nil
	}
	started := r.startTranslating()
	return r, started
}

func (r Reader) turnsView() string {
	rows := r.rows()
	look := r.canvas.styles
	body := r.turns.rows(look, r.canvas.contentWidth(), rows)
	box := r.canvas.labelled(r.canvas.box(true).Height(rows).Render(body), "replies", true)
	line := lipgloss.Width(box)
	return strings.Join([]string{r.header(line), box, r.turns.footer(look, line-1)}, "\n")
}
