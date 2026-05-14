package queue

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryQueuePushReturnsErrQueueFullWhenBufferIsFull(t *testing.T) {
	q := NewMemoryQueue(1)

	if err := q.Push(context.Background(), Job{ID: "one", URL: "https://example.com"}); err != nil {
		t.Fatalf("first push failed: %v", err)
	}

	if err := q.Push(context.Background(), Job{ID: "two", URL: "https://example.org"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

func TestMemoryQueuePushThenPop(t *testing.T) {
	q := NewMemoryQueue(1)
	job := Job{ID: "job-1", URL: "https://example.com"}

	if err := q.Push(context.Background(), job); err != nil {
		t.Fatalf("push failed: %v", err)
	}

	got, err := q.Pop(context.Background())
	if err != nil {
		t.Fatalf("pop failed: %v", err)
	}
	if got != job {
		t.Fatalf("got %+v, want %+v", got, job)
	}
}
