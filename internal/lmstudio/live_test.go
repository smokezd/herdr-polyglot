//go:build live

package lmstudio_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wazum/herdr-polyglot/internal/lmstudio"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

// A check against the model loaded in LM Studio on this machine, kept behind a
// build tag so the ordinary suite needs nothing running:
//
//	go test -tags live -run Live -v ./internal/lmstudio/
//
// HERDR_POLYGLOT_MODEL names a model; without it the loaded one answers.
func TestLiveTranslationThroughTheLoadedModel(t *testing.T) {
	probe := http.Client{Timeout: 2 * time.Second}
	response, err := probe.Get("http://localhost:1234/v1/models")
	if err != nil {
		t.Skip("LM Studio is not serving on localhost:1234")
	}
	response.Body.Close()

	// As the popup sends it: code taken out first, a marker in its place.
	translator := translation.Protecting(lmstudio.New(
		lmstudio.WithTargetLanguage("EN-US"),
		lmstudio.WithModel(os.Getenv("HERDR_POLYGLOT_MODEL"))))

	started := time.Now()
	translated, err := translator.Translate(context.Background(),
		"Bitte behebe den fehlschlagenden Test in `parser_test.go` und erklär mir danach, was falsch war.")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	t.Logf("%s (in %v)", translated, time.Since(started).Round(time.Millisecond))

	for _, part := range []string{"test", "fix", "`parser_test.go`"} {
		if !strings.Contains(strings.ToLower(translated), part) {
			t.Errorf("the translation is %q, want English with %q in it", translated, part)
		}
	}
}
