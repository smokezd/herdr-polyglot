package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// hyphenated is prose of the kind that made glamour leave a word alone on a
// line: its first wrap took a hyphen for a place to break and overshot the
// width, and its second wrap broke the line again.
const hyphenated = "❓ **Q3** - **The free-state copy clashes with the glossary**: the per-firm `CONTEXT.md` " +
	"says a 100%-off Discount applies to the read-only, well-known set-up, and the " +
	"follow-up keeps the opt-in, top-level, end-to-end check as a so-called no-op. " +
	"Mock 695-8's copy is \"A free plan does not need a card.\" The options are listed below.\n\n" +
	"- one item\n- two-part item that goes on with read-only, well-known, end-to-end, opt-in, " +
	"top-level, so-called, per-firm and follow-up words until it has to wrap\n\n" +
	"---\n\n| a-b | c |\n|-----|---|\n| x-y | z |\n"

func TestALineIsNeverBrokenTwiceLeavingAWordOnItsOwn(t *testing.T) {
	t.Parallel()

	for width := 40; width <= 140; width++ {
		lines := newMarkdown("dark").lines(hyphenated, width)
		plain := make([]string, len(lines))
		for index, line := range lines {
			plain[index] = strings.TrimSpace(ansi.Strip(line))
		}
		for index, line := range plain {
			if line == "" || index == len(plain)-1 || plain[index+1] == "" {
				continue
			}
			if len([]rune(line)) < 6 && !strings.HasPrefix(line, "•") && !strings.ContainsAny(line, "│|─") {
				t.Errorf("width %d: %q is left on a line of its own after %q", width, line, plain[max(index-1, 0)])
			}
		}
		whole := strings.Join(plain, " ")
		for _, word := range []string{"free-state", "100%-off", "end-to-end", "695-8's", "two-part", "x-y"} {
			if !strings.Contains(whole, word) {
				t.Errorf("width %d: %q is not shown as written", width, word)
			}
		}
	}
}
