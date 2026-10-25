package overlay

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/x/ansi"
)

// defaultMarkdownStyle is glamour's own dark style, the one glow uses on a
// dark terminal.
const defaultMarkdownStyle = "dark"

// MarkdownStyles are glamour's styles in the order S goes through them. Its
// auto style is not one: it asks the terminal for its background, which a
// popup may never answer.
var MarkdownStyles = []string{"dark", "light", "dracula", "tokyo-night", "pink", "ascii", "notty"}

// nextStyle is the style after style, round to the first again.
func nextStyle(style string) string {
	for index, known := range MarkdownStyles {
		if known == style {
			return MarkdownStyles[(index+1)%len(MarkdownStyles)]
		}
	}
	return MarkdownStyles[0]
}

// markdown renders a reply the way glow does. The screen is drawn again on
// every key and every piece of a streamed translation, so what was rendered is
// kept until the width changes; a reply being written is rendered anew as it
// grows, which glamour does quickly enough.
type markdown struct {
	style string

	mu       sync.Mutex
	width    int
	renderer *glamour.TermRenderer
	rendered map[string][]string
}

// keptRenderings is how many texts are kept rendered before starting again: a
// streamed reply leaves one behind for every piece.
const keptRenderings = 256

func newMarkdown(style string) *markdown {
	if style == "" {
		style = defaultMarkdownStyle
	}
	return &markdown{style: style}
}

// Style is the style rendered in.
func (m *markdown) Style() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.style
}

// use renders in style from now on, every text afresh.
func (m *markdown) use(style string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.style, m.renderer = style, nil
}

// lines is text rendered at width, one styled line each, without the blank
// lines glamour puts around a document. Text glamour cannot render is shown
// as it was written.
func (m *markdown) lines(text string, width int) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.renderer == nil || m.width != width {
		renderer, err := glamour.NewTermRenderer(glamour.WithStandardStyle(m.style), glamour.WithWordWrap(width))
		if err != nil {
			return strings.Split(wrapped(text, width), "\n")
		}
		m.renderer, m.width, m.rendered = renderer, width, map[string][]string{}
	}
	if kept, ok := m.rendered[text]; ok {
		return kept
	}

	out, err := m.renderer.Render(joined(text))
	if err != nil {
		return strings.Split(wrapped(text, width), "\n")
	}
	lines := strings.Split(strings.ReplaceAll(out, nonBreaking, "-"), "\n")
	blank := func(line string) bool { return strings.TrimSpace(ansi.Strip(line)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	// A line wider than the box would push its border out.
	for index, line := range lines {
		lines[index] = ansi.Truncate(line, width, "")
	}

	if len(m.rendered) >= keptRenderings {
		m.rendered = map[string][]string{}
	}
	m.rendered[text] = lines
	return lines
}

// nonBreaking is a hyphen no line is broken at.
const nonBreaking = "\u2011"

// joined keeps glamour from breaking a line at a hyphen. Its first wrap takes
// every hyphen for a place to break, counts wrong when it does, and writes a
// line wider than it was told; its second wrap then breaks that line again,
// and leaves its last word on a line of its own. A hyphen that never breaks a
// line has neither wrap counting wrong, and is put back once the reply is
// rendered. The hyphens markdown is written in are left as they are: a line
// that starts with one is a list item or a rule, and a line of nothing but
// hyphens, bars and colons is a table's rule.
func joined(text string) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") || strings.Trim(trimmed, "-|: ") == "" {
			continue
		}
		lines[index] = strings.ReplaceAll(line, "-", nonBreaking)
	}
	return strings.Join(lines, "\n")
}

// markdownHint is what M does next.
func markdownHint(on bool) string {
	if on {
		return "plain"
	}
	return "markdown"
}
