package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fieldwork/internal/arxiv"

	redis "github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(redisURL string) (*RedisStore, error) {
	options, err := redis.ParseURL(strings.TrimSpace(redisURL))
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return &RedisStore{client: client}, nil
}

func (s *RedisStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *RedisStore) SetStatus(ctx context.Context, id, status string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id cannot be empty")
	}
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("status cannot be empty")
	}
	return s.client.Set(ctx, statusKey(id), strings.TrimSpace(status), 0).Err()
}

func (s *RedisStore) GetStatus(ctx context.Context, id string) (string, error) {
	value, err := s.client.Get(ctx, statusKey(id)).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return value, nil
}

func (s *RedisStore) ListByStatus(ctx context.Context, status string) ([]string, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return nil, fmt.Errorf("status cannot be empty")
	}

	ids := make([]string, 0)
	var cursor uint64
	for {
		keys, nextCursor, err := s.client.Scan(ctx, cursor, "fieldwork:paper:*:status", 200).Result()
		if err != nil {
			return nil, err
		}
		cursor = nextCursor

		for _, key := range keys {
			value, err := s.client.Get(ctx, key).Result()
			if err != nil {
				if err == redis.Nil {
					continue
				}
				return nil, err
			}
			if value != status {
				continue
			}

			id, ok := idFromStatusKey(key)
			if ok {
				ids = append(ids, id)
			}
		}

		if cursor == 0 {
			break
		}
	}

	return ids, nil
}

func (s *RedisStore) SetMeta(ctx context.Context, id string, paper arxiv.Paper) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id cannot be empty")
	}

	values := map[string]any{
		"title":        paper.Title,
		"authors":      strings.Join(paper.Authors, "|"),
		"categories":   strings.Join(paper.Categories, ","),
		"submitted_at": paper.SubmittedAt.UTC().Format(time.RFC3339),
		"pdf_url":      paper.PDFURL,
	}

	return s.client.HSet(ctx, metaKey(id), values).Err()
}

func (s *RedisStore) GetMeta(ctx context.Context, id string) (arxiv.Paper, error) {
	values, err := s.client.HGetAll(ctx, metaKey(id)).Result()
	if err != nil {
		return arxiv.Paper{}, err
	}
	if len(values) == 0 {
		return arxiv.Paper{}, nil
	}

	submittedAt := time.Time{}
	if raw := strings.TrimSpace(values["submitted_at"]); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return arxiv.Paper{}, fmt.Errorf("parse submitted_at for %s: %w", id, err)
		}
		submittedAt = parsed
	}

	paper := arxiv.Paper{
		ID:          id,
		Title:       strings.TrimSpace(values["title"]),
		Authors:     splitNonEmpty(values["authors"], "|"),
		Categories:  splitNonEmpty(values["categories"], ","),
		SubmittedAt: submittedAt,
		PDFURL:      strings.TrimSpace(values["pdf_url"]),
	}

	return paper, nil
}

func (s *RedisStore) SetDigCurrent(ctx context.Context, timestamp string) error {
	return s.client.Set(ctx, "fieldwork:dig:current", strings.TrimSpace(timestamp), 0).Err()
}

func (s *RedisStore) SetDigStats(ctx context.Context, stats map[string]any) error {
	if len(stats) == 0 {
		return nil
	}
	return s.client.HSet(ctx, "fieldwork:dig:stats", stats).Err()
}

func statusKey(id string) string {
	return fmt.Sprintf("fieldwork:paper:%s:status", strings.TrimSpace(id))
}

func metaKey(id string) string {
	return fmt.Sprintf("fieldwork:paper:%s:meta", strings.TrimSpace(id))
}

func idFromStatusKey(key string) (string, bool) {
	const prefix = "fieldwork:paper:"
	const suffix = ":status"
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, suffix) {
		return "", false
	}
	id := strings.TrimPrefix(key, prefix)
	id = strings.TrimSuffix(id, suffix)
	if id == "" {
		return "", false
	}
	return id, true
}

func splitNonEmpty(raw, sep string) []string {
	parts := strings.Split(raw, sep)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean != "" {
			out = append(out, clean)
		}
	}
	return out
}
