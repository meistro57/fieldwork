package arxiv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchLatestBuildsExpectedQueryAndParsesFeed(t *testing.T) {
	t.Parallel()

	var (
		gotSearchQuery string
		gotSortBy      string
		gotSortOrder   string
		gotStart       string
		gotMaxResults  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSearchQuery = r.URL.Query().Get("search_query")
		gotSortBy = r.URL.Query().Get("sortBy")
		gotSortOrder = r.URL.Query().Get("sortOrder")
		gotStart = r.URL.Query().Get("start")
		gotMaxResults = r.URL.Query().Get("max_results")

		w.Header().Set("Content-Type", "application/atom+xml")
		_, _ = fmt.Fprint(w, `<feed>
<entry>
  <id>http://arxiv.org/abs/2401.12345v1</id>
  <title>  Example   Title </title>
  <summary> first
line </summary>
  <published>2024-01-15T00:00:00Z</published>
  <author><name>Ada Lovelace</name></author>
  <author><name>Grace Hopper</name></author>
  <category term="quant-ph" />
  <category term="hep-th" />
  <link title="pdf" href="https://arxiv.org/pdf/2401.12345v1.pdf" type="application/pdf" />
</entry>
</feed>`)
	}))
	defer server.Close()

	client := NewClient([]string{"quant-ph", "gr-qc"}, server.Client())
	client.BaseURL = server.URL

	papers, err := client.FetchLatest(context.Background(), nil, 50)
	if err != nil {
		t.Fatalf("FetchLatest returned error: %v", err)
	}

	if gotSearchQuery != "cat:quant-ph+OR+cat:gr-qc" {
		t.Fatalf("unexpected search_query: %s", gotSearchQuery)
	}
	if gotSortBy != "submittedDate" {
		t.Fatalf("unexpected sortBy: %s", gotSortBy)
	}
	if gotSortOrder != "descending" {
		t.Fatalf("unexpected sortOrder: %s", gotSortOrder)
	}
	if gotStart != "0" {
		t.Fatalf("unexpected start: %s", gotStart)
	}
	if gotMaxResults != "50" {
		t.Fatalf("unexpected max_results: %s", gotMaxResults)
	}

	if len(papers) != 1 {
		t.Fatalf("expected 1 paper, got %d", len(papers))
	}

	paper := papers[0]
	if paper.ID != "2401.12345v1" {
		t.Fatalf("unexpected ID: %s", paper.ID)
	}
	if paper.Title != "Example Title" {
		t.Fatalf("unexpected title: %q", paper.Title)
	}
	if paper.Abstract != "first line" {
		t.Fatalf("unexpected abstract: %q", paper.Abstract)
	}
	if len(paper.Authors) != 2 || paper.Authors[0] != "Ada Lovelace" || paper.Authors[1] != "Grace Hopper" {
		t.Fatalf("unexpected authors: %#v", paper.Authors)
	}
	if len(paper.Categories) != 2 || paper.Categories[0] != "quant-ph" || paper.Categories[1] != "hep-th" {
		t.Fatalf("unexpected categories: %#v", paper.Categories)
	}
	if paper.PDFURL != "https://arxiv.org/pdf/2401.12345v1.pdf" {
		t.Fatalf("unexpected PDFURL: %s", paper.PDFURL)
	}
}

func TestFetchLatestErrorsOnNon2xx(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient([]string{"quant-ph"}, server.Client())
	client.BaseURL = server.URL

	_, err := client.FetchLatest(context.Background(), nil, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
