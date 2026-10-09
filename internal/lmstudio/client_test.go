package lmstudio_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wazum/herdr-polyglot/internal/lmstudio"
)

type capturedRequest struct {
	authorization string
	body          map[string]any
}

func (c *capturedRequest) message(role string) string {
	messages, _ := c.body["messages"].([]any)
	for _, entry := range messages {
		message, _ := entry.(map[string]any)
		if message["role"] == role {
			content, _ := message["content"].(string)
			return content
		}
	}
	return ""
}

func serverReturning(t *testing.T, status int, response string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.authorization = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured.body)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	return server, captured
}

func answering(content string) string {
	encoded, _ := json.Marshal(content)
	return `{"choices":[{"index":0,"message":{"role":"assistant","content":` + string(encoded) + `}}]}`
}

func TestTranslateSendsTheDraftAndReturnsTheTranslation(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answering("Please fix the failing test"))
	client := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithTargetLanguage("EN-US"))

	english, err := client.Translate(context.Background(), "Bitte behebe den fehlschlagenden Test")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if english != "Please fix the failing test" {
		t.Errorf("Translate returned %q, want the translated text", english)
	}
	if user := captured.message("user"); user != "<text>Bitte behebe den fehlschlagenden Test</text>" {
		t.Errorf("the draft went as %q, want it inside the tags", user)
	}
	if system := captured.message("system"); !strings.Contains(system, "American English") {
		t.Errorf("the instructions are %q, want the target language named", system)
	}
	if captured.body["temperature"] != 0.0 || captured.body["stream"] != false {
		t.Errorf("the request is %v, want a steady answer in one piece", captured.body)
	}
}

// Without a model named the server answers with the one it has loaded, which is
// what someone who never set one expects.
func TestTheModelIsSentOnlyWhenOneIsNamed(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answering("Fix it"))

	if _, err := lmstudio.New(lmstudio.WithEndpoint(server.URL)).Translate(context.Background(), "Behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if _, named := captured.body["model"]; named {
		t.Errorf("the request names model %v, want none", captured.body["model"])
	}

	named := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithModel("qwen/qwen3-8b"))
	if _, err := named.Translate(context.Background(), "Behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if captured.body["model"] != "qwen/qwen3-8b" {
		t.Errorf("the request names model %v, want the one configured", captured.body["model"])
	}
}

func TestAKeyIsSentOnlyWhenThereIsOne(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answering("Fix it"))

	if _, err := lmstudio.New(lmstudio.WithEndpoint(server.URL)).Translate(context.Background(), "Behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if captured.authorization != "" {
		t.Errorf("Authorization is %q without a key, want none", captured.authorization)
	}

	keyed := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithAPIKey("key-123"))
	if _, err := keyed.Translate(context.Background(), "Behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if captured.authorization != "Bearer key-123" {
		t.Errorf("Authorization is %q, want the key as a bearer token", captured.authorization)
	}
}

// What is added after the draft stays outside the tags, so it is read as an
// instruction and never translated as part of the prompt.
func TestWhatIsAddedToThePromptGoesWhereItIsRead(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answering("Fix it"))
	client := lmstudio.New(lmstudio.WithEndpoint(server.URL),
		lmstudio.WithSystemPromptExtra("Use British spelling."),
		lmstudio.WithUserPromptExtra("/no_think"))

	if _, err := client.Translate(context.Background(), "Behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if user := captured.message("user"); user != "<text>Behebe es</text>\n/no_think" {
		t.Errorf("the draft went as %q, want the addition after the tags", user)
	}
	if system := captured.message("system"); !strings.HasSuffix(system, "\nUse British spelling.") {
		t.Errorf("the instructions are %q, want the addition at the end", system)
	}
}

func TestFieldsAddedToTheRequestJoinAndReplaceItsOwn(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answering("Fix it"))
	client := lmstudio.New(lmstudio.WithEndpoint(server.URL),
		lmstudio.WithRequestExtra(map[string]any{"temperature": 0.3, "max_tokens": 512}))

	if _, err := client.Translate(context.Background(), "Behebe es"); err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if captured.body["temperature"] != 0.3 || captured.body["max_tokens"] != 512.0 {
		t.Errorf("the request is %v, want the fields added and replaced", captured.body)
	}
	if captured.message("user") == "" {
		t.Error("the draft went missing once fields were added")
	}
}

func TestWhatTheModelWroteAroundTheTranslationIsLeftOut(t *testing.T) {
	t.Parallel()

	for written, want := range map[string]string{
		"<think>\nThe user wants…\n</think>\n\nFix it": "Fix it",
		"<text>Fix it</text>":                          "Fix it",
		"<think>hm</think><text>Fix it</text>\n":       "Fix it",
		"  Fix it\n":                                   "Fix it",
		"Use <text> and </text> here":                  "Use <text> and </text> here",
		"<text>a</text> and <text>b</text>":            "<text>a</text> and <text>b</text>",
	} {
		server, _ := serverReturning(t, http.StatusOK, answering(written))
		translated, err := lmstudio.New(lmstudio.WithEndpoint(server.URL)).Translate(context.Background(), "Behebe es")
		if err != nil {
			t.Errorf("%q: Translate returned unexpected error: %v", written, err)
			continue
		}
		if translated != want {
			t.Errorf("%q: Translate returned %q, want %q", written, translated, want)
		}
	}
}

func TestAnEmptyAnswerIsNotATranslation(t *testing.T) {
	t.Parallel()

	for _, response := range []string{`{"choices":[]}`, answering(""), answering("<think>only thinking</think>")} {
		server, _ := serverReturning(t, http.StatusOK, response)
		_, err := lmstudio.New(lmstudio.WithEndpoint(server.URL)).Translate(context.Background(), "Behebe es")
		if err == nil || !strings.Contains(err.Error(), "without a translation") {
			t.Errorf("%s: Translate returned %v, want it to say there was no translation", response, err)
		}
	}
}

func TestARefusalSaysWhatTheServerSaid(t *testing.T) {
	t.Parallel()

	for response, want := range map[string]string{
		`{"error":{"message":"No models loaded. Please load a model."}}`: "No models loaded",
		`{"error":"Model not found"}`:                                    "Model not found",
		`Bad Gateway`:                                                    "Bad Gateway",
	} {
		server, _ := serverReturning(t, http.StatusBadRequest, response)
		_, err := lmstudio.New(lmstudio.WithEndpoint(server.URL)).Translate(context.Background(), "Behebe es")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: Translate returned %v, want %q in it", response, err, want)
		}
	}
}

func TestAServerThatIsNotRunningCouldNotBeReached(t *testing.T) {
	t.Parallel()
	server, _ := serverReturning(t, http.StatusOK, answering("Fix it"))
	endpoint := server.URL
	server.Close()

	_, err := lmstudio.New(lmstudio.WithEndpoint(endpoint)).Translate(context.Background(), "Behebe es")
	if err == nil || !strings.Contains(err.Error(), "LM Studio could not be reached") {
		t.Errorf("Translate returned %v, want it to say the server could not be reached", err)
	}
}

// The first request after starting loads the model, which can outlast the wait.
func TestNoAnswerInTimeSuggestsTheModelIsStillLoading(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	client := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithTimeout(50*time.Millisecond))
	_, err := client.Translate(context.Background(), "Behebe es")
	if err == nil || !strings.Contains(err.Error(), "still loading") {
		t.Errorf("Translate returned %v, want it to suggest the model is loading", err)
	}
}

func TestClosingThePopupIsNotATimeout(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	_, err := lmstudio.New(lmstudio.WithEndpoint(server.URL)).Translate(ctx, "Behebe es")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Translate returned %v, want the cancellation as it was", err)
	}
}

func TestWhatCameBeforeTravelsAheadOfTheTextAndIsNotToBeTranslated(t *testing.T) {
	t.Parallel()
	server, captured := serverReturning(t, http.StatusOK, answering("Der Test läuft jetzt."))
	client := lmstudio.New(lmstudio.WithEndpoint(server.URL), lmstudio.WithTargetLanguage("DE"),
		lmstudio.WithAgentReplies())

	if _, err := client.TranslateWithContext(context.Background(),
		"The test passes now.", "I fixed the flaky test."); err != nil {
		t.Fatalf("TranslateWithContext returned unexpected error: %v", err)
	}

	user := captured.message("user")
	if want := "<context>I fixed the flaky test.</context>\n<text>The test passes now.</text>"; user != want {
		t.Errorf("user message is %q, want %q", user, want)
	}
	system := captured.message("system")
	for _, said := range []string{"German", "coding assistant", "<context>"} {
		if !strings.Contains(system, said) {
			t.Errorf("instructions %q do not mention %q", system, said)
		}
	}
}
