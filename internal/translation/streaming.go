package translation

import (
	"context"
	"strings"
)

// Progress is a translation as far as it has come: what the service has
// thought so far, if it thinks out loud, and the translation so far. Each one
// replaces the last, so a slow reader may skip some without losing anything.
type Progress struct {
	Thinking string
	Text     string
	// Thought says the thinking is over and the translation has begun.
	Thought bool
}

// StreamingTranslator hands out a translation while it is still being written.
// A language model takes long enough over a whole reply that waiting for the
// end means staring at nothing.
type StreamingTranslator interface {
	TranslateStreaming(ctx context.Context, text, preceding string, progress func(Progress)) (string, error)
}

// TranslateStreaming keeps code out of a streamed translation as well. A
// marker that has arrived whole is shown as the code it stands for, so what
// is on screen never reads ⟦0⟧. A service that cannot stream answers at once.
func (p *protecting) TranslateStreaming(
	ctx context.Context, text, preceding string, progress func(Progress),
) (string, error) {
	streaming, streams := p.translator.(StreamingTranslator)
	if !streams {
		return p.TranslateWithContext(ctx, text, preceding)
	}

	marked, protected := takeOut(text)
	translated, err := streaming.TranslateStreaming(ctx, marked, withoutCode(preceding),
		func(so Progress) {
			so.Text = restoredSoFar(so.Text, protected)
			progress(so)
		})
	if err != nil {
		return "", err
	}
	return putBack(translated, protected)
}

// restoredSoFar puts back every marker that has arrived, leaving the check
// that each came exactly once to the end, when the translation is whole. A
// marker still arriving is held back: the next piece completes it.
func restoredSoFar(translated string, protected []string) string {
	if open := strings.LastIndex(translated, markerOpen); open >= 0 &&
		!strings.Contains(translated[open:], markerClose) {
		translated = translated[:open]
	}
	for index, span := range protected {
		translated = strings.ReplaceAll(translated, marker(index), span)
	}
	return translated
}
