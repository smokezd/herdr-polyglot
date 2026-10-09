package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/wazum/herdr-polyglot/internal/command"
	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/lmstudio"
	"github.com/wazum/herdr-polyglot/internal/translation"
)

type service struct {
	name       string
	translator translation.Translator
	// translates says the popup has something to translate with, so it offers the
	// keys and the second panel that go with it.
	translates bool
	// trouble is a service that was asked for and could not be built, which the
	// popup says out loud.
	trouble error
}

// chooseService decides what the draft goes through. Nothing leaves the machine
// unless a service elsewhere is named: a command on its own means the program it
// names, and no choice at all means the model loaded in LM Studio.
func chooseService(settings *config.Settings) service {
	registry := services(settings.Model)

	name, err := chosenName(settings)
	if err == nil {
		err = onlyForModels(name, settings.Model)
	}
	if err == nil {
		var translator translation.Translator
		translator, err = registry.Translator(name, settings.Options)
		if err == nil {
			return service{
				name:       name,
				translator: translator,
				translates: name != translation.OffProvider{}.Name(),
			}
		}
	}

	if settings.ConfigFile != "" {
		err = fmt.Errorf("%w; configure it in %s", err, settings.ConfigFile)
	}
	return service{
		name:       name,
		translator: translation.Broken{Why: err},
		translates: true,
		trouble:    err,
	}
}

// A key and a command are two answers to the same question, and picking one of
// them would send the draft somewhere it was not meant to go. A key on its own is
// not an answer either: it may be left over, and a draft sent on the strength of
// it would reach a service nobody asked for this time.
func chosenName(settings *config.Settings) (string, error) {
	if settings.Provider != "" {
		return settings.Provider, nil
	}

	hasKey, hasCommand := settings.Options.APIKey != "", settings.Options.Command != ""
	switch {
	case hasKey && hasCommand:
		return "", errors.New(
			"there is both an API key and a command, so it is unclear which translates: " +
				"choose one with HERDR_POLYGLOT_PROVIDER")
	case hasKey:
		return "", errors.New(
			"an API key is set but no service is named, and drafts leave the machine only " +
				"when asked to: set HERDR_POLYGLOT_PROVIDER=deepl (or google) to use it")
	case hasCommand:
		return command.Provider{}.Name(), nil
	default:
		return lmstudio.Provider{}.Name(), nil
	}
}

// Settings for a language model mean nothing to another service, and quietly
// ignoring them would leave someone wondering why their model never changed.
// Turning translation off, or trying it dry, is not choosing another service:
// the settings wait for the model to be used again.
func onlyForModels(name string, model config.ModelSettings) error {
	switch name {
	case lmstudio.Provider{}.Name(), translation.OffProvider{}.Name(), translation.DryRunProvider{}.Name():
		return nil
	}

	given := map[string]bool{
		"HERDR_POLYGLOT_MODEL":               model.Name != "",
		"HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA": model.SystemPromptExtra != "",
		"HERDR_POLYGLOT_USER_PROMPT_EXTRA":   model.UserPromptExtra != "",
		"HERDR_POLYGLOT_REQUEST_EXTRA":       len(model.RequestExtra) > 0,
	}
	for _, variable := range slices.Sorted(maps.Keys(given)) {
		if given[variable] {
			return fmt.Errorf("%s applies only to %s, not to %s",
				variable, lmstudio.Provider{}.Name(), name)
		}
	}
	return nil
}
