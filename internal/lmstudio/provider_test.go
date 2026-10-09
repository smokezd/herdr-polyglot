package lmstudio_test

import (
	"testing"

	"github.com/wazum/herdr-polyglot/internal/lmstudio"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

func TestTheProviderNeedsNothingToBeBuilt(t *testing.T) {
	t.Parallel()

	translator, err := lmstudio.Provider{}.New(translation.Options{})
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	if endpoint := translator.(*lmstudio.Client).Endpoint(); endpoint != "http://localhost:1234/v1/chat/completions" {
		t.Errorf("the endpoint is %q, want LM Studio on this machine", endpoint)
	}
}

// Without a key there is nothing to keep off the wire, and a server elsewhere on
// the network was written down on purpose.
func TestAPlainEndpointElsewhereIsRefusedOnlyWithAKey(t *testing.T) {
	t.Parallel()
	lan := translation.Options{Endpoint: "http://192.168.1.20:1234/v1/chat/completions"}

	if _, err := (lmstudio.Provider{}).New(lan); err != nil {
		t.Errorf("a plain endpoint without a key was refused: %v", err)
	}

	lan.APIKey = "key-123"
	if _, err := (lmstudio.Provider{}).New(lan); err == nil {
		t.Error("a plain endpoint elsewhere was accepted with a key")
	}

	lan.Endpoint = "http://127.0.0.1:1234/v1/chat/completions"
	if _, err := (lmstudio.Provider{}).New(lan); err != nil {
		t.Errorf("a plain endpoint on this machine was refused with a key: %v", err)
	}
}

func TestAnEndpointThatIsNotHTTPIsRefused(t *testing.T) {
	t.Parallel()

	if _, err := (lmstudio.Provider{}).New(translation.Options{Endpoint: "ftp://localhost/"}); err == nil {
		t.Error("an ftp endpoint was accepted")
	}
}
