package llm

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExplain_Success(t *testing.T) {
	want := "this diff renames a function and adds error handling"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"choices": [
				{"message": {"role": "assistant", "content": "` + want + `"}}
			]
		}`))
	}))
	defer ts.Close()

	got, err := Explain(ts.URL, "gpt-4o-mini", "test-key", "explain this diff: +foo -bar")
	if err != nil {
		t.Fatalf("Explain returned unexpected error: %v", err)
	}
	if got != want {
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

			_, err := Explain(ts.URL, "gpt-4o-mini", "test-key", "explain this diff")
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
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	}))
	defer ts.Close()

	_, err := Explain(ts.URL, "gpt-4o-mini", "test-key", "explain this diff")
	if err == nil {
		t.Fatal("Explain() expected an error for malformed JSON, got nil")
	}

	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		t.Fatalf("expected a JSON decoding error, not a RequestError: %v", err)
	}
}

func TestExplain_ConnectionFailure(t *testing.T) {
	// Start and immediately close the server so the port is unreachable,
	// simulating a network/connection failure without hitting any real
	// external endpoint.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := ts.URL
	ts.Close()

	_, err := Explain(unreachableURL, "gpt-4o-mini", "test-key", "explain this diff")
	if err == nil {
		t.Fatal("Explain() expected a connection error, got nil")
	}

	var reqErr *RequestError
	if errors.As(err, &reqErr) {
		t.Fatalf("expected a connection error, not a RequestError: %v", err)
	}
}
