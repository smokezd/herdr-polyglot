package lmstudio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ChatEndpoint is where a server whose address is base answers chat completions.
func ChatEndpoint(base string) string {
	return strings.TrimRight(base, "/") + "/v1/chat/completions"
}

// Probe asks the server at base which models it has, which is also how a picker
// learns whether it is up at all. How long to wait is the caller's to say.
func Probe(ctx context.Context, base, apiKey string) ([]string, error) {
	if err := requireSecureEndpoint(base, apiKey != ""); err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimRight(base, "/")+"/v1/models", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("asking %s for its models: %w", base, err)
	}
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}

	// No redirects: the key must not follow one anywhere.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", base, response.Status)
	}

	var listed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes)).Decode(&listed); err != nil {
		return nil, fmt.Errorf("reading the models %s has: %w", base, err)
	}
	models := make([]string, 0, len(listed.Data))
	for _, model := range listed.Data {
		models = append(models, model.ID)
	}
	return models, nil
}
