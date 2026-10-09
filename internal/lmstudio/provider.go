package lmstudio

import (
	"fmt"
	"net"
	"net/url"

	"github.com/wazum/herdr-polyglot/internal/translation"
)

// Provider carries what only a language model needs, set where the services are
// listed, so the options every service is handed stay the same for all of them.
type Provider struct {
	Model             string
	SystemPromptExtra string
	UserPromptExtra   string
	RequestExtra      map[string]any
}

func (Provider) Name() string { return "lmstudio" }

// Nothing is required: with no settings at all this is the model loaded in LM
// Studio on this machine.
func (p Provider) New(options translation.Options) (translation.Translator, error) {
	if err := requireSecureEndpoint(options.Endpoint, options.APIKey != ""); err != nil {
		return nil, err
	}

	clientOptions := []Option{
		WithAPIKey(options.APIKey),
		WithModel(p.Model),
		WithSystemPromptExtra(p.SystemPromptExtra),
		WithUserPromptExtra(p.UserPromptExtra),
		WithRequestExtra(p.RequestExtra),
	}
	if options.TargetLanguage != "" {
		clientOptions = append(clientOptions, WithTargetLanguage(options.TargetLanguage))
	}
	if options.Endpoint != "" {
		clientOptions = append(clientOptions, WithEndpoint(options.Endpoint))
	}
	return New(clientOptions...), nil
}

// requireSecureEndpoint keeps a key off the wire in clear. Without one, a server
// elsewhere on the network is the author's own choice to make: the endpoint was
// written down on purpose.
func requireSecureEndpoint(endpoint string, withKey bool) error {
	if endpoint == "" {
		return nil
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("endpoint %q is not a URL: %w", endpoint, err)
	}
	switch {
	case parsed.Scheme == "https":
		return nil
	case parsed.Scheme != "http":
		return fmt.Errorf("endpoint %q is neither http nor https", endpoint)
	case !withKey || isLoopback(parsed.Hostname()):
		return nil
	}
	return fmt.Errorf("endpoint %q must use https, or the API key travels in clear", endpoint)
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
