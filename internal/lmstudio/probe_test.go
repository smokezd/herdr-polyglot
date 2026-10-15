package lmstudio_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wazum/herdr-polyglot/internal/lmstudio"
)

func TestTheChatEndpointIsFoundUnderTheBase(t *testing.T) {
	t.Parallel()

	if got := lmstudio.ChatEndpoint("http://gpu:1234"); got != "http://gpu:1234/v1/chat/completions" {
		t.Errorf("ChatEndpoint is %q, want the chat completions under the base", got)
	}
}

func TestAProbeListsTheModelsAHostHas(t *testing.T) {
	t.Parallel()

	var path, authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, authorization = r.URL.Path, r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen3-32b"},{"id":"gemma-3"}]}`))
	}))
	defer server.Close()

	models, err := lmstudio.Probe(context.Background(), server.URL, "secret")
	if err != nil {
		t.Fatalf("Probe returned unexpected error: %v", err)
	}
	if len(models) != 2 || models[0] != "qwen3-32b" {
		t.Errorf("models are %v, want the ones the host listed", models)
	}
	if path != "/v1/models" {
		t.Errorf("asked %q, want /v1/models", path)
	}
	if authorization != "Bearer secret" {
		t.Errorf("Authorization is %q, want the host's key", authorization)
	}
}

func TestAProbeOfAHostThatIsDownFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := server.URL
	server.Close()

	if _, err := lmstudio.Probe(context.Background(), base, ""); err == nil {
		t.Error("Probe of a closed server succeeded, want it to fail")
	}
}

func TestAProbeOfAHostThatRefusesFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if _, err := lmstudio.Probe(context.Background(), server.URL, ""); err == nil {
		t.Error("Probe of a refusing server succeeded, want it to fail")
	}
}

func TestAProbeGivesUpWhenItsTimeRunsOut(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := lmstudio.Probe(ctx, server.URL, ""); err == nil {
		t.Error("Probe of a silent server succeeded, want it to give up")
	}
}

func TestAProbeKeepsAKeyOffTheNetworkInClear(t *testing.T) {
	t.Parallel()

	_, err := lmstudio.Probe(context.Background(), "http://192.0.2.1:1234", "secret")
	if err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("Probe returned %v, want a key over plain http to another machine refused", err)
	}
}
