package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"mdiff/internal/config"
)

// openAIEndpoint returns an OpenAI-compatible endpoint at baseURL.
func openAIEndpoint(baseURL string) config.LLM {
	return config.LLM{Provider: config.ProviderOpenAI, BaseURL: baseURL, Model: "gpt-4o-mini", APIKey: "test-key"}
}

// writeSSE writes deltas to w as an OpenAI-compatible server-sent event
// stream: a role-only opening chunk (which carries no content and must be
// skipped), one content chunk per delta, then the [DONE] marker. A blank
// line and an SSE comment are interleaved to prove keep-alive noise is
// tolerated.
func writeSSE(t *testing.T, w http.ResponseWriter, deltas []string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\n")
	fmt.Fprint(w, ": keep-alive\n\n")
	for _, d := range deltas {
		payload, err := json.Marshal(d)
		if err != nil {
			t.Fatalf("writeSSE: marshal delta: %v", err)
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", payload)
	}
	fmt.Fprint(w, "data: [DONE]\n\n")
}

func TestExplain_Success(t *testing.T) {
	deltas := []string{"this diff renames ", "a function and ", "adds error handling"}
	want := strings.Join(deltas, "")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}
		writeSSE(t, w, deltas)
	}))
	defer ts.Close()

	got, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff: +foo -bar", nil)
	if err != nil {
		t.Fatalf("Explain returned unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("Explain() = %q, want %q", got, want)
	}
}

func TestExplain_StreamsDeltasInOrder(t *testing.T) {
	deltas := []string{"the ", "explanation ", "streams"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if !req.Stream {
			t.Error("request body had stream=false, want stream=true")
		}
		writeSSE(t, w, deltas)
	}))
	defer ts.Close()

	var got []string
	full, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff", func(chunk string) {
		got = append(got, chunk)
	})
	if err != nil {
		t.Fatalf("Explain returned unexpected error: %v", err)
	}
	if !slices.Equal(got, deltas) {
		t.Errorf("onDelta received %q, want %q", got, deltas)
	}
	if want := strings.Join(deltas, ""); full != want {
		t.Errorf("Explain() = %q, want the concatenation of the deltas %q", full, want)
	}
}

func TestExplain_DeltasArriveBeforeTheResponseEnds(t *testing.T) {
	// The whole point of streaming is that prose reaches the UI while the
	// endpoint is still generating, so the handler holds the stream open
	// until the first delta has already been delivered to onDelta.
	first := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer is not an http.Flusher, cannot stream")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		flusher.Flush()

		select {
		case <-first:
		case <-time.After(5 * time.Second):
			t.Error("onDelta was not called until the response ended: the reply is not streamed")
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" second\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()

	var once sync.Once
	got, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff", func(string) {
		once.Do(func() { close(first) })
	})
	if err != nil {
		t.Fatalf("Explain returned unexpected error: %v", err)
	}
	if want := "first second"; got != want {
		t.Errorf("Explain() = %q, want %q", got, want)
	}
}

func TestExplain_NonSuccessStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, body: `{"error":{"message":"invalid api key"}}`},
		{name: "server error", statusCode: http.StatusInternalServerError, body: `{"error":{"message":"internal error"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			_, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff", nil)
			if err == nil {
				t.Fatal("Explain() expected an error, got nil")
			}

			var reqErr *RequestError
			if !errors.As(err, &reqErr) {
				t.Fatalf("expected error to be *RequestError, got %T: %v", err, err)
			}
			if reqErr.StatusCode != tt.statusCode {
				t.Errorf("RequestError.StatusCode = %d, want %d", reqErr.StatusCode, tt.statusCode)
			}
			if !strings.Contains(reqErr.Body, "error") {
				t.Errorf("RequestError.Body = %q, want it to contain the response body", reqErr.Body)
			}
		})
	}
}

func TestExplain_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {not valid json\n\n")
	}))
	defer ts.Close()

	_, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff", nil)
	if err == nil {
		t.Fatal("Explain() expected an error for malformed JSON, got nil")
	}

	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		t.Fatalf("expected a JSON decoding error, not a RequestError: %v", err)
	}
}

func TestExplain_EmptyStream(t *testing.T) {
	// A well-formed stream that never carries a content delta (only the
	// role-only opening chunk) must not pass for a successful but empty
	// explanation: the caller would render a blank pane with no clue why.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeSSE(t, w, nil)
	}))
	defer ts.Close()

	got, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff", nil)
	if err == nil {
		t.Fatal("Explain() expected an error for a stream carrying no content, got nil")
	}
	if got != "" {
		t.Errorf("Explain() = %q, want an empty string alongside the error", got)
	}
}

func TestExplain_ConnectionFailure(t *testing.T) {
	// Start and immediately close the server so the port is unreachable,
	// simulating a network/connection failure without hitting any real
	// external endpoint.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := ts.URL
	ts.Close()

	_, err := Explain(context.Background(), openAIEndpoint(unreachableURL), "explain this diff", nil)
	if err == nil {
		t.Fatal("Explain() expected a connection error, got nil")
	}

	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		t.Fatalf("expected a connection error, not a RequestError: %v", err)
	}
}

// netTimeout reports whether err is a network-layer timeout, i.e. the
// http.Client's own Timeout fired rather than the response completing.
type netTimeout interface {
	Timeout() bool
}

func TestExplain_ClientTimeout(t *testing.T) {
	// A handler that blocks well past httpClient's timeout, so Explain must
	// give up on its own rather than hang forever waiting for a response.
	block := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer ts.Close()
	defer close(block)

	orig := httpClient.Timeout
	httpClient.Timeout = 50 * time.Millisecond
	defer func() { httpClient.Timeout = orig }()

	_, err := Explain(context.Background(), openAIEndpoint(ts.URL), "explain this diff", nil)
	if err == nil {
		t.Fatal("Explain() expected a timeout error, got nil")
	}

	var timeoutErr netTimeout
	if !errors.As(err, &timeoutErr) || !timeoutErr.Timeout() {
		t.Fatalf("Explain() error = %v, want a network timeout error", err)
	}
}

func TestExplain_ContextCancellation(t *testing.T) {
	// A handler that blocks until the request context is canceled, so
	// Explain must return promptly once ctx is canceled rather than waiting
	// for httpClient's own (much longer) timeout.
	block := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
	}))
	defer ts.Close()
	defer close(block)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := Explain(ctx, openAIEndpoint(ts.URL), "explain this diff", nil)
	if err == nil {
		t.Fatal("Explain() expected a context deadline error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Explain() error = %v, want it to wrap context.DeadlineExceeded", err)
	}
}

// captureHeaders returns a server that replies with a one-delta stream and
// records the request path and headers of the last request.
func captureHeaders(t *testing.T) (ts *httptest.Server, last func() (path string, header http.Header)) {
	t.Helper()
	var (
		mu     sync.Mutex
		path   string
		header http.Header
	)
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		path, header = r.URL.Path, r.Header.Clone()
		mu.Unlock()
		writeSSE(t, w, []string{"ok"})
	}))
	t.Cleanup(ts.Close)
	return ts, func() (string, http.Header) {
		mu.Lock()
		defer mu.Unlock()
		return path, header
	}
}

func TestExplain_AzureEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		baseURL func(origin string) string
	}{
		{name: "plain resource URL", baseURL: func(origin string) string { return origin }},
		{name: "full target URI", baseURL: func(origin string) string {
			return origin + "/openai/deployments/my-deploy/chat/completions?api-version=2024-02-01"
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, last := captureHeaders(t)
			ep := config.LLM{Provider: config.ProviderAzure, BaseURL: tt.baseURL(ts.URL), Model: "my-deploy", APIKey: "azure-key"}

			if _, err := Explain(context.Background(), ep, "explain this diff", nil); err != nil {
				t.Fatalf("Explain returned unexpected error: %v", err)
			}
			path, _ := last()
			if want := "/openai/v1/chat/completions"; path != want {
				t.Errorf("request path = %q, want %q", path, want)
			}
		})
	}
}

func TestExplain_AzureInvalidBaseURL(t *testing.T) {
	for _, baseURL := range []string{"my-resource.openai.azure.com", "ftp://example.com", "https://", "http://[::1"} {
		t.Run(baseURL, func(t *testing.T) {
			ep := config.LLM{Provider: config.ProviderAzure, BaseURL: baseURL, Model: "my-deploy", APIKey: "azure-key"}

			_, err := Explain(context.Background(), ep, "explain this diff", nil)
			if err == nil {
				t.Fatal("Explain() expected an error for an invalid Azure base URL, got nil")
			}
			if !strings.Contains(err.Error(), baseURL) {
				t.Errorf("Explain() error = %q, want it to name the base URL %q", err, baseURL)
			}
		})
	}
}

func TestExplain_AzureSendsAPIKeyHeaderOnly(t *testing.T) {
	ts, last := captureHeaders(t)
	ep := config.LLM{Provider: config.ProviderAzure, BaseURL: ts.URL, Model: "my-deploy", APIKey: "azure-key"}

	if _, err := Explain(context.Background(), ep, "explain this diff", nil); err != nil {
		t.Fatalf("Explain returned unexpected error: %v", err)
	}
	_, header := last()
	if got := header.Get("api-key"); got != "azure-key" {
		t.Errorf("api-key header = %q, want %q", got, "azure-key")
	}
	if got := header.Get("Authorization"); got != "" {
		t.Errorf("Authorization header = %q, want none", got)
	}
}

func TestExplain_AzureAPIKeyRedirect(t *testing.T) {
	tests := []struct {
		name       string
		crossHost  bool
		wantAPIKey string
	}{
		{name: "other host", crossHost: true, wantAPIKey: ""},
		{name: "same host", crossHost: false, wantAPIKey: "azure-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, last := captureHeaders(t)
			var origin *httptest.Server
			origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/final" {
					target.Config.Handler.ServeHTTP(w, r)
					return
				}
				location := origin.URL + "/final"
				if tt.crossHost {
					location = target.URL + "/final"
				}
				http.Redirect(w, r, location, http.StatusTemporaryRedirect)
			}))
			t.Cleanup(origin.Close)
			ep := config.LLM{Provider: config.ProviderAzure, BaseURL: origin.URL, Model: "my-deploy", APIKey: "azure-key"}

			if _, err := Explain(context.Background(), ep, "explain this diff", nil); err != nil {
				t.Fatalf("Explain returned unexpected error: %v", err)
			}
			_, header := last()
			if got := header.Get("api-key"); got != tt.wantAPIKey {
				t.Errorf("api-key header at redirect target = %q, want %q", got, tt.wantAPIKey)
			}
		})
	}
}

func TestExplain_OpenAIAuthHeader(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		wantHeader string
	}{
		{name: "key set", apiKey: "test-key", wantHeader: "Bearer test-key"},
		{name: "empty key", apiKey: "", wantHeader: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts, last := captureHeaders(t)
			ep := openAIEndpoint(ts.URL)
			ep.APIKey = tt.apiKey

			if _, err := Explain(context.Background(), ep, "explain this diff", nil); err != nil {
				t.Fatalf("Explain returned unexpected error: %v", err)
			}
			_, header := last()
			if got := header.Get("Authorization"); got != tt.wantHeader {
				t.Errorf("Authorization header = %q, want %q", got, tt.wantHeader)
			}
			if _, ok := header["Api-Key"]; ok {
				t.Error("OpenAI request carried an api-key header, want none")
			}
		})
	}
}
