package lmstudio_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wazum/herdr-polyglot/internal/lmstudio"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

// streamingServer answers with one event per delta, flushed as it goes, and
// remembers whether it was asked to stream.
func streamingServer(t *testing.T, deltas []map[string]string, stall time.Duration) (server *httptest.Server, asked *bool) {
	t.Helper()
	asked = new(bool)
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		*asked, _ = body["stream"].(bool)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for index, delta := range deltas {
			event, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta}}})
			_, _ = io.WriteString(w, "data: "+string(event)+"\n\n")
			flusher.Flush()
			if stall > 0 && index == len(deltas)-1 {
				select {
				case <-time.After(stall):
				case <-r.Context().Done():
					return
				}
			}
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	return server, asked
}

func streamed(t *testing.T, client *lmstudio.Client) (string, []translation.Progress, error) {
	t.Helper()
	var seen []translation.Progress
	translated, err := client.TranslateStreaming(context.Background(), "The test passes.", "",
		func(so translation.Progress) { seen = append(seen, so) })
	return translated, seen, err
}

func TestThinkingArrivesFirstAndEndsWhereTheTranslationBegins(t *testing.T) {
	t.Parallel()

	for name, deltas := range map[string][]map[string]string{
		"in its own field": {
			{"reasoning_content": "The user wants "},
			{"reasoning_content": "German."},
			{"content": "Der Test "},
			{"content": "läuft."},
		},
		"in the newer field": {
			{"reasoning": "The user wants German."},
			{"content": "Der Test läuft."},
		},
		"in think tags": {
			{"content": "<thi"},
			{"content": "nk>The user wants "},
			{"content": "German.</think>\n\n<text>Der Test "},
			{"content": "läuft.</te"},
			{"content": "xt>"},
		},
	} {
		server, asked := streamingServer(t, deltas, 0)
		client := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithTargetLanguage("DE"))

		translated, seen, err := streamed(t, client)
		if err != nil {
			t.Fatalf("%s: TranslateStreaming returned unexpected error: %v", name, err)
		}
		if !*asked {
			t.Errorf("%s: the request did not ask for a stream", name)
		}
		if translated != "Der Test läuft." {
			t.Errorf("%s: translation is %q, want only the answer", name, translated)
		}
		for _, so := range seen {
			if strings.ContainsAny(so.Text, "<>") {
				t.Errorf("%s: a tag reached the screen: %+v", name, so)
			}
		}
		first, last := seen[0], seen[len(seen)-1]
		if first.Thought || first.Text != "" {
			t.Errorf("%s: first progress is %+v, want thinking only", name, first)
		}
		if !last.Thought || last.Thinking != "The user wants German." || last.Text != "Der Test läuft." {
			t.Errorf("%s: last progress is %+v, want the thinking over and the answer whole", name, last)
		}
	}
}

func TestAModelThatDoesNotThinkStreamsOnlyItsAnswer(t *testing.T) {
	t.Parallel()
	server, _ := streamingServer(t, []map[string]string{{"content": "Der Test "}, {"content": "läuft."}}, 0)

	translated, seen, err := streamed(t, lmstudio.New(lmstudio.WithEndpoint(server.URL)))
	if err != nil || translated != "Der Test läuft." {
		t.Fatalf("translation is %q (%v), want the answer", translated, err)
	}
	if seen[0].Text != "Der Test" || seen[0].Thinking != "" {
		t.Errorf("first progress is %+v, want the start of the answer and no thinking", seen[0])
	}
}

func TestAStreamThatFallsSilentIsGivenUpOn(t *testing.T) {
	t.Parallel()
	server, _ := streamingServer(t, []map[string]string{{"content": "Der Test"}}, 5*time.Second)
	client := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithTimeout(200*time.Millisecond))

	_, _, err := streamed(t, client)
	if err == nil || !strings.Contains(err.Error(), "stopped answering") {
		t.Errorf("error is %v, want the server to have stopped answering", err)
	}
}
