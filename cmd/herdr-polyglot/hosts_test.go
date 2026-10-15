package main

import (
	"strings"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

func twoMachines() config.Settings {
	return config.Settings{
		Options: translation.Options{TargetLanguage: "EN-US"},
		Hosts: []config.Host{
			{Name: "laptop", Base: "http://localhost:1234"},
			{Name: "gpu", Base: "http://192.168.1.20:1234", Model: "qwen3-32b", APIKey: "gpu-key"},
		},
	}
}

func TestTheFirstHostTranslatesUntilAnotherIsChosen(t *testing.T) {
	settings := twoMachines()
	settings.StateDir = t.TempDir()

	hosts := newHosts(&settings)
	if hosts.Current() != "laptop" {
		t.Errorf("the host is %q, want the first one written", hosts.Current())
	}
	if got := hosts.applied().Options.Endpoint; got != "http://localhost:1234/v1/chat/completions" {
		t.Errorf("the endpoint is %q, want the laptop's", got)
	}
}

func TestTheChosenHostIsRememberedForTheNextPopup(t *testing.T) {
	settings := twoMachines()
	settings.StateDir = t.TempDir()

	newHosts(&settings).choose("gpu")
	hosts := newHosts(&settings)

	if hosts.Current() != "gpu" {
		t.Fatalf("the host is %q, want the one chosen last time", hosts.Current())
	}
	applied := hosts.applied()
	if applied.Options.Endpoint != "http://192.168.1.20:1234/v1/chat/completions" ||
		applied.Model.Name != "qwen3-32b" || applied.Options.APIKey != "gpu-key" {
		t.Errorf("applied %+v and %+v, want the gpu's endpoint, model and key",
			applied.Options, applied.Model)
	}
	if label := hosts.label("lmstudio"); label != "lmstudio · gpu" {
		t.Errorf("the service is called %q, want the host named", label)
	}
}

func TestAHostNoLongerWrittenDownFallsBackToTheFirst(t *testing.T) {
	settings := twoMachines()
	settings.StateDir = t.TempDir()
	newHosts(&settings).choose("gpu")

	settings.Hosts = settings.Hosts[:1]
	if got := newHosts(&settings).Current(); got != "laptop" {
		t.Errorf("the host is %q, want the first now the chosen one is gone", got)
	}
}

func TestWithoutHostsNothingIsAppliedOrNamed(t *testing.T) {
	settings := config.Settings{Options: translation.Options{Endpoint: "http://box:1234/v1/chat/completions"}}

	hosts := newHosts(&settings)
	if got := hosts.applied().Options.Endpoint; got != "http://box:1234/v1/chat/completions" {
		t.Errorf("the endpoint is %q, want the one configured", got)
	}
	if label := hosts.label("lmstudio"); label != "lmstudio" {
		t.Errorf("the service is called %q, want only its name", label)
	}
	if len(hosts.List()) != 1 {
		t.Errorf("the list is %+v, want the one machine", hosts.List())
	}
}

func TestHostsAreOnlyForLMStudio(t *testing.T) {
	settings := twoMachines()
	settings.Provider = "deepl"
	settings.Options.APIKey = "key-123"

	chosen := chooseService(newHosts(&settings).applied())
	if chosen.trouble == nil || !strings.Contains(chosen.trouble.Error(), "HERDR_POLYGLOT_LMSTUDIO_HOSTS") {
		t.Errorf("the trouble is %v, want the hosts refused by name", chosen.trouble)
	}
}

// A host's key is the host's, not a key left lying around: the model on that
// host still translates, with no service named.
func TestAHostWithAKeyStillTranslatesWithTheLocalModel(t *testing.T) {
	settings := twoMachines()
	settings.Read.Language = "DE"
	settings.Hosts[1].Base = "https://gpu.lan:1234"
	hosts := newHosts(&settings)
	hosts.choose("gpu")

	for _, chosen := range []service{
		chooseService(hosts.applied()),
		chooseReplyService(hosts.applied()),
	} {
		if chosen.trouble != nil || chosen.name != "lmstudio" {
			t.Errorf("chose %q with trouble %v, want lmstudio and none", chosen.name, chosen.trouble)
		}
	}
}
