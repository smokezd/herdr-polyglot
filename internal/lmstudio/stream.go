package lmstudio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wazum/herdr-polyglot/internal/translation"
)

// errSilent is a server that stopped sending in the middle of an answer.
var errSilent = errors.New(service + " stopped answering — is the model still loading?")

// TranslateStreaming asks for the answer as it is written and reports it as
// it comes: the model's thinking first, if it thinks out loud, then the
// translation. A model may say what it thinks in a field of its own or in
// <think> tags ahead of its answer; both read the same here.
//
// The time allowed is for silence, not for the whole answer: a long reply
// streams for longer than any single request should take to begin.
func (c *Client) TranslateStreaming(
	ctx context.Context, draft, preceding string, progress func(translation.Progress),
) (string, error) {
	asked := c.request(draft, preceding)
	asked["stream"] = true
	body, err := json.Marshal(asked)
	if err != nil {
		return "", fmt.Errorf("preparing the translation request: %w", err)
	}

	ctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	quiet := c.httpClient.Timeout
	if quiet <= 0 {
		quiet = defaultTimeout
	}
	silence := time.AfterFunc(quiet, func() { stop(errSilent) })
	defer silence.Stop()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("preparing the translation request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	streaming := &http.Client{Transport: c.httpClient.Transport, CheckRedirect: c.httpClient.CheckRedirect}
	response, err := streaming.Do(request)
	if err != nil {
		return "", streamTrouble(ctx, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s refused the draft: %s%s",
			service, response.Status, explanation(response.Body))
	}

	var reasoning, content strings.Builder
	events := bufio.NewScanner(response.Body)
	events.Buffer(make([]byte, 0, 64<<10), maxResponseBytes)
	for events.Scan() {
		data, isData := strings.CutPrefix(events.Text(), "data:")
		if !isData {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		silence.Reset(quiet)

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					Reasoning        string `json:"reasoning"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		reasoning.WriteString(delta.ReasoningContent + delta.Reasoning)
		content.WriteString(delta.Content)
		progress(soFar(reasoning.String(), content.String()))
	}
	if err := events.Err(); err != nil {
		return "", streamTrouble(ctx, fmt.Errorf("reading the translation: %w", err))
	}
	if err := context.Cause(ctx); err != nil {
		return "", streamTrouble(ctx, err)
	}

	translated := answerIn(content.String())
	if translated == "" {
		return "", errors.New(service + " answered without a translation")
	}
	return translated, nil
}

func streamTrouble(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); errors.Is(cause, errSilent) {
		return errSilent
	}
	return trouble(ctx, err)
}

// soFar takes an answer apart while it is still arriving. A tag only partly
// there is held back rather than shown, since the next piece may complete it.
func soFar(reasoning, content string) translation.Progress {
	thinking := reasoning
	text := strings.TrimLeft(content, " \t\r\n")
	thought := reasoning != "" && text != ""

	if rest, opened := strings.CutPrefix(text, "<think>"); opened {
		inner, after, closed := strings.Cut(rest, "</think>")
		thinking += inner
		text, thought = "", closed
		if closed {
			text = after
		}
	} else if beginsTag(text, "<think>") {
		text = ""
	}

	text = strings.TrimLeft(text, " \t\r\n")
	if inner, wrapped := strings.CutPrefix(text, "<text>"); wrapped {
		text = inner
	} else if beginsTag(text, "<text>") {
		text = ""
	}
	text = withoutPartialSuffix(text, "</text>")

	return translation.Progress{
		Thinking: strings.TrimSpace(thinking),
		Text:     strings.TrimSpace(text),
		Thought:  thought,
	}
}

// beginsTag says the text so far may be the start of the tag, so the next
// piece decides whether it is one.
func beginsTag(text, tag string) bool {
	return text != "" && len(text) < len(tag) && tag[:len(text)] == text
}

// withoutPartialSuffix drops the start of a closing tag from the end of text,
// or the whole tag once it is there.
func withoutPartialSuffix(text, tag string) string {
	for cut := len(tag); cut > 0; cut-- {
		if strings.HasSuffix(text, tag[:cut]) {
			return text[:len(text)-cut]
		}
	}
	return text
}
