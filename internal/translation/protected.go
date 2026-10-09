package translation

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Services translate code as if it were prose: identifiers get renamed, comments
// and string literals are rewritten, and indentation is reformatted. Code is
// therefore never sent. A marker stands in its place, which keeps the sentence
// around it whole — a service given only the prose between the code would be
// translating fragments.
//
// Markers are these brackets and a number, which nobody types by accident. What
// looks like one in the draft is protected too, so it cannot collide with ours.
const (
	markerOpen  = "⟦"
	markerClose = "⟧"
)

// A fence runs to the closing one, or to the end of a draft still being written.
// Inline spans need both backticks: a lone one is a character someone typed.
var protectedSpans = regexp.MustCompile(
	"(?s)```.*?```|```.*$|`[^`\n]+`|" + markerOpen + `\d*` + markerClose)

type protecting struct{ translator Translator }

// Protecting keeps code out of a translation, and out of the request.
func Protecting(translator Translator) Translator {
	return &protecting{translator: translator}
}

func (p *protecting) Translate(ctx context.Context, draft string) (string, error) {
	marked, protected := takeOut(draft)
	if len(protected) == 0 {
		return p.translator.Translate(ctx, draft)
	}

	translated, err := p.translator.Translate(ctx, marked)
	if err != nil {
		return "", err
	}
	return putBack(translated, protected)
}

// TranslateWithContext keeps code out of what came before as well. That text is
// only read, never returned, so its code is left out rather than marked: a
// marker there would be one the service could mistake for the text's own.
// A service that cannot be told what came before translates the text alone.
func (p *protecting) TranslateWithContext(ctx context.Context, text, preceding string) (string, error) {
	contextual, takesContext := p.translator.(ContextualTranslator)
	if !takesContext || strings.TrimSpace(preceding) == "" {
		return p.Translate(ctx, text)
	}

	marked, protected := takeOut(text)
	translated, err := contextual.TranslateWithContext(ctx, marked, withoutCode(preceding))
	if err != nil {
		return "", err
	}
	return putBack(translated, protected)
}

func (p *protecting) Unwrap() Translator { return p.translator }

func withoutCode(text string) string {
	return protectedSpans.ReplaceAllString(text, "…")
}

// takeOut replaces every protected span with a marker, and says what was taken.
func takeOut(draft string) (marked string, protected []string) {
	marked = protectedSpans.ReplaceAllStringFunc(draft, func(span string) string {
		protected = append(protected, span)
		return marker(len(protected) - 1)
	})
	return marked, protected
}

// putBack restores what was taken out. A service that dropped or repeated a
// marker has lost or duplicated the code with it, which is worth saying rather
// than handing an agent a prompt with a hole in it.
func putBack(translated string, protected []string) (string, error) {
	for index, span := range protected {
		found := marker(index)
		switch strings.Count(translated, found) {
		case 1:
			translated = strings.Replace(translated, found, span, 1)
		case 0:
			return "", errors.New("the service did not return a protected part of the draft")
		default:
			return "", errors.New("the service repeated a protected part of the draft")
		}
	}
	return translated, nil
}

func marker(index int) string {
	return markerOpen + strconv.Itoa(index) + markerClose
}
