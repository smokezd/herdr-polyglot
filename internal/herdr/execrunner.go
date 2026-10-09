package herdr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type ExecRunner struct{ binary string }

func NewExecRunner(binary string) ExecRunner {
	return ExecRunner{binary: binary}
}

func (e ExecRunner) Run(ctx context.Context, args ...string) error {
	_, err := e.Output(ctx, args...)
	return err
}

// Output runs the binary like Run and returns what it wrote to stdout.
func (e ExecRunner) Output(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, e.binary, args...)
	command.Stdout = &stdout
	command.Stderr = &stderr
	command.Env = withoutCredentials(os.Environ())

	if err := command.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("herdr %s: %w: %s", args[0], err, message)
		}
		return nil, fmt.Errorf("herdr %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}

// withoutCredentials keeps API keys out of the herdr process, which needs none
// of them and would pass them on to anything it starts.
func withoutCredentials(environment []string) []string {
	kept := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if strings.Contains(name, "API_KEY") {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}
