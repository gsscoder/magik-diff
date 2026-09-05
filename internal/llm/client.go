// Package llm provides a minimal HTTP client for OpenAI-compatible
// chat-completions endpoints. It deliberately carries no LLM SDK
// dependency: requests are built and sent with net/http from the standard
// library, per the locked architecture decision in section 6 of
// memory-bank/detailed-spec/architecture.md.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// requestTimeout bounds how long a single Explain call may take, so a hung
// or slow-to-respond endpoint cannot wedge the caller forever.
const requestTimeout = 180 * time.Second

// httpClient is the client used for all Explain requests. Timeout is set
// explicitly because http.DefaultClient has no timeout at all.
var httpClient = &http.Client{Timeout: requestTimeout}

// chatMessage is a single OpenAI-compatible chat message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest is the request body for the chat-completions
// endpoint, streaming only.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatCompletionChunk is the subset of one server-sent chat-completions
// chunk needed to extract the assistant's incremental reply text. A chunk
// may legitimately carry no content: the first one usually holds only the
// role, and some endpoints emit a trailing usage-only chunk with no
// choices at all.
type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// streamBufferSize is the largest single SSE line the scanner accepts. The
// default 64KB limit is generous for a token delta but not guaranteed, and
// overrunning it would abort an otherwise healthy stream.
const streamBufferSize = 1 << 20

// doneMarker terminates an OpenAI-compatible SSE stream.
const doneMarker = "[DONE]"

// RequestError is returned when the endpoint responds with a non-2xx
// HTTP status. It carries the status code and response body so callers
// can distinguish it from network and decoding failures.
type RequestError struct {
	StatusCode int
	Body       string
}

func (e *RequestError) Error() string {
	return fmt.Sprintf("llm: request failed with status %d: %s", e.StatusCode, e.Body)
}

// Explain sends prompt as a single user message to the chat-completions
// endpoint at baseURL and returns the assistant's complete reply text.
//
// baseURL is the OpenAI-compatible API root (e.g. "https://api.openai.com/v1"
// or an httptest.Server URL in tests); "/chat/completions" is appended to it.
//
// The call is streaming: the server-sent response is consumed chunk by
// chunk and onDelta, when non-nil, is called with each non-empty content
// delta in arrival order, so a caller can render prose as it is generated.
// The concatenation of every delta is what Explain returns, so a nil
// onDelta simply means "no incremental callback" and changes nothing else.
// onDelta runs on the calling goroutine and must not block for long.
//
// Because the whole generation is now read inside the request,
// requestTimeout is a wall-clock bound on the entire response, not just on
// the response headers. The call is canceled early if ctx is canceled.
func Explain(ctx context.Context, baseURL, model, apiKey, prompt string, onDelta func(string)) (string, error) {
	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: true,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: failed to encode request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm: request to %s failed: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("llm: failed to read response body: %w", err)
		}
		return "", &RequestError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	return readStream(resp.Body, onDelta)
}

// readStream consumes an OpenAI-compatible server-sent event stream from r,
// forwarding each non-empty content delta to onDelta (when non-nil) and
// returning the concatenation of them all. Blank lines and SSE comment
// lines (which some endpoints send as keep-alives) are skipped, and the
// stream ends at the [DONE] marker. A stream that carries no content at
// all is reported as an error rather than as an empty reply.
func readStream(r io.Reader, onDelta func(string)) (string, error) {
	var full strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), streamBufferSize)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == doneMarker {
			break
		}

		var chunk chatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return "", fmt.Errorf("llm: failed to parse response JSON: %w", err)
		}
		if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == "" {
			continue
		}

		content := chunk.Choices[0].Delta.Content
		full.WriteString(content)
		if onDelta != nil {
			onDelta(content)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("llm: failed to read response stream: %w", err)
	}
	if full.Len() == 0 {
		return "", fmt.Errorf("llm: the endpoint returned an empty response")
	}
	return full.String(), nil
}
