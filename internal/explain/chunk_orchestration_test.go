package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// bigFileContent returns file content large enough that, combined with two
// siblings of the same size, planChunks is forced to split it into more
// than one group, prefixed with marker so a fake server can tell which
// file's diff a given request carries.
func bigFileContent(marker string) string {
	return marker + strings.Repeat("x", 15000) + "\n"
}

// chunkStub starts an httptest.Server whose response to each request is
// decided by respond, given that request's outgoing prompt (the single
// user message content), so a test can drive distinguishable per-chunk
// responses, artificial latency, and injected failures.
func chunkStub(t *testing.T, respond func(prompt string) (status int, content string, delay time.Duration)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("chunkStub: decode request body: %v", err)
		}
		var prompt string
		if len(req.Messages) > 0 {
			prompt = req.Messages[0].Content
		}

		status, content, delay := respond(prompt)
		if delay > 0 {
			time.Sleep(delay)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			fmt.Fprint(w, content)
			return
		}

		encoded, err := json.Marshal(content)
		if err != nil {
			t.Fatalf("chunkStub: encode content: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%s}}]}\n\n", encoded)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

// requestKind classifies which phase of a chunked Explain call a fake
// server received a prompt for, by the always-present marker phrases in
// chunkPrompt, synthesisPrompt, and allChangesPrompt.
func requestKind(prompt string) string {
	switch {
	case strings.Contains(prompt, "split out only because the full changeset is too large"):
		return "chunk"
	case strings.Contains(prompt, "factual summaries of consecutive slices"):
		return "synth"
	case strings.Contains(prompt, "combined diff covering every changed file"):
		return "all"
	default:
		return "unknown"
	}
}

// recordedRequest captures one request a fake server received, tagged by
// requestKind, for assertions made after the Explain call returns.
type recordedRequest struct {
	kind   string
	prompt string
}

// requestRecorder collects recordedRequests from concurrent map-phase
// calls under a mutex.
type requestRecorder struct {
	mu       sync.Mutex
	requests []recordedRequest
}

func (r *requestRecorder) record(prompt string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, recordedRequest{kind: requestKind(prompt), prompt: prompt})
}

func (r *requestRecorder) byKind(kind string) []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []recordedRequest
	for _, req := range r.requests {
		if req.kind == kind {
			out = append(out, req)
		}
	}
	return out
}

// writeChunkedChangeset writes three files into dir, sized and marked so
// that planChunks splits them into two groups: {a.txt, b.txt} and {c.txt}.
func writeChunkedChangeset(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "a.txt", bigFileContent("MARKERA"))
	writeFile(t, dir, "b.txt", bigFileContent("MARKERB"))
	writeFile(t, dir, "c.txt", bigFileContent("MARKERC"))
}

func TestExplain_Chunked_MapPhaseNeverEmbedsProjectBrief(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeChunkedChangeset(t, dir)

	rec := &requestRecorder{}
	ts := chunkStub(t, func(prompt string) (int, string, time.Duration) {
		rec.record(prompt)
		return http.StatusOK, "a summary", 0
	})
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt", "b.txt", "c.txt"}, "this project is a CLI tool written in Go", nil); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	chunkReqs := rec.byKind("chunk")
	if len(chunkReqs) != 2 {
		t.Fatalf("got %d chunk requests, want 2 (grouping should split 3 large files into 2 groups)", len(chunkReqs))
	}
	for _, req := range chunkReqs {
		if strings.Contains(req.prompt, "--- project brief ---") {
			t.Errorf("map-phase chunk prompt embeds the project brief, want it never to: %q", req.prompt)
		}
	}
}

func TestExplain_Chunked_SynthesisPhaseEmbedsProjectBrief(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeChunkedChangeset(t, dir)

	rec := &requestRecorder{}
	ts := chunkStub(t, func(prompt string) (int, string, time.Duration) {
		rec.record(prompt)
		return http.StatusOK, "a summary", 0
	})
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt", "b.txt", "c.txt"}, "this project is a CLI tool written in Go", nil); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	synthReqs := rec.byKind("synth")
	if len(synthReqs) != 1 {
		t.Fatalf("got %d synthesis requests, want exactly 1", len(synthReqs))
	}
	if !strings.Contains(synthReqs[0].prompt, "--- project brief ---") {
		t.Errorf("synthesis prompt = %q, want it to embed the project brief", synthReqs[0].prompt)
	}
}

func TestExplain_Chunked_PreservesOriginalOrderDespiteArrivalReordering(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeChunkedChangeset(t, dir)

	ts := chunkStub(t, func(prompt string) (int, string, time.Duration) {
		switch {
		case strings.Contains(prompt, "MARKERC"):
			// The lone-file second group answers fast.
			return http.StatusOK, "GROUP2-SUMMARY", 0
		case strings.Contains(prompt, "MARKERA"):
			// The two-file first group is deliberately slower, so it
			// arrives after the second group's response.
			return http.StatusOK, "GROUP1-SUMMARY", 50 * time.Millisecond
		default:
			return http.StatusOK, "final explanation", 0
		}
	})
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	got, err := s.Explain(context.Background(), repo, "", []string{"a.txt", "b.txt", "c.txt"}, "", nil)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if got != "final explanation" {
		t.Errorf("Explain() = %q, want %q", got, "final explanation")
	}
}

func TestExplain_Chunked_SynthesisListsSummariesInOriginalOrder(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeChunkedChangeset(t, dir)

	var mu sync.Mutex
	var synthPrompt string
	ts := chunkStub(t, func(prompt string) (int, string, time.Duration) {
		switch {
		case strings.Contains(prompt, "MARKERC"):
			return http.StatusOK, "GROUP2-SUMMARY", 0
		case strings.Contains(prompt, "MARKERA"):
			// Slower response for the group that must appear first in the
			// synthesis prompt, so a naive "first to arrive" ordering
			// would fail this test.
			return http.StatusOK, "GROUP1-SUMMARY", 50 * time.Millisecond
		default:
			mu.Lock()
			synthPrompt = prompt
			mu.Unlock()
			return http.StatusOK, "final explanation", 0
		}
	})
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	if _, err := s.Explain(context.Background(), repo, "", []string{"a.txt", "b.txt", "c.txt"}, "", nil); err != nil {
		t.Fatalf("Explain: %v", err)
	}

	mu.Lock()
	prompt := synthPrompt
	mu.Unlock()

	idxGroup1 := strings.Index(prompt, "GROUP1-SUMMARY")
	idxGroup2 := strings.Index(prompt, "GROUP2-SUMMARY")
	if idxGroup1 == -1 || idxGroup2 == -1 {
		t.Fatalf("synthesis prompt missing one or both summaries: %q", prompt)
	}
	if idxGroup1 > idxGroup2 {
		t.Errorf("synthesis prompt lists group 2 before group 1, despite group 1 being first in input order: %q", prompt)
	}
	if !strings.Contains(prompt, "summary 1:\nGROUP1-SUMMARY") {
		t.Errorf("synthesis prompt = %q, want it to label the first group's summary as \"summary 1\"", prompt)
	}
	if !strings.Contains(prompt, "summary 2:\nGROUP2-SUMMARY") {
		t.Errorf("synthesis prompt = %q, want it to label the second group's summary as \"summary 2\"", prompt)
	}
}

func TestExplain_Chunked_OneChunkErrorFailsWholeCallWithoutPartialSynthesis(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeChunkedChangeset(t, dir)

	rec := &requestRecorder{}
	ts := chunkStub(t, func(prompt string) (int, string, time.Duration) {
		rec.record(prompt)
		if strings.Contains(prompt, "MARKERC") {
			return http.StatusInternalServerError, "synthetic failure", 0
		}
		return http.StatusOK, "GROUP1-SUMMARY", 0
	})
	defer ts.Close()
	configureForTest(t, ts.URL)

	s := NewService()
	_, err := s.Explain(context.Background(), repo, "", []string{"a.txt", "b.txt", "c.txt"}, "", nil)
	if err == nil {
		t.Fatal("Explain: expected an error when one chunk call fails, got nil")
	}

	if synthReqs := rec.byKind("synth"); len(synthReqs) != 0 {
		t.Errorf("got %d synthesis requests after a chunk failure, want 0 (no partial synthesis)", len(synthReqs))
	}
}

func TestExplain_SmallChangeset_UnaffectedByChunking(t *testing.T) {
	dir, repo := initRepo(t)
	runGitIn(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	writeFile(t, dir, "a.txt", "a content\n")
	writeFile(t, dir, "b.txt", "b content\n")

	var requestCount int
	var gotPrompt string
	ts := llmStub(t, func(prompt string) {
		requestCount++
		gotPrompt = prompt
	})
	defer ts.Close()
	configureForTest(t, ts.URL)

	combined, err := combineDiffs(context.Background(), repo, "", []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatalf("combineDiffs: %v", err)
	}

	s := NewService()
	got, err := s.Explain(context.Background(), repo, "", []string{"a.txt", "b.txt"}, "a project brief", nil)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	if got != "an explanation" {
		t.Errorf("Explain() = %q, want %q", got, "an explanation")
	}
	if requestCount != 1 {
		t.Fatalf("LLM backend received %d requests, want 1: a small changeset must not be chunked", requestCount)
	}
	if want := allChangesPrompt(combined, "a project brief"); gotPrompt != want {
		t.Errorf("outgoing prompt = %q, want %q (byte-identical to the pre-chunking \"all\" prompt)", gotPrompt, want)
	}

	wantKey := cacheKey(combined, "test-model", "all", "a project brief")
	if cached, ok := s.cached(wantKey); !ok || cached != "an explanation" {
		t.Errorf("s.cached(%q) = (%q, %v), want (\"an explanation\", true): cache key shape must be unchanged for the unchunked case", wantKey, cached, ok)
	}
}
