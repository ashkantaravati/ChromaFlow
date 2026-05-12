package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"chromaflow/internal/redisx"
)

type RedisQueue struct {
	client        *redisx.Client
	queueKey      string
	processingKey string
	capacity      int
}

func NewRedisQueue(ctx context.Context, rawURL, prefix string, capacity int) (*RedisQueue, error) {
	client, err := redisx.New(rawURL)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		prefix = "chromaflow"
	}
	q := &RedisQueue{client: client, queueKey: prefix + ":queue", processingKey: prefix + ":processing", capacity: capacity}
	if err := q.requeueProcessing(ctx); err != nil {
		return nil, err
	}
	return q, nil
}

func (q *RedisQueue) Push(ctx context.Context, job Job) error {
	if q.capacity > 0 {
		depth, err := q.Len(ctx)
		if err != nil {
			return err
		}
		if depth >= q.capacity {
			return ErrQueueFull
		}
	}
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	_, err = q.client.Do(ctx, "LPUSH", q.queueKey, string(data))
	return err
}

func (q *RedisQueue) Pop(ctx context.Context) (Job, error) {
	for {
		select {
		case <-ctx.Done():
			return Job{}, ctx.Err()
		default:
		}
		v, err := q.client.Do(ctx, "BRPOPLPUSH", q.queueKey, q.processingKey, "5")
		if err != nil {
			return Job{}, err
		}
		if v == nil {
			continue
		}
		payload := redisx.String(v)
		var job Job
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			_, _ = q.client.Do(ctx, "LREM", q.processingKey, "1", payload)
			return Job{}, err
		}
		job.QueueMessageID = payload
		return job, nil
	}
}

func (q *RedisQueue) Ack(ctx context.Context, job Job) error {
	payload := job.QueueMessageID
	if payload == "" {
		data, err := json.Marshal(job)
		if err != nil {
			return err
		}
		payload = string(data)
	}
	_, err := q.client.Do(ctx, "LREM", q.processingKey, "1", payload)
	return err
}

func (q *RedisQueue) Len(ctx context.Context) (int, error) {
	queued, err := q.client.Do(ctx, "LLEN", q.queueKey)
	if err != nil {
		return 0, err
	}
	processing, err := q.client.Do(ctx, "LLEN", q.processingKey)
	if err != nil {
		return 0, err
	}
	return redisx.Int(queued) + redisx.Int(processing), nil
}

func (q *RedisQueue) Cap() int { return q.capacity }

func (q *RedisQueue) requeueProcessing(ctx context.Context) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		v, err := q.client.Do(ctx, "RPOPLPUSH", q.processingKey, q.queueKey)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			return fmt.Errorf("recover processing queue: %w", err)
		}
		if v == nil {
			return nil
		}
	}
	return fmt.Errorf("recover processing queue timed out after 5s")
}
