package store

import (
	"context"
	"testing"
	"time"

	"fieldwork/internal/arxiv"

	"github.com/alicebob/miniredis/v2"
)

func TestStatusAndListByStatus(t *testing.T) {
	t.Parallel()

	mini := miniredis.RunT(t)
	s, err := NewRedisStore("redis://" + mini.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore error: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.SetStatus(ctx, "2401.00001", "downloaded"); err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}
	if err := s.SetStatus(ctx, "2401.00002", "failed"); err != nil {
		t.Fatalf("SetStatus error: %v", err)
	}

	status, err := s.GetStatus(ctx, "2401.00001")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if status != "downloaded" {
		t.Fatalf("unexpected status: %s", status)
	}

	ids, err := s.ListByStatus(ctx, "downloaded")
	if err != nil {
		t.Fatalf("ListByStatus error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "2401.00001" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}

func TestSetMetaAndGetMeta(t *testing.T) {
	t.Parallel()

	mini := miniredis.RunT(t)
	s, err := NewRedisStore("redis://" + mini.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore error: %v", err)
	}
	defer s.Close()

	submittedAt := time.Date(2024, time.January, 15, 0, 0, 0, 0, time.UTC)
	paper := arxiv.Paper{
		ID:          "2401.12345",
		Title:       "Example",
		Authors:     []string{"Ada", "Grace"},
		Categories:  []string{"quant-ph", "hep-th"},
		SubmittedAt: submittedAt,
		PDFURL:      "https://arxiv.org/pdf/2401.12345.pdf",
	}

	ctx := context.Background()
	if err := s.SetMeta(ctx, paper.ID, paper); err != nil {
		t.Fatalf("SetMeta error: %v", err)
	}

	got, err := s.GetMeta(ctx, paper.ID)
	if err != nil {
		t.Fatalf("GetMeta error: %v", err)
	}
	if got.ID != paper.ID {
		t.Fatalf("unexpected ID: %s", got.ID)
	}
	if got.Title != paper.Title {
		t.Fatalf("unexpected title: %s", got.Title)
	}
	if len(got.Authors) != 2 || got.Authors[0] != "Ada" || got.Authors[1] != "Grace" {
		t.Fatalf("unexpected authors: %#v", got.Authors)
	}
	if len(got.Categories) != 2 || got.Categories[0] != "quant-ph" || got.Categories[1] != "hep-th" {
		t.Fatalf("unexpected categories: %#v", got.Categories)
	}
	if !got.SubmittedAt.Equal(submittedAt) {
		t.Fatalf("unexpected submitted at: %v", got.SubmittedAt)
	}
	if got.PDFURL != paper.PDFURL {
		t.Fatalf("unexpected PDFURL: %s", got.PDFURL)
	}
}

func TestDigKeys(t *testing.T) {
	t.Parallel()

	mini := miniredis.RunT(t)
	s, err := NewRedisStore("redis://" + mini.Addr())
	if err != nil {
		t.Fatalf("NewRedisStore error: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.SetDigCurrent(ctx, "2026-05-07T00:00:00Z"); err != nil {
		t.Fatalf("SetDigCurrent error: %v", err)
	}
	if err := s.SetDigStats(ctx, map[string]any{"total": 50, "downloaded": 10}); err != nil {
		t.Fatalf("SetDigStats error: %v", err)
	}

	gotCurrent, err := mini.Get("fieldwork:dig:current")
	if err != nil {
		t.Fatalf("read dig current error: %v", err)
	}
	if gotCurrent != "2026-05-07T00:00:00Z" {
		t.Fatalf("unexpected dig current: %s", gotCurrent)
	}
	if got := mini.HGet("fieldwork:dig:stats", "total"); got != "50" {
		t.Fatalf("unexpected total: %s", got)
	}
	if got := mini.HGet("fieldwork:dig:stats", "downloaded"); got != "10" {
		t.Fatalf("unexpected downloaded: %s", got)
	}
}
