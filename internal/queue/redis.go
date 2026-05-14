package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"chromaflow/internal/redisx"

	"github.com/google/uuid"
)

type RedisQueue struct {
	client            *redisx.Client
	queueKey          string
	processingKey     string
	capacity          int
	visibilityTimeout time.Duration
}

func NewRedisQueue(ctx context.Context, rawURL, prefix string, capacity int, visibilityTimeout time.Duration) (*RedisQueue, error) {
	client, err := redisx.New(rawURL)
	if err != nil {
		return nil, err
	}
	if prefix == "" {
		prefix = "chromaflow"
	}
	if visibilityTimeout <= 0 {
		visibilityTimeout = 5 * time.Minute
	}
	return &RedisQueue{client: client, queueKey: prefix + ":queue", processingKey: prefix + ":processing", capacity: capacity, visibilityTimeout: visibilityTimeout}, nil
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
	if job.QueueMessageID == "" {
		job.QueueMessageID = uuid.NewString()
	}
	job.LeaseUntil = time.Time{}
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
		if err := q.requeueExpired(ctx, time.Now().UTC()); err != nil {
			return Job{}, err
		}
		v, err := q.client.Do(ctx, "BRPOP", q.queueKey, "5")
		if err != nil {
			return Job{}, err
		}
		if v == nil {
			continue
		}
		arr, _ := v.([]any)
		if len(arr) < 2 {
			continue
		}
		payload := redisx.String(arr[1])
		var job Job
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			return Job{}, err
		}
		if job.QueueMessageID == "" {
			job.QueueMessageID = uuid.NewString()
		}
		job.LeaseUntil = time.Now().UTC().Add(q.visibilityTimeout)
		leasedPayload, err := json.Marshal(job)
		if err != nil {
			return Job{}, err
		}
		leased := string(leasedPayload)
		_, err = q.client.Do(ctx, "ZADD", q.processingKey, strconv.FormatInt(job.LeaseUntil.Unix(), 10), leased)
		if err != nil {
			_, _ = q.client.Do(ctx, "LPUSH", q.queueKey, payload)
			return Job{}, err
		}
		job.QueueMessageID = leased
		return job, nil
	}
}

func (q *RedisQueue) Ack(ctx context.Context, job Job) error {
	if job.QueueMessageID == "" {
		return nil
	}
	_, err := q.client.Do(ctx, "ZREM", q.processingKey, job.QueueMessageID)
	return err
}

func (q *RedisQueue) Len(ctx context.Context) (int, error) {
	queued, err := q.client.Do(ctx, "LLEN", q.queueKey)
	if err != nil {
		return 0, err
	}
	processing, err := q.client.Do(ctx, "ZCARD", q.processingKey)
	if err != nil {
		return 0, err
	}
	return redisx.Int(queued) + redisx.Int(processing), nil
}

func (q *RedisQueue) Cap() int { return q.capacity }

func (q *RedisQueue) requeueExpired(ctx context.Context, now time.Time) error {
	v, err := q.client.Do(ctx, "ZRANGEBYSCORE", q.processingKey, "-inf", strconv.FormatInt(now.Unix(), 10), "LIMIT", "0", "25")
	if err != nil {
		return fmt.Errorf("read expired processing jobs: %w", err)
	}
	arr, _ := v.([]any)
	for _, item := range arr {
		payload := redisx.String(item)
		var job Job
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			_, _ = q.client.Do(ctx, "ZREM", q.processingKey, payload)
			continue
		}
		job.LeaseUntil = time.Time{}
		data, err := json.Marshal(job)
		if err != nil {
			return err
		}
		_, err = q.client.Do(ctx, "ZREM", q.processingKey, payload)
		if err != nil {
			return err
		}
		_, err = q.client.Do(ctx, "LPUSH", q.queueKey, string(data))
		if err != nil {
			return err
		}
	}
	return nil
}
