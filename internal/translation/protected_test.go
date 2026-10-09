package translation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/translation"
)

const codeBlock = "```go\nif kunde == nil { // sollte nie passieren\n\treturn fmt.Errorf(\"kunde fehlt\")\n}\n```"

// A service asked to translate code renames the identifiers in it, so the code is
// never sent: it is taken out, and put back exactly as it was.
func TestACodeBlockIsNeitherSentNorChanged(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Der Fehler steht hier:\n\n" + codeBlock + "\n\nBitte behebe das."
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, codeBlock) {
		t.Errorf("the code came back as %q, want it untouched", translated)
	}
	for _, sent := range service.sent() {
		if strings.Contains(sent, "kunde") || strings.Contains(sent, "fmt.Errorf") {
			t.Errorf("the code was sent to the service: %q", sent)
		}
	}
}

func TestAnInlineCodeSpanIsNeitherSentNorChanged(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(),
		"Schreibe einen Test für `berechneRabatt(warenkorb, prozent)` bitte.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "`berechneRabatt(warenkorb, prozent)`") {
		t.Errorf("the function name came back as %q, want it untouched", translated)
	}
	if sent := strings.Join(service.sent(), " "); strings.Contains(sent, "berechneRabatt") {
		t.Errorf("the function name was sent to the service: %q", sent)
	}
}

// The prose still goes as whole sentences, with a marker standing in for the code,
// or the service would be translating fragments.
func TestTheProseAroundTheCodeIsStillTranslatedAsASentence(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	if _, err := translator.Translate(context.Background(),
		"Ersetze `alterName` durch `neuerName` im Formular."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	sent := strings.Join(service.sent(), " ")
	if !strings.Contains(sent, "Ersetze") || !strings.Contains(sent, "im Formular") {
		t.Errorf("the service was sent %q, want the sentence around the code", sent)
	}
	if strings.Count(sent, "⟦") != 2 {
		t.Errorf("the service was sent %q, want a marker for each protected span", sent)
	}
}

func TestADraftWithoutCodeIsPassedStraightThrough(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(), "Bitte behebe den Test.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "<Bitte behebe den Test.>" {
		t.Errorf("Translate returned %q, want the plain answer", translated)
	}
	if sent := strings.Join(service.sent(), " "); strings.Contains(sent, "⟦") {
		t.Errorf("the service was sent a marker for a draft with no code: %q", sent)
	}
}

// A block being typed has no closing fence yet, and half-written code is the least
// worth translating.
func TestAnUnfinishedCodeBlockIsProtectedToTheEnd(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	draft := "Hier der Fehler:\n\n```go\nif kunde == nil {"
	translated, err := translator.Translate(context.Background(), draft)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "```go\nif kunde == nil {") {
		t.Errorf("the unfinished block came back as %q, want it untouched", translated)
	}
	if sent := strings.Join(service.sent(), " "); strings.Contains(sent, "kunde") {
		t.Errorf("the unfinished block was sent: %q", sent)
	}
}

// A lone backtick is a character someone typed, not a span to protect.
func TestALoneBacktickIsLeftAlone(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	if _, err := translator.Translate(context.Background(),
		"Das Zeichen ` steht auf der Tastatur oben links."); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if sent := strings.Join(service.sent(), " "); !strings.Contains(sent, "Tastatur") {
		t.Errorf("the service was sent %q, want the whole sentence", sent)
	}
}

type markerLosingTranslator struct{ spyTranslator }

func (m *markerLosingTranslator) Translate(ctx context.Context, text string) (string, error) {
	translated, err := m.spyTranslator.Translate(ctx, text)
	if err != nil {
		return "", err
	}
	// A service that drops a marker takes the code with it.
	if at := strings.Index(translated, "⟦"); at >= 0 {
		if end := strings.Index(translated[at:], "⟧"); end >= 0 {
			translated = translated[:at] + translated[at+end+len("⟧"):]
		}
	}
	return translated, nil
}

// Delivering a prompt with the code silently missing is worse than saying so.
func TestAMarkerTheServiceLostIsAnErrorRatherThanAQuietLoss(t *testing.T) {
	t.Parallel()
	translator := translation.Protecting(&markerLosingTranslator{})

	_, err := translator.Translate(context.Background(),
		"Der Fehler steht hier:\n\n"+codeBlock+"\n\nBitte behebe das.")
	if err == nil {
		t.Fatal("Translate returned no error for a translation that lost the code")
	}
	if !strings.Contains(err.Error(), "protected") {
		t.Errorf("Translate returned %v, want it to say a protected part went missing", err)
	}
}

// Someone writing the marker characters themselves must not collide with ours.
func TestTextThatLooksLikeAMarkerComesBackAsItWas(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator := translation.Protecting(service)

	translated, err := translator.Translate(context.Background(),
		"Die Notation ⟦0⟧ kommt aus der Mathematik.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if !strings.Contains(translated, "⟦0⟧") {
		t.Errorf("Translate returned %q, want the notation kept", translated)
	}
}

// Live mode and the allowance counter both look through wrappers.
func TestProtectingCanBeLookedThrough(t *testing.T) {
	t.Parallel()
	service := &reportingSpy{spent: translation.Usage{Used: 12, Limit: 500}}

	reporter, err := translation.ReporterOf(translation.Protecting(translation.Segmented(service)))
	if err != nil {
		t.Fatalf("ReporterOf returned unexpected error: %v", err)
	}
	spent, err := reporter.Usage(context.Background())
	if err != nil {
		t.Fatalf("Usage returned unexpected error: %v", err)
	}
	if spent.Used != 12 {
		t.Errorf("Usage returned %+v, want what the service said", spent)
	}
}

func TestWhatCameBeforeIsPassedOnWithoutItsCode(t *testing.T) {
	t.Parallel()
	service := &spyTranslator{}
	translator, takesContext := translation.Protecting(service).(translation.ContextualTranslator)
	if !takesContext {
		t.Fatal("a protected translator cannot be told what came before")
	}

	translated, err := translator.TranslateWithContext(context.Background(),
		"Run `make qa` now.", "I changed `berechneRabatt`:\n\n"+codeBlock)
	if err != nil {
		t.Fatalf("TranslateWithContext returned unexpected error: %v", err)
	}

	if !strings.Contains(translated, "`make qa`") {
		t.Errorf("the code came back as %q, want it untouched", translated)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.contexts) != 1 {
		t.Fatalf("the service was told what came before %d times, want once", len(service.contexts))
	}
	if sent := service.contexts[0]; strings.Contains(sent, "berechneRabatt") || strings.Contains(sent, "kunde") {
		t.Errorf("code from what came before was sent: %q", sent)
	}
}

// streamingSpy hands out its answer a piece at a time.
type streamingSpy struct {
	pieces []translation.Progress
}

func (s *streamingSpy) Translate(context.Context, string) (string, error) { return "", nil }

func (s *streamingSpy) TranslateStreaming(
	_ context.Context, _, _ string, progress func(translation.Progress),
) (string, error) {
	for _, piece := range s.pieces {
		progress(piece)
	}
	return s.pieces[len(s.pieces)-1].Text, nil
}

func TestAStreamedTranslationShowsTheCodeAsSoonAsItsMarkerArrives(t *testing.T) {
	t.Parallel()
	service := &streamingSpy{pieces: []translation.Progress{
		{Thinking: "hmm"},
		{Thinking: "hmm", Thought: true, Text: "Führe ⟦0"},
		{Thinking: "hmm", Thought: true, Text: "Führe ⟦0⟧ aus."},
	}}
	translator, streams := translation.Protecting(service).(translation.StreamingTranslator)
	if !streams {
		t.Fatal("a protected translator cannot stream")
	}

	var seen []translation.Progress
	translated, err := translator.TranslateStreaming(context.Background(), "Run `make qa`.", "",
		func(so translation.Progress) { seen = append(seen, so) })
	if err != nil {
		t.Fatalf("TranslateStreaming returned unexpected error: %v", err)
	}

	if translated != "Führe `make qa` aus." {
		t.Errorf("translation is %q, want the code put back", translated)
	}
	if len(seen) != 3 || seen[0].Thinking != "hmm" || seen[1].Text != "Führe " ||
		seen[2].Text != "Führe `make qa` aus." {
		t.Errorf("progress was %+v, want the thinking, then the text with a half marker held back and a whole one restored", seen)
	}
}

func TestAServiceThatCannotStreamAnswersAtOnce(t *testing.T) {
	t.Parallel()
	translator := translation.Protecting(&spyTranslator{}).(translation.StreamingTranslator)

	called := false
	translated, err := translator.TranslateStreaming(context.Background(), "Hallo", "",
		func(translation.Progress) { called = true })
	if err != nil || translated != "<Hallo>" {
		t.Fatalf("translation is %q (%v), want the service's answer", translated, err)
	}
	if called {
		t.Error("progress was reported for a service that does not stream")
	}
}
