package overlay

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Host is one machine a language model answers on. Model is the one named for
// it, if any; otherwise the machine's own choice is shown once it is known.
type Host struct {
	Name    string
	Address string
	Model   string
}

// Hosts are the machines a translation can go to. Probe asks one which models it
// has, which is also how the picker learns whether it is up.
type Hosts interface {
	List() []Host
	Current() string
	Probe(ctx context.Context, name string) ([]string, error)
}

// probeTimeout is how long a host has to answer before it is shown as down. A
// machine on the network that takes longer is no use for translating as you type.
const probeTimeout = time.Second

// oneHost is what ctrl+o says where there is nothing to choose between.
const oneHost = "one LM Studio host — add more with HERDR_POLYGLOT_LMSTUDIO_HOSTS"

// hostPicker is the list ctrl+o opens. It takes over the panel the way reading
// the translation does, and leaves everything underneath as it was.
type hostPicker struct {
	open    bool
	hosts   []Host
	current string
	cursor  int
	// state is what each host's probe said, by name; a host not in it is still
	// being asked.
	state map[string]string
	// opening tells this list's answers from those of one closed before.
	opening int
}

type hostProbedMsg struct {
	opening int
	name    string
	state   string
}

// choosable says whether there is more than one host to pick from.
func choosable(hosts Hosts) bool {
	return hosts != nil && len(hosts.List()) > 1
}

// show opens the list on the host in use and asks every host at once how it is.
func (p hostPicker) show(ctx context.Context, hosts Hosts) (hostPicker, tea.Cmd) {
	p.open = true
	p.hosts = hosts.List()
	p.current = hosts.Current()
	p.cursor = 0
	for index, host := range p.hosts {
		if host.Name == p.current {
			p.cursor = index
		}
	}
	p.state = map[string]string{}
	p.opening++

	asking := make([]tea.Cmd, 0, len(p.hosts))
	for _, host := range p.hosts {
		asking = append(asking, probe(ctx, hosts, host, p.opening))
	}
	return p, tea.Batch(asking...)
}

func probe(ctx context.Context, hosts Hosts, host Host, opening int) tea.Cmd {
	return func() tea.Msg {
		asking, cancel := context.WithTimeout(ctx, probeTimeout)
		defer cancel()
		models, err := hosts.Probe(asking, host.Name)
		state := "✗ unreachable"
		switch {
		case err != nil:
		case host.Model != "":
			state = "✓ " + host.Model
		case len(models) > 0:
			state = "✓ " + models[0] + " (loaded)"
		default:
			state = "✓ no model"
		}
		return hostProbedMsg{opening: opening, name: host.Name, state: state}
	}
}

func (p hostPicker) probed(msg hostProbedMsg) hostPicker {
	if p.open && msg.opening == p.opening {
		p.state[msg.name] = msg.state
	}
	return p
}

// key moves through the list, or ends it: chosen is the host picked, empty when
// the list was left without picking one or with the one already in use.
func (p hostPicker) key(key tea.KeyMsg) (picker hostPicker, chosen string) {
	switch {
	case key.Type == tea.KeyEsc:
		p.open = false
	case key.Type == tea.KeyEnter:
		p.open = false
		if picked := p.hosts[p.cursor].Name; picked != p.current {
			return p, picked
		}
	case key.Type == tea.KeyUp, key.String() == "k":
		p.cursor = max(p.cursor-1, 0)
	case key.Type == tea.KeyDown, key.String() == "j":
		p.cursor = min(p.cursor+1, len(p.hosts)-1)
	}
	return p, ""
}

// rows draws one line per host. A line too long for the box gives up the end of
// the address first: the name and whether the host is up are what to choose by.
func (p hostPicker) rows(look styles, width int) string {
	nameWidth, addressWidth := 0, 0
	for _, host := range p.hosts {
		nameWidth = max(nameWidth, lipgloss.Width(host.Name))
		addressWidth = max(addressWidth, lipgloss.Width(host.Address))
	}

	lines := make([]string, 0, len(p.hosts))
	for index, host := range p.hosts {
		pointer, marker := "  ", "  "
		if index == p.cursor {
			pointer = "› "
		}
		if host.Name == p.current {
			marker = "● "
		}
		state, ok := p.state[host.Name]
		stateStyle := look.text
		switch {
		case !ok:
			state, stateStyle = "…", look.placeholder
		case strings.HasPrefix(state, "✗"):
			stateStyle = look.danger
		}

		name := host.Name + strings.Repeat(" ", nameWidth-lipgloss.Width(host.Name))
		room := width - lipgloss.Width(pointer+marker+name) - lipgloss.Width(state) - 4
		address := host.Address + strings.Repeat(" ", addressWidth-lipgloss.Width(host.Address))
		if lipgloss.Width(address) > room {
			address = cutTo(address, max(room-1, 0)) + "…"
			if room <= 1 {
				address = ""
			}
		}

		nameStyle := look.text
		if index == p.cursor {
			nameStyle = look.bright
		}
		lines = append(lines, look.accent.Render(pointer)+look.text.Render(marker)+
			nameStyle.Render(name)+"  "+look.faded.Render(address)+"  "+stateStyle.Render(state))
	}
	return strings.Join(lines, "\n")
}

// footer names the keys the list answers to.
func (p hostPicker) footer(look styles, inner int) string {
	key := func(name, does string) string { return look.key.Render(name) + look.hint.Render(" "+does) }
	hints := []string{key("↑↓", "move"), key("enter", "use this host"), key("esc", "back")}
	return spread(" "+strings.Join(hints, look.hint.Render(" · ")), "", inner)
}

// openPicker is ctrl+o in the prompt popup. A prompt on its way to the agent is
// past changing where it is translated.
func (m Model) openPicker() (tea.Model, tea.Cmd) {
	if m.stage == translating {
		return m, nil
	}
	if !choosable(m.options.Hosts) || m.options.ChooseHost == nil {
		return m, hint(oneHost)
	}
	var asking tea.Cmd
	m.picker, asking = m.picker.show(m.ctx, m.options.Hosts)
	return m, asking
}

func (m Model) pickKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	var chosen string
	m.picker, chosen = m.picker.key(key)
	if chosen == "" {
		return m, nil
	}
	return m.useHost(chosen)
}

// useHost translates on another machine from now on. What the last one made of
// the draft is dropped, and made again here if it was on screen or live is on.
func (m Model) useHost(name string) (tea.Model, tea.Cmd) {
	prompter, service, trouble := m.options.ChooseHost(name)
	m.prompter, m.options.Service = prompter, service

	m.stopPreview()
	m.endPulse()
	shown := m.preview != "" || m.previewError != nil
	m.preview, m.previewOf, m.previewError = "", "", nil
	if m.stage == confirming {
		m.stage = composing
	}
	m.refit()

	if trouble != nil {
		return m.raiseNotice(trouble)
	}
	if !m.options.Live && !shown {
		return m, nil
	}
	m.revision++
	started, cmd := m.startPreview()
	translating := started.(Model)
	return translating, tea.Batch(cmd, translating.beginPulse())
}

func (m Model) pickerView() string {
	rows := m.readingRows()
	body := m.picker.rows(m.styles, m.contentWidth())
	box := m.labelled(m.box(true).Height(rows).Render(firstRows(body, rows)), "hosts", true)
	line := lipgloss.Width(box)
	return strings.Join([]string{m.header(line), box, m.picker.footer(m.styles, line-1)}, "\n")
}

func (r Reader) openPicker() (tea.Model, tea.Cmd) {
	if !choosable(r.options.Hosts) || r.options.ChooseHost == nil {
		r.hint = oneHost
		return r, nil
	}
	var asking tea.Cmd
	r.picker, asking = r.picker.show(r.ctx, r.options.Hosts)
	return r, asking
}

func (r Reader) pickKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	var chosen string
	r.picker, chosen = r.picker.key(key)
	if chosen == "" {
		return r, nil
	}
	return r.useHost(chosen)
}

// useHost translates the replies on show again, on the machine chosen.
func (r Reader) useHost(name string) (tea.Model, tea.Cmd) {
	translator, service, trouble := r.options.ChooseHost(name)
	r.translator, r.options.Service, r.options.Trouble = translator, service, trouble
	r.stop()
	r.attempt++
	r.failure = trouble
	r.canvas.endPulse()
	if !r.loaded || len(r.shown) == 0 || trouble != nil {
		return r, nil
	}
	r.results = make([]result, len(r.shown))
	r.next, r.follow = 0, true
	started := r.startTranslating()
	return r, started
}

func (r Reader) pickerView() string {
	rows := r.rows()
	look := r.canvas.styles
	body := r.picker.rows(look, r.canvas.contentWidth())
	box := r.canvas.labelled(r.canvas.box(true).Height(rows).Render(firstRows(body, rows)), "hosts", true)
	line := lipgloss.Width(box)
	return strings.Join([]string{r.header(line), box, r.picker.footer(look, line-1)}, "\n")
}

// firstRows keeps a list to the rows a box has. A list longer than that is more
// machines than anyone runs a model on.
func firstRows(text string, rows int) string {
	lines := strings.Split(text, "\n")
	return strings.Join(lines[:min(len(lines), max(rows, 1))], "\n")
}
