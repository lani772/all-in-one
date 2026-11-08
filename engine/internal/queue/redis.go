package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	streamName = "hosting:build-jobs"
	groupName  = "hosting-build-workers"
)

type Job struct {
	StreamID string
	ID       string
}

type Queue struct {
	client *redis.Client
	claimMu sync.Mutex
	claimCursor string
}

func New(addr, password string, db int) (*Queue, error) {
	if addr == "" {
		return nil, errors.New("REDIS_ADDR is required")
	}
	if db < 0 {
		return nil, errors.New("REDIS_DB cannot be negative")
	}
	return &Queue{client: redis.NewClient(&redis.Options{
		Addr: addr, Password: password, DB: db,
		DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 5 * time.Second, PoolSize: 10,
	})}, nil
}

func (q *Queue) Close() error {
	if q == nil || q.client == nil {
		return nil
	}
	return q.client.Close()
}

func (q *Queue) Ping(ctx context.Context) error {
	if q == nil || q.client == nil {
		return errors.New("Redis queue is not configured")
	}
	return q.client.Ping(ctx).Err()
}

func (q *Queue) EnsureGroup(ctx context.Context) error {
	if q == nil || q.client == nil {
		return errors.New("Redis queue is not configured")
	}
	err := q.client.XGroupCreateMkStream(ctx, streamName, groupName, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("create build stream consumer group: %w", err)
	}
	return nil
}

// Enqueue is called by the PostgreSQL outbox dispatcher. Duplicate stream
// entries are safe: the worker claims a database job only while it is queued.
func (q *Queue) Enqueue(ctx context.Context, jobID string) error {
	if jobID == "" {
		return errors.New("build job ID is required")
	}
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName, MaxLen: 100000, Approx: true,
		Values: map[string]any{"job_id": jobID},
	}).Err()
}

func (q *Queue) Read(ctx context.Context, consumer string, block time.Duration, count int64) ([]Job, error) {
	if count < 1 {
		count = 1
	}
	streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: groupName, Consumer: consumer,
		Streams: []string{streamName, ">"}, Count: count, Block: block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return collect(streams), nil
}

func (q *Queue) ClaimStale(ctx context.Context, consumer string, minIdle time.Duration, count int64) ([]Job, error) {
	if count < 1 {
		count = 1
	}
	q.claimMu.Lock()
	defer q.claimMu.Unlock()
	start := q.claimCursor
	if start == "" { start = "0-0" }
	messages, next, err := q.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream: streamName, Group: groupName, Consumer: consumer,
		MinIdle: minIdle, Start: start, Count: count,
	}).Result()
	if err != nil {
		return nil, err
	}
	if next == "0-0" { q.claimCursor = "" } else { q.claimCursor = next }
	return collect([]redis.XStream{{Stream: streamName, Messages: messages}}), nil
}

func (q *Queue) Ack(ctx context.Context, streamID string) error {
	if streamID == "" {
		return errors.New("stream message ID is required")
	}
	return q.client.XAck(ctx, streamName, groupName, streamID).Err()
}

func collect(streams []redis.XStream) []Job {
	var jobs []Job
	for _, stream := range streams {
		for _, message := range stream.Messages {
			value, ok := message.Values["job_id"]
			if !ok {
				continue
			}
			var id string
			switch v := value.(type) {
			case string:
				id = v
			case []byte:
				id = string(v)
			case int64:
				id = strconv.FormatInt(v, 10)
			}
			if id != "" {
				jobs = append(jobs, Job{StreamID: message.ID, ID: id})
			}
		}
	}
	return jobs
}
