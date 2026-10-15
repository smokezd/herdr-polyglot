package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/wazum/herdr-polyglot/internal/config"
	"github.com/wazum/herdr-polyglot/internal/draft"
	"github.com/wazum/herdr-polyglot/internal/lmstudio"
	"github.com/wazum/herdr-polyglot/internal/overlay"
)

// lmHosts are the machines LM Studio is written down for, and which of them was
// chosen last. The choice is the plugin's, not a pane's: which machine is up
// depends on where the author is, not on which agent they talk to.
type lmHosts struct {
	settings *config.Settings
	current  string
	kept     draft.Slot
}

func newHosts(settings *config.Settings) *lmHosts {
	hosts := &lmHosts{
		settings: settings,
		kept:     draft.NewStore(settings.StateDir).Of("lmstudio-host").For("all"),
	}
	if len(settings.Hosts) == 0 {
		return hosts
	}
	hosts.current = settings.Hosts[0].Name
	remembered, _ := hosts.kept.Load()
	if _, found := hosts.find(strings.TrimSpace(remembered)); found {
		hosts.current = strings.TrimSpace(remembered)
	}
	return hosts
}

func (h *lmHosts) find(name string) (config.Host, bool) {
	for _, host := range h.settings.Hosts {
		if host.Name == name {
			return host, true
		}
	}
	return config.Host{}, false
}

// List is every host, or the one machine there is when none are named.
func (h *lmHosts) List() []overlay.Host {
	if len(h.settings.Hosts) == 0 {
		address := h.settings.Options.Endpoint
		if address == "" {
			address = "http://localhost:1234"
		}
		return []overlay.Host{{Address: address, Model: h.settings.Model.Name}}
	}
	listed := make([]overlay.Host, 0, len(h.settings.Hosts))
	for _, host := range h.settings.Hosts {
		listed = append(listed, overlay.Host{Name: host.Name, Address: host.Base, Model: host.Model})
	}
	return listed
}

func (h *lmHosts) Current() string { return h.current }

func (h *lmHosts) Probe(ctx context.Context, name string) ([]string, error) {
	host, found := h.find(name)
	if !found {
		return nil, fmt.Errorf("no host named %q", name)
	}
	return lmstudio.Probe(ctx, host.Base, host.APIKey)
}

// choose makes name the host from now on, in this popup and the next ones.
// Failing to remember it costs only the next popup, so it is not worth more
// than a line in herdr's log.
func (h *lmHosts) choose(name string) {
	h.current = name
	if err := h.kept.Save(name); err != nil {
		fmt.Fprintln(os.Stderr, "polyglot: the chosen host could not be remembered:", err)
	}
}

// applied is the settings with the chosen host's address, key and model in
// place of the ones for a single machine. Any other service is left the settings
// as they are, and refuses the hosts itself.
func (h *lmHosts) applied() *config.Settings {
	settings := *h.settings
	host, found := h.find(h.current)
	if name, err := chosenName(&settings); !found || err != nil || name != (lmstudio.Provider{}).Name() {
		return &settings
	}
	// Named, so the host's key is not taken for one left over without a service.
	settings.Provider = lmstudio.Provider{}.Name()
	settings.Options.Endpoint = lmstudio.ChatEndpoint(host.Base)
	settings.Options.APIKey = host.APIKey
	settings.Model.Name = host.Model
	return &settings
}

// label names the host along with the service once there is more than one to
// tell apart.
func (h *lmHosts) label(service string) string {
	if len(h.settings.Hosts) < 2 || service != (lmstudio.Provider{}).Name() {
		return service
	}
	return service + " · " + h.current
}
