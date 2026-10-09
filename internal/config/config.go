// Package config resolves the plugin's settings from the environment herdr
// provides and from the .env file in the plugin's own config directory. It
// stays neutral about which translation service is used.
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wazum/herdr-polyglot/internal/translation"
)

const (
	targetVar    = "HERDR_POLYGLOT_TARGET"
	providerVar  = "HERDR_POLYGLOT_PROVIDER"
	apiKeyVar    = "HERDR_POLYGLOT_API_KEY"
	languageVar  = "HERDR_POLYGLOT_LANGUAGE"
	endpointVar  = "HERDR_POLYGLOT_ENDPOINT"
	commandVar   = "HERDR_POLYGLOT_COMMAND"
	modelVar     = "HERDR_POLYGLOT_MODEL"
	systemVar    = "HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA"
	userVar      = "HERDR_POLYGLOT_USER_PROMPT_EXTRA"
	requestVar   = "HERDR_POLYGLOT_REQUEST_EXTRA"
	submitVar    = "HERDR_POLYGLOT_SUBMIT"
	vimVar       = "HERDR_POLYGLOT_VIM"
	liveVar      = "HERDR_POLYGLOT_LIVE"
	keepDraftVar = "HERDR_POLYGLOT_KEEP_DRAFT"
	confirmVar   = "HERDR_POLYGLOT_CONFIRM"
	maxDraftVar  = "HERDR_POLYGLOT_MAX_DRAFT"
	pulseVar     = "HERDR_POLYGLOT_PULSE"
	logoVar      = "HERDR_POLYGLOT_LOGO"
	modeVar      = "HERDR_POLYGLOT_MODE"
	readLangVar  = "HERDR_POLYGLOT_READ_LANGUAGE"
	readShowVar  = "HERDR_POLYGLOT_READ_REPLIES"
	readCtxVar   = "HERDR_POLYGLOT_READ_CONTEXT"
	widthVar     = "HERDR_POLYGLOT_WIDTH"
	heightVar    = "HERDR_POLYGLOT_HEIGHT"
	readWideVar  = "HERDR_POLYGLOT_READ_WIDTH"
	readHighVar  = "HERDR_POLYGLOT_READ_HEIGHT"
	stateDirVar  = "HERDR_PLUGIN_STATE_DIR"
	configDirVar = "HERDR_PLUGIN_CONFIG_DIR"
	binaryVar    = "HERDR_BIN_PATH"

	defaultLanguage = "EN-US"
	defaultBinary   = "herdr"
	dotenvName      = ".env"

	// ReadMode is what HERDR_POLYGLOT_MODE says when the popup shows the agent's
	// replies instead of taking a prompt.
	ReadMode = "read"

	defaultReadReplies = 1
	defaultReadContext = 2
)

type Settings struct {
	Target string
	// Provider names the translation service; empty means the default.
	Provider string
	Options  translation.Options
	// Model is only for a service that is a language model, so it stays out of
	// the options every service is handed.
	Model      ModelSettings
	ConfigFile string
	// StateDir is where an unfinished prompt is kept between sessions.
	StateDir    string
	HerdrBinary string
	Submit      bool
	Vim         bool
	Live        bool
	KeepDraft   bool
	Confirm     bool
	Pulse       bool
	Logo        bool
	MaxDraft    int
	// Reading is true when the popup shows the agent's replies, translated,
	// instead of taking a prompt.
	Reading bool
	Read    ReadSettings
	// Prompt is the size of the popup a prompt is written in.
	Prompt Size
}

// Size is how big a popup is asked to be. Each side is a number of cells or a
// share of the screen such as "80%", and nil leaves it to the plugin.
type Size struct {
	Width  any
	Height any
}

// ReadSettings say what the agent's replies are translated into and how many
// of them are read: Replies are translated and shown, Context more before them
// only go along so the translation keeps the same words.
type ReadSettings struct {
	// Language is empty until it is set: there is no language to assume a
	// reader wants the agent's replies in.
	Language string
	Replies  int
	Context  int
	// Size is the popup's, which wants more room than a prompt's.
	Size Size
}

// ModelSettings say which model translates and what to add to what it is sent.
// Empty means the service's own choice.
type ModelSettings struct {
	Name              string
	SystemPromptExtra string
	UserPromptExtra   string
	RequestExtra      map[string]any
}

// The environment wins over the .env file, so a one-off invocation can
// override stored settings. Credentials are passed along unchecked: only the
// service knows what it needs.
func Load(getenv func(string) string) (Settings, error) {
	// Without the directory herdr sets aside there is nothing of ours to read.
	// Falling back to a relative path would let a .env in whatever directory
	// the process started in decide the settings.
	configFile := ""
	stored := map[string]string{}
	if configDir := getenv(configDirVar); configDir != "" {
		configFile = filepath.Join(configDir, dotenvName)
		stored = readDotenv(configFile)
	}

	lookup := func(key string) string {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
		return stored[key]
	}

	provider := lookup(providerVar)

	// A setting that cannot be read is not quietly taken as off: a typo in
	// HERDR_POLYGLOT_SUBMIT would otherwise send every prompt to the agent.
	given := &reading{lookup: lookup}
	settings := Settings{
		Target:      lookup(targetVar),
		Provider:    provider,
		ConfigFile:  configFile,
		StateDir:    getenv(stateDirVar),
		HerdrBinary: orDefault(lookup(binaryVar), defaultBinary),
		Submit:      given.flag(submitVar, true),
		Vim:         given.flag(vimVar, false),
		Live:        given.flag(liveVar, false),
		KeepDraft:   given.flag(keepDraftVar, true),
		Confirm:     given.flag(confirmVar, false),
		Pulse:       given.flag(pulseVar, true),
		Logo:        given.flag(logoVar, true),
		MaxDraft:    given.number(maxDraftVar, "characters"),
		Reading:     lookup(modeVar) == ReadMode,
		Read: ReadSettings{
			Language: lookup(readLangVar),
			Replies:  given.count(readShowVar, defaultReadReplies, 1),
			Context:  given.count(readCtxVar, defaultReadContext, 0),
			Size:     Size{Width: given.extent(readWideVar), Height: given.extent(readHighVar)},
		},
		Prompt: Size{Width: given.extent(widthVar), Height: given.extent(heightVar)},
		Options: translation.Options{
			APIKey:         orDefault(lookup(scopedKeyVar(provider)), lookup(apiKeyVar)),
			TargetLanguage: orDefault(lookup(languageVar), defaultLanguage),
			Endpoint:       lookup(endpointVar),
			Command:        lookup(commandVar),
		},
		Model: ModelSettings{
			Name:              lookup(modelVar),
			SystemPromptExtra: lookup(systemVar),
			UserPromptExtra:   lookup(userVar),
			RequestExtra:      given.object(requestVar),
		},
	}

	if given.err != nil {
		return Settings{}, given.err
	}
	if settings.Target == "" {
		return Settings{}, fmt.Errorf("no target pane: %s is not set", targetVar)
	}
	return settings, nil
}

// reading takes the settings apart, keeping the first value it could not make
// sense of so that Load can refuse it.
type reading struct {
	lookup func(string) string
	err    error
}

func (r *reading) flag(variable string, whenUnset bool) bool {
	value := strings.TrimSpace(r.lookup(variable))
	switch strings.ToLower(value) {
	case "":
		return whenUnset
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		r.refuse(variable, value, "neither on (1, true, yes, on) nor off (0, false, no, off)")
		return whenUnset
	}
}

// A zero means no number was given and the overlay picks its own.
func (r *reading) number(variable, unit string) int {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return 0
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < 0 {
		r.refuse(variable, value, "not a whole number of "+unit)
		return 0
	}
	return number
}

// count reads a number of replies, which has a default other than zero and
// may have to be at least one.
func (r *reading) count(variable string, whenUnset, least int) int {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return whenUnset
	}

	number, err := strconv.Atoi(value)
	if err != nil || number < least {
		r.refuse(variable, value, fmt.Sprintf("not a whole number of replies from %d up", least))
		return whenUnset
	}
	return number
}

// extent reads one side of a popup: a whole number of cells, or a percentage
// of the screen, which herdr takes as it is written.
func (r *reading) extent(variable string) any {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return nil
	}

	if share, isShare := strings.CutSuffix(value, "%"); isShare {
		percent, err := strconv.Atoi(strings.TrimSpace(share))
		if err != nil || percent < 1 || percent > 100 {
			r.refuse(variable, value, "not a share of the screen from 1% to 100%")
			return nil
		}
		return strconv.Itoa(percent) + "%"
	}

	cells, err := strconv.Atoi(value)
	if err != nil || cells < minExtent {
		r.refuse(variable, value, fmt.Sprintf("neither a number of cells from %d up nor a share such as 80%%", minExtent))
		return nil
	}
	return cells
}

// minExtent is the least either side may be: smaller than this, a popup has
// no room for its own heading and footer.
const minExtent = 10

// object reads a JSON object to add to a request. The draft and the way the
// answer comes back are not the setting's to change: the one is what is being
// translated, the other is what the answer is read from.
func (r *reading) object(variable string) map[string]any {
	value := strings.TrimSpace(r.lookup(variable))
	if value == "" {
		return nil
	}

	var object map[string]any
	if err := json.Unmarshal([]byte(value), &object); err != nil || object == nil {
		r.refuse(variable, value, "not a JSON object")
		return nil
	}
	for _, reserved := range []string{"messages", "stream"} {
		if _, given := object[reserved]; given {
			r.refuse(variable, value, "setting "+reserved+", which is not the setting's to change")
			return nil
		}
	}
	return object
}

func (r *reading) refuse(variable, value, why string) {
	if r.err == nil {
		r.err = fmt.Errorf("%s is %q, which is %s", variable, value, why)
	}
}

// Scoping the key by service lets several services be configured side by side.
func scopedKeyVar(provider string) string {
	if provider == "" {
		return ""
	}
	scope := strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(provider))
	return "HERDR_POLYGLOT_" + scope + "_API_KEY"
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// readDotenv parses KEY=VALUE lines, ignoring comments and blanks. A missing
// file is normal: the plugin works from the environment alone.
func readDotenv(path string) map[string]string {
	values := map[string]string{}

	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close()

	lines := bufio.NewScanner(file)
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values
}
