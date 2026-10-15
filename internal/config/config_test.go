package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wazum/herdr-polyglot/internal/config"
)

func envFrom(pairs map[string]string) func(string) string {
	return func(key string) string { return pairs[key] }
}

func configDirContaining(t *testing.T, dotenv string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	return dir
}

func TestLoadTakesTheTargetAndCredentialsFromTheEnvironment(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":  "w1:p3",
		"HERDR_POLYGLOT_API_KEY": "key-123",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Target != "w1:p3" {
		t.Errorf("Target is %q, want w1:p3", settings.Target)
	}
	if settings.Options.APIKey != "key-123" {
		t.Errorf("APIKey is %q, want key-123", settings.Options.APIKey)
	}
	if settings.Provider != "" {
		t.Errorf("Provider is %q, want it left to the composition root", settings.Provider)
	}
	if !settings.Submit {
		t.Error("Submit is false, want submitting by default")
	}
	if settings.Options.TargetLanguage != "EN-US" {
		t.Errorf("TargetLanguage is %q, want EN-US by default", settings.Options.TargetLanguage)
	}
	if settings.HerdrBinary != "herdr" {
		t.Errorf("HerdrBinary is %q, want herdr by default", settings.HerdrBinary)
	}
}

func TestLoadHonoursTheChosenServiceAndItsOptions(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":   "w1:p3",
		"HERDR_POLYGLOT_PROVIDER": "dry-run",
		"HERDR_POLYGLOT_LANGUAGE": "EN-GB",
		"HERDR_POLYGLOT_ENDPOINT": "https://translate.example/v2",
		"HERDR_POLYGLOT_SUBMIT":   "0",
		"HERDR_BIN_PATH":          "/opt/homebrew/bin/herdr",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Provider != "dry-run" {
		t.Errorf("Provider is %q, want dry-run", settings.Provider)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want EN-GB", settings.Options.TargetLanguage)
	}
	if settings.Options.Endpoint != "https://translate.example/v2" {
		t.Errorf("Endpoint is %q, want the configured one", settings.Options.Endpoint)
	}
	if settings.Submit {
		t.Error("Submit is true, want it disabled by HERDR_POLYGLOT_SUBMIT=0")
	}
	if settings.HerdrBinary != "/opt/homebrew/bin/herdr" {
		t.Errorf("HerdrBinary is %q, want the path from HERDR_BIN_PATH", settings.HerdrBinary)
	}
}

// A command line is a setting like any other, and it keeps its quoting: it is a
// shell that runs it, not the plugin.
func TestLoadKeepsTheTranslationCommandAsItWasWritten(t *testing.T) {
	t.Parallel()
	written := `translateLocally -m de-en-base | sed 's/  */ /g'`

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":  "w1:p3",
		"HERDR_POLYGLOT_COMMAND": written,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.Command != written {
		t.Errorf("Command is %q, want it as it was written", settings.Options.Command)
	}
}

func TestLoadReadsSettingsFromTheDotEnvInThePluginConfigDirectory(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "# credentials\nHERDR_POLYGLOT_API_KEY=key-from-file\nHERDR_POLYGLOT_LANGUAGE=\"EN-GB\"\n")

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":   "w1:p3",
		"HERDR_PLUGIN_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "key-from-file" {
		t.Errorf("APIKey is %q, want the value from the .env file", settings.Options.APIKey)
	}
	if settings.Options.TargetLanguage != "EN-GB" {
		t.Errorf("TargetLanguage is %q, want the unquoted value from the .env file", settings.Options.TargetLanguage)
	}
	if settings.ConfigFile != filepath.Join(configDir, ".env") {
		t.Errorf("ConfigFile is %q, want the .env path so callers can point users at it", settings.ConfigFile)
	}
}

func TestAServiceSpecificKeyWinsOverTheGenericOne(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "HERDR_POLYGLOT_API_KEY=generic-key\nHERDR_POLYGLOT_ACME_API_KEY=acme-key\n")

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":   "w1:p3",
		"HERDR_POLYGLOT_PROVIDER": "acme",
		"HERDR_PLUGIN_CONFIG_DIR": configDir,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "acme-key" {
		t.Errorf("APIKey is %q, want the key scoped to the chosen service", settings.Options.APIKey)
	}
}

func TestTheEnvironmentWinsOverTheDotEnvFile(t *testing.T) {
	t.Parallel()
	configDir := configDirContaining(t, "HERDR_POLYGLOT_API_KEY=key-from-file\n")

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":   "w1:p3",
		"HERDR_PLUGIN_CONFIG_DIR": configDir,
		"HERDR_POLYGLOT_API_KEY":  "key-from-environment",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "key-from-environment" {
		t.Errorf("APIKey is %q, want the environment to win", settings.Options.APIKey)
	}
}

func TestLoadLeavesMissingCredentialsToTheService(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Options.APIKey != "" {
		t.Errorf("APIKey is %q, want it empty", settings.Options.APIKey)
	}
}

func TestLoadWithoutATargetFails(t *testing.T) {
	t.Parallel()

	_, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_API_KEY": "key-123"}))

	if err == nil {
		t.Fatal("Load returned no error, want a complaint about the missing target")
	}
	if !strings.Contains(err.Error(), "HERDR_POLYGLOT_TARGET") {
		t.Errorf("error %q does not name the target variable", err)
	}
}

func TestVimBindingsAreOffUnlessAskedFor(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Vim {
		t.Error("Vim is true, want plain editing by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET": "w1:p3",
		"HERDR_POLYGLOT_VIM":    "1",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Vim {
		t.Error("Vim is false, want it enabled by HERDR_POLYGLOT_VIM=1")
	}
}

func TestLivePreviewIsOffUnlessAskedFor(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Live {
		t.Error("Live is true, want translating only on send by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET": "w1:p3",
		"HERDR_POLYGLOT_LIVE":   "1",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Live {
		t.Error("Live is false, want it enabled by HERDR_POLYGLOT_LIVE=1")
	}
}

func TestKeepingDraftsIsOnUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.KeepDraft {
		t.Error("KeepDraft is false, want an unfinished prompt kept by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":     "w1:p3",
		"HERDR_POLYGLOT_KEEP_DRAFT": "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.KeepDraft {
		t.Error("KeepDraft is true, want it turned off by HERDR_POLYGLOT_KEEP_DRAFT=0")
	}
}

func TestTheStateDirectoryIsPassedAlongForKeepingDrafts(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":  "w1:p3",
		"HERDR_PLUGIN_STATE_DIR": "/tmp/state",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.StateDir != "/tmp/state" {
		t.Errorf("StateDir is %q, want the directory herdr set aside", settings.StateDir)
	}
}

func TestConfirmingBeforeSendingIsOffUnlessAskedFor(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Confirm {
		t.Error("Confirm is true, want sending to stay one key by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":  "w1:p3",
		"HERDR_POLYGLOT_CONFIRM": "1",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Confirm {
		t.Error("Confirm is false, want it enabled by HERDR_POLYGLOT_CONFIRM=1")
	}
}

// Without the directory herdr sets aside, there is no plugin configuration to
// read; reading .env from wherever the process happens to run would let a file
// in a repository decide which binary gets executed.
func TestWithoutAConfigDirectoryNoDotEnvIsRead(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, ".env"),
		[]byte("HERDR_BIN_PATH=/tmp/planted\n"), 0o600); err != nil {
		t.Fatalf("writing the planted file: %v", err)
	}
	t.Chdir(directory)

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	if settings.HerdrBinary != "herdr" {
		t.Errorf("HerdrBinary is %q, want the planted file ignored", settings.HerdrBinary)
	}
	if settings.ConfigFile != "" {
		t.Errorf("ConfigFile is %q, want none without a config directory", settings.ConfigFile)
	}
}

func TestThePulseIsOnUnlessTurnedOff(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Pulse {
		t.Error("Pulse is false, want the live indicator to breathe by default")
	}

	settings, err = config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET": "w1:p3",
		"HERDR_POLYGLOT_PULSE":  "0",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Pulse {
		t.Error("Pulse is true, want it turned off by HERDR_POLYGLOT_PULSE=0")
	}
}

// A setting nobody can read is a setting nobody can trust: HERDR_POLYGLOT_SUBMIT=flase
// would otherwise send every prompt straight to the agent.
func TestAValueThatIsNeitherOnNorOffIsRefused(t *testing.T) {
	t.Parallel()

	for _, variable := range []string{
		"HERDR_POLYGLOT_SUBMIT",
		"HERDR_POLYGLOT_VIM",
		"HERDR_POLYGLOT_LIVE",
		"HERDR_POLYGLOT_CONFIRM",
		"HERDR_POLYGLOT_KEEP_DRAFT",
		"HERDR_POLYGLOT_PULSE",
	} {
		environment := map[string]string{
			"HERDR_POLYGLOT_TARGET": "w1:p1",
			variable:                "flase",
		}

		_, err := config.Load(envFrom(environment))
		if err == nil {
			t.Errorf("%s=flase was accepted, want it refused", variable)
			continue
		}
		if !strings.Contains(err.Error(), variable) {
			t.Errorf("%s=flase failed with %v, want the variable named", variable, err)
		}
	}
}

func TestADraftLimitThatIsNotANumberIsRefused(t *testing.T) {
	t.Parallel()

	_, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":    "w1:p1",
		"HERDR_POLYGLOT_MAX_DRAFT": "zweitausend",
	}))
	if err == nil || !strings.Contains(err.Error(), "HERDR_POLYGLOT_MAX_DRAFT") {
		t.Errorf("Load returned %v, want the unreadable draft limit refused by name", err)
	}
}

func TestLoadTakesTheSettingsForALanguageModel(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":              "w1:p1",
		"HERDR_POLYGLOT_MODEL":               "qwen/qwen3-8b",
		"HERDR_POLYGLOT_SYSTEM_PROMPT_EXTRA": "Use British spelling.",
		"HERDR_POLYGLOT_USER_PROMPT_EXTRA":   "/no_think",
		"HERDR_POLYGLOT_REQUEST_EXTRA":       `{"max_tokens": 2048, "temperature": 0.2}`,
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	model := settings.Model
	if model.Name != "qwen/qwen3-8b" {
		t.Errorf("Name is %q, want the one configured", model.Name)
	}
	if model.SystemPromptExtra != "Use British spelling." {
		t.Errorf("SystemPromptExtra is %q, want the one configured", model.SystemPromptExtra)
	}
	if model.UserPromptExtra != "/no_think" {
		t.Errorf("UserPromptExtra is %q, want the one configured", model.UserPromptExtra)
	}
	if model.RequestExtra["max_tokens"] != 2048.0 || model.RequestExtra["temperature"] != 0.2 {
		t.Errorf("RequestExtra is %v, want the fields configured", model.RequestExtra)
	}
}

// The .env reader takes quotes off both ends of a value, which a JSON object
// written there, quoted or not, must survive.
func TestARequestExtraInTheDotEnvFileIsReadWhole(t *testing.T) {
	t.Parallel()

	for _, written := range []string{
		`{"top_p":0.9,"stop":"x"}`,
		`'{"top_p":0.9,"stop":"x"}'`,
	} {
		settings, err := config.Load(envFrom(map[string]string{
			"HERDR_POLYGLOT_TARGET":   "w1:p1",
			"HERDR_PLUGIN_CONFIG_DIR": configDirContaining(t, "HERDR_POLYGLOT_REQUEST_EXTRA="+written+"\n"),
		}))
		if err != nil {
			t.Errorf("%s: Load returned unexpected error: %v", written, err)
			continue
		}
		if settings.Model.RequestExtra["stop"] != "x" {
			t.Errorf("%s: RequestExtra is %v, want it read whole", written, settings.Model.RequestExtra)
		}
	}
}

func TestARequestExtraThatCannotBeUsedIsRefused(t *testing.T) {
	t.Parallel()

	for _, written := range []string{
		`{"max_tokens": `,
		`[1, 2]`,
		`null`,
		`{"messages": []}`,
		`{"stream": true}`,
	} {
		_, err := config.Load(envFrom(map[string]string{
			"HERDR_POLYGLOT_TARGET":        "w1:p1",
			"HERDR_POLYGLOT_REQUEST_EXTRA": written,
		}))
		if err == nil || !strings.Contains(err.Error(), "HERDR_POLYGLOT_REQUEST_EXTRA") {
			t.Errorf("%s: Load returned %v, want it refused by name", written, err)
		}
	}
}

func TestLoadReadsOneReplyWithTwoBeforeItAndNoLanguageUnlessTold(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{"HERDR_POLYGLOT_TARGET": "w1:p3"}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if settings.Reading {
		t.Error("Reading is true, want the prompt box unless the mode says otherwise")
	}
	want := config.ReadSettings{Language: "", Replies: 1, Context: 2}
	if settings.Read != want {
		t.Errorf("Read is %+v, want %+v", settings.Read, want)
	}
}

func TestLoadTakesTheReadingSettingsFromTheDotenv(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET": "w1:p3",
		"HERDR_POLYGLOT_MODE":   "read",
		"HERDR_PLUGIN_CONFIG_DIR": configDirContaining(t,
			"HERDR_POLYGLOT_READ_LANGUAGE=DE\nHERDR_POLYGLOT_READ_REPLIES=3\nHERDR_POLYGLOT_READ_CONTEXT=0\n"),
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if !settings.Reading {
		t.Error("Reading is false, want it on for HERDR_POLYGLOT_MODE=read")
	}
	want := config.ReadSettings{Language: "DE", Replies: 3, Context: 0}
	if settings.Read != want {
		t.Errorf("Read is %+v, want %+v", settings.Read, want)
	}
}

func TestLoadRefusesACountOfRepliesItCannotUse(t *testing.T) {
	t.Parallel()

	for variable, value := range map[string]string{
		"HERDR_POLYGLOT_READ_REPLIES": "0",
		"HERDR_POLYGLOT_READ_CONTEXT": "-1",
	} {
		_, err := config.Load(envFrom(map[string]string{
			"HERDR_POLYGLOT_TARGET": "w1:p3",
			variable:                value,
		}))
		if err == nil || !strings.Contains(err.Error(), variable) {
			t.Errorf("%s=%s: error is %v, want one naming the setting", variable, value, err)
		}
	}
}

func TestAPopupIsSizedInCellsOrAsAShareOfTheScreen(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":      "w1:p3",
		"HERDR_POLYGLOT_WIDTH":       "120",
		"HERDR_POLYGLOT_READ_WIDTH":  "80%",
		"HERDR_POLYGLOT_READ_HEIGHT": " 90 % ",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if want := (config.Size{Width: 120}); settings.Prompt != want {
		t.Errorf("Prompt is %+v, want %+v", settings.Prompt, want)
	}
	if want := (config.Size{Width: "80%", Height: "90%"}); settings.Read.Size != want {
		t.Errorf("Read.Size is %+v, want %+v", settings.Read.Size, want)
	}
}

func TestLoadRefusesAPopupSizeItCannotUse(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"wide", "5", "0%", "150%", "-80"} {
		_, err := config.Load(envFrom(map[string]string{
			"HERDR_POLYGLOT_TARGET":     "w1:p3",
			"HERDR_POLYGLOT_READ_WIDTH": value,
		}))
		if err == nil || !strings.Contains(err.Error(), "HERDR_POLYGLOT_READ_WIDTH") {
			t.Errorf("%q: error is %v, want one naming the setting", value, err)
		}
	}
}

func TestLoadTakesTheLMStudioHostsInTheOrderWritten(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":                   "w1:p1",
		"HERDR_POLYGLOT_MODEL":                    "qwen/qwen3-8b",
		"HERDR_POLYGLOT_API_KEY":                  "shared",
		"HERDR_POLYGLOT_LMSTUDIO_HOSTS":           "laptop=http://localhost:1234, gpu-box=http://192.168.1.20:1234/v1/chat/completions",
		"HERDR_POLYGLOT_LMSTUDIO_GPU_BOX_MODEL":   "qwen3-32b",
		"HERDR_POLYGLOT_LMSTUDIO_GPU_BOX_API_KEY": "gpu-key",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	want := []config.Host{
		{Name: "laptop", Base: "http://localhost:1234", Model: "qwen/qwen3-8b", APIKey: "shared"},
		{Name: "gpu-box", Base: "http://192.168.1.20:1234", Model: "qwen3-32b", APIKey: "gpu-key"},
	}
	if len(settings.Hosts) != len(want) {
		t.Fatalf("Hosts is %+v, want %+v", settings.Hosts, want)
	}
	for i := range want {
		if settings.Hosts[i] != want[i] {
			t.Errorf("Hosts[%d] is %+v, want %+v", i, settings.Hosts[i], want[i])
		}
	}
}

func TestWithoutLMStudioHostsThereAreNone(t *testing.T) {
	t.Parallel()

	settings, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":   "w1:p1",
		"HERDR_POLYGLOT_ENDPOINT": "http://localhost:1234/v1/chat/completions",
	}))
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}
	if len(settings.Hosts) != 0 {
		t.Errorf("Hosts is %+v, want none", settings.Hosts)
	}
}

func TestLMStudioHostsThatCannotBeUsedAreRefused(t *testing.T) {
	t.Parallel()

	for _, written := range []string{
		"laptop",
		"=http://localhost:1234",
		"laptop=",
		"laptop=localhost:1234",
		"laptop=ftp://localhost:1234",
		"laptop=http://localhost:1234,laptop=http://localhost:4321",
	} {
		_, err := config.Load(envFrom(map[string]string{
			"HERDR_POLYGLOT_TARGET":         "w1:p1",
			"HERDR_POLYGLOT_LMSTUDIO_HOSTS": written,
		}))
		if err == nil || !strings.Contains(err.Error(), "HERDR_POLYGLOT_LMSTUDIO_HOSTS") {
			t.Errorf("%s: Load returned %v, want it refused by name", written, err)
		}
	}
}

func TestLMStudioHostsAndAnEndpointTogetherAreRefused(t *testing.T) {
	t.Parallel()

	_, err := config.Load(envFrom(map[string]string{
		"HERDR_POLYGLOT_TARGET":         "w1:p1",
		"HERDR_POLYGLOT_ENDPOINT":       "http://localhost:1234/v1/chat/completions",
		"HERDR_POLYGLOT_LMSTUDIO_HOSTS": "laptop=http://localhost:1234",
	}))
	if err == nil || !strings.Contains(err.Error(), "HERDR_POLYGLOT_ENDPOINT") {
		t.Errorf("Load returned %v, want the endpoint named as the one to move", err)
	}
}
