package main

import (
	"context"
	"strings"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

// With nothing configured the draft goes no further than the machine: to the
// model loaded in LM Studio.
func TestWithNothingConfiguredTheLocalModelTranslates(t *testing.T) {
	chosen := chooseService(&config.Settings{})

	if chosen.name != "lmstudio" {
		t.Errorf("with nothing configured the service is %q, want lmstudio", chosen.name)
	}
	if chosen.trouble != nil {
		t.Errorf("with nothing configured there is trouble to report: %v", chosen.trouble)
	}
	if !chosen.translates {
		t.Error("with nothing configured the popup does not claim to translate")
	}
}

func TestThePlainDraftBoxIsThereWhenAskedFor(t *testing.T) {
	chosen := chooseService(&config.Settings{Provider: "off"})

	if chosen.translates {
		t.Error("with translation off the popup claims it translates")
	}

	written := "Bitte behebe den fehlschlagenden Test"
	delivered, err := chosen.translator.Translate(context.Background(), written)
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if delivered != written {
		t.Errorf("Translate returned %q, want the draft as it was written", delivered)
	}
}

// A key may be left over from before, and a draft sent on the strength of it
// would reach a service nobody asked for this time.
func TestAKeyOnItsOwnIsNotEnoughToSendTheDraftAway(t *testing.T) {
	chosen := chooseService(&config.Settings{
		Options: translation.Options{APIKey: "key-123", TargetLanguage: "EN-US"},
	})

	if chosen.trouble == nil {
		t.Fatal("a key on its own chose a service")
	}
	if !strings.Contains(chosen.trouble.Error(), "HERDR_POLYGLOT_PROVIDER") {
		t.Errorf("the trouble is %v, want it to say which setting decides", chosen.trouble)
	}
	if _, err := chosen.translator.Translate(context.Background(), "Bitte behebe es"); err == nil {
		t.Error("something was translated with a key nobody named a service for")
	}
}

func TestANamedServiceUsesItsKey(t *testing.T) {
	chosen := chooseService(&config.Settings{
		Provider: "deepl",
		Options:  translation.Options{APIKey: "key-123", TargetLanguage: "EN-US"},
	})

	if chosen.name != "deepl" {
		t.Errorf("the service is %q, want the one named", chosen.name)
	}
	if chosen.trouble != nil {
		t.Errorf("a named service with its key reports trouble: %v", chosen.trouble)
	}
}

// A model setting means nothing to a service that is not a model, and quietly
// ignoring it would leave someone wondering why their model never changed.
func TestModelSettingsForAnotherServiceAreRefused(t *testing.T) {
	for name, model := range map[string]config.ModelSettings{
		"HERDR_POLYGLOT_MODEL":               {Name: "qwen"},
		"HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA": {SystemPromptExtra: "British spelling"},
		"HERDR_POLYGLOT_USER_PROMPT_EXTRA":   {UserPromptExtra: "/no_think"},
		"HERDR_POLYGLOT_REQUEST_EXTRA":       {RequestExtra: map[string]any{"top_p": 0.9}},
	} {
		chosen := chooseService(&config.Settings{
			Provider: "deepl",
			Options:  translation.Options{APIKey: "key-123"},
			Model:    model,
		})

		if chosen.trouble == nil || !strings.Contains(chosen.trouble.Error(), name) {
			t.Errorf("%s with deepl gave trouble %v, want it refused by name", name, chosen.trouble)
		}
	}
}

func TestModelSettingsWaitWhileTranslationIsOff(t *testing.T) {
	for _, name := range []string{"off", "dry-run"} {
		chosen := chooseService(&config.Settings{
			Provider: name,
			Model:    config.ModelSettings{Name: "qwen", UserPromptExtra: "/no_think"},
		})

		if chosen.trouble != nil {
			t.Errorf("model settings with %s gave trouble %v, want them left alone", name, chosen.trouble)
		}
	}
}

func TestModelSettingsAreTakenByTheLocalModel(t *testing.T) {
	chosen := chooseService(&config.Settings{
		Model: config.ModelSettings{Name: "qwen", UserPromptExtra: "/no_think"},
	})

	if chosen.name != "lmstudio" || chosen.trouble != nil {
		t.Errorf("model settings chose %q with trouble %v, want lmstudio without", chosen.name, chosen.trouble)
	}
}

// Asking for a service that cannot be built is not the same as asking for none:
// the draft must never reach an agent untranslated because a key was missing.
func TestAServiceThatCannotBeBuiltSaysSoAndRefusesToTranslate(t *testing.T) {
	chosen := chooseService(&config.Settings{
		Provider:   "deepl",
		ConfigFile: "/somewhere/.env",
	})

	if chosen.trouble == nil {
		t.Fatal("a service that cannot be built reports no trouble")
	}
	if !strings.Contains(chosen.trouble.Error(), "/somewhere/.env") {
		t.Errorf("the trouble is %v, want it to say where the key belongs", chosen.trouble)
	}
	if !chosen.translates {
		t.Error("the popup pretends translation is off when the service is only broken")
	}

	if _, err := chosen.translator.Translate(context.Background(), "Bitte behebe es"); err == nil {
		t.Error("the broken service translated something instead of saying why it cannot")
	}
}

// A command needs no key, so setting one is enough to choose it: there is nothing
// else it could mean.
func TestACommandOnItsOwnPicksTheLocalService(t *testing.T) {
	chosen := chooseService(&config.Settings{
		Options: translation.Options{Command: "sed 's/behebe/fix/'", TargetLanguage: "EN-US"},
	})

	if chosen.name != "cmd" {
		t.Errorf("with a command the service is %q, want cmd", chosen.name)
	}
	if !chosen.translates {
		t.Error("with a command the popup does not claim to translate")
	}
	if chosen.trouble != nil {
		t.Errorf("with a command there is trouble to report: %v", chosen.trouble)
	}

	translated, err := chosen.translator.Translate(context.Background(), "Bitte behebe es")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "Bitte fix es" {
		t.Errorf("Translate returned %q, want what the command answered", translated)
	}
}

// A key and a command together say nothing about which was meant, and guessing
// wrong would send a draft to a service the person thought they had left behind.
func TestAKeyAndACommandTogetherAreRefusedRatherThanGuessedAt(t *testing.T) {
	chosen := chooseService(&config.Settings{
		Options: translation.Options{APIKey: "key-123", Command: "cat"},
	})

	if chosen.trouble == nil {
		t.Fatal("a key and a command together report no trouble")
	}
	if !strings.Contains(chosen.trouble.Error(), "HERDR_POLYGLOT_PROVIDER") {
		t.Errorf("the trouble is %v, want it to say which setting decides", chosen.trouble)
	}
	if _, err := chosen.translator.Translate(context.Background(), "Bitte behebe es"); err == nil {
		t.Error("something was translated while it is unclear by what")
	}
}

func TestAnExplicitServiceIsUsedAsAsked(t *testing.T) {
	chosen := chooseService(&config.Settings{Provider: "dry-run"})

	if chosen.name != "dry-run" {
		t.Errorf("the service is %q, want the one asked for", chosen.name)
	}
	translated, err := chosen.translator.Translate(context.Background(), "Bitte behebe es")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if !strings.Contains(translated, "dry-run") {
		t.Errorf("Translate returned %q, want the dry-run marker", translated)
	}
}

func TestRepliesAreNotTranslatedUntilTheirLanguageIsNamed(t *testing.T) {
	t.Parallel()

	chosen := chooseReplyService(&config.Settings{Provider: "dry-run", ConfigFile: "/cfg/.env"})

	if chosen.trouble == nil || !strings.Contains(chosen.trouble.Error(), "HERDR_POLYGLOT_READ_LANGUAGE") ||
		!strings.Contains(chosen.trouble.Error(), "/cfg/.env") {
		t.Fatalf("trouble is %v, want the missing setting and where it goes", chosen.trouble)
	}
	if _, err := chosen.translator.Translate(context.Background(), "a reply"); err == nil {
		t.Error("a reply was translated without a language to translate it into")
	}
}

func TestRepliesGoThroughTheSameServiceIntoTheirOwnLanguage(t *testing.T) {
	t.Parallel()
	settings := &config.Settings{
		Options: translation.Options{
			Command:        `printf '%s' "$POLYGLOT_TARGET_LANGUAGE"`,
			TargetLanguage: "EN-US",
		},
		Read: config.ReadSettings{Language: "DE", Replies: 1},
	}

	chosen := chooseReplyService(settings)

	if chosen.name != "cmd" || chosen.trouble != nil {
		t.Fatalf("chose %q (trouble %v), want the service a prompt goes through", chosen.name, chosen.trouble)
	}
	translated, err := chosen.translator.Translate(context.Background(), "a reply")
	if err != nil {
		t.Fatalf("Translate returned unexpected error: %v", err)
	}
	if translated != "DE" {
		t.Errorf("translated into %q, want DE", translated)
	}
	if settings.Options.TargetLanguage != "EN-US" {
		t.Errorf("the prompt language became %q, want it left alone", settings.Options.TargetLanguage)
	}
}
