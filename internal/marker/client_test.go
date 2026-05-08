package marker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if r.URL.Path != "/parse" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("unexpected content type: %s", ct)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"markdown":"# Intro","title":"Paper","sections":["Intro"],"charts":[],"latex_blocks":["x^2"]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	client.Backoff = 10 * time.Millisecond

	result, err := client.Parse(context.Background(), "/tmp/paper.pdf")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if result.Title != "Paper" {
		t.Fatalf("unexpected title: %s", result.Title)
	}
	if len(result.Sections) != 1 || result.Sections[0] != "Intro" {
		t.Fatalf("unexpected sections: %#v", result.Sections)
	}
	if len(result.LatexBlocks) != 1 || result.LatexBlocks[0] != "x^2" {
		t.Fatalf("unexpected latex blocks: %#v", result.LatexBlocks)
	}
}

func TestParseRetriesThenSucceeds(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := attempts.Add(1)
		if current == 1 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"markdown":"ok","title":"ok","sections":[],"charts":[],"latex_blocks":[]}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	client.Retries = 2
	client.Backoff = 10 * time.Millisecond

	_, err := client.Parse(context.Background(), "/tmp/paper.pdf")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if attempts.Load() != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts.Load())
	}
}

func TestParseReturnsErrorOnNon2xx(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	client.Retries = 1

	_, err := client.Parse(context.Background(), "/tmp/paper.pdf")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("unexpected error: %v", err)
	}
}
