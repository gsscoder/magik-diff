// Package llm provides a minimal HTTP client for OpenAI-compatible
// chat-completions endpoints. It deliberately carries no LLM SDK
// dependency: requests are built and sent with net/http from the standard
// library, per the locked architecture decision in section 6 of
// memory-bank/detailed-spec/architecture.md.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// chatMessage is a single OpenAI-compatible chat message.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest is the request body for the chat-completions
// endpoint, non-streaming only.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatCompletionResponse is the subset of the OpenAI chat-completions
// response shape needed to extract the assistant's reply.
type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

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
// endpoint at baseURL and returns the assistant's reply text.
//
// baseURL is the OpenAI-compatible API root (e.g. "https://api.openai.com/v1"
// or an httptest.Server URL in tests); "/chat/completions" is appended to it.
// The call is non-streaming: it waits for the full JSON response body.
func Explain(baseURL, model, apiKey, prompt string) (string, error) {
	reqBody := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llm: failed to encode request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("llm: failed to build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("llm: request to %s failed: %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llm: failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &RequestError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("llm: failed to parse response JSON: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llm: response contained no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
