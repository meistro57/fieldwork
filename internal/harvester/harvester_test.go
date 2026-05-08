package harvester

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fieldwork/internal/arxiv"
	"fieldwork/internal/store"

	"github.com/alicebob/miniredis/v2"
)

func TestHarvestDownloadsAndSetsRedisKeys(t *testing.T) {
	t.Parallel()

	mini := miniredis.RunT(t)
	redisStore, err := store.NewRedisStore("redis://" + mini.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore error: %v", err)
	}
	defer redisStore.Close()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api":
			w.Header().Set("Content-Type", "application/atom+xml")
			_, _ = fmt.Fprintf(w, `<feed>
<entry>
  <id>http://arxiv.org/abs/2401.12345v1</id>
  <title>Test Paper</title>
  <summary>Test abstract</summary>
  <published>2024-01-15T00:00:00Z</published>
  <author><name>Ada Lovelace</name></author>
  <category term="quant-ph" />
  <link title="pdf" href="%s/pdf/2401.12345v1.pdf" type="application/pdf" />
</entry>
</feed>`, serverURL)
		case r.URL.Path == "/pdf/2401.12345v1.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4 test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client := arxiv.NewClient([]string{"quant-ph"}, server.Client())
	client.BaseURL = server.URL + "/api"

	tempDir := t.TempDir()
	h := New(client, redisStore, tempDir, time.Millisecond)
	h.HTTPClient = server.Client()

	result, err := h.Harvest(context.Background(), nil, 1)
	if err != nil {
		t.Fatalf("Harvest error: %v", err)
	}

	if result.Total != 1 {
		t.Fatalf("unexpected total: %d", result.Total)
	}
	if result.Downloaded != 1 {
		t.Fatalf("unexpected downloaded: %d", result.Downloaded)
	}
	if result.Failed != 0 {
		t.Fatalf("unexpected failed: %d", result.Failed)
	}

	pdfPath := filepath.Join(tempDir, "2401.12345v1.pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		t.Fatalf("expected downloaded PDF at %s: %v", pdfPath, err)
	}

	status, err := redisStore.GetStatus(context.Background(), "2401.12345v1")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if status != "downloaded" {
		t.Fatalf("unexpected status: %s", status)
	}

	meta, err := redisStore.GetMeta(context.Background(), "2401.12345v1")
	if err != nil {
		t.Fatalf("GetMeta error: %v", err)
	}
	if meta.Title != "Test Paper" {
		t.Fatalf("unexpected title: %s", meta.Title)
	}
}
