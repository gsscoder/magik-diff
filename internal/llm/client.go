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
	"net/url"
	"strings"
	"time"

	"mdiff/internal/config"
)

// requestTimeout bounds how long a single Explain call may take, so a hung
// or slow-to-respond endpoint cannot wedge the caller forever.
const requestTimeout = 180 * time.Second

// maxRedirects matches the net/http default redirect limit.
const maxRedirects = 10

// httpClient is the client used for all Explain requests. Timeout is set
// explicitly because http.DefaultClient has no timeout at all.
var httpClient = &http.Client{Timeout: requestTimeout, CheckRedirect: checkRedirect}

// checkRedirect applies the net/http default redirect limit and additionally
// drops the Azure "api-key" header when the redirect leaves the original host.
// net/http strips only Authorization, WWW-Authenticate and Cookie across hosts,
// so without this the key would reach the redirect target.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("llm: stopped after %d redirects", maxRedirects)
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del("api-key")
	}
	return nil
}

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

// endpointURL returns the chat-completions URL for ep.
//
// For OpenAI-compatible endpoints it is BaseURL + "/chat/completions". For
// Azure only the scheme, host and port of BaseURL are used, so both the plain
// resource URL and the full "Target URI" copied from the Azure portal (with
// its deployment path and api-version query) reach the same v1 endpoint.
func endpointURL(ep config.LLM) (string, error) {
	if ep.Provider != config.ProviderAzure {
		return ep.BaseURL + "/chat/completions", nil
	}
	u, err := url.Parse(ep.BaseURL)
	if err != nil {
		return "", fmt.Errorf("llm: invalid Azure base URL %q: %w", ep.BaseURL, err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("llm: invalid Azure base URL %q: want an absolute http(s) URL", ep.BaseURL)
	}
	return u.Scheme + "://" + u.Host + "/openai/v1/chat/completions", nil
}

// setAuth sets the provider-specific authentication header on req. An
// OpenAI-compatible endpoint may run without a key, so its header is omitted
// when the key is empty.
func setAuth(req *http.Request, ep config.LLM) {
	switch {
	case ep.Provider == config.ProviderAzure:
		req.Header.Set("api-key", ep.APIKey)
	case ep.APIKey != "":
		req.Header.Set("Authorization", "Bearer "+ep.APIKey)
	}
}

// Explain sends prompt as a single user message to the chat-completions
// endpoint described by ep and returns the assistant's complete reply text.
//
// ep.BaseURL is the OpenAI-compatible API root (e.g. "https://api.openai.com/v1"
// or an httptest.Server URL in tests); see endpointURL for how the request URL
// and authentication header are derived from ep.Provider. ep.Model is the
// deployment name for Azure.
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
func Explain(ctx context.Context, ep config.LLM, prompt string, onDelta func(string)) (string, error) {
	reqBody := chatCompletionRequest{
		Model: ep.Model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: true,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: failed to encode request: %w", err)
	}

	endpoint, err := endpointURL(ep)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	setAuth(httpReq, ep)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm: request to %s failed: %w", ep.BaseURL, err)
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
