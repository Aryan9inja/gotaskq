package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Aryan9inja/gotaskq/internal/job"
	testutils "github.com/Aryan9inja/gotaskq/internal/testUtils"
	"github.com/redis/go-redis/v9"
)

func evalDequeueLua(t *testing.T, client redis.UniversalClient, ctx context.Context, q *RedisQueue, nowMs int64) (int64, string) {
	t.Helper()

	res, err := client.Eval(
		ctx,
		dequeueReadyLua,
		[]string{q.key, q.payloadKey},
		nowMs,
	).Result()
	if err != nil {
		t.Fatalf("Failed to evaluate Lua script: %v", err)
	}

	parts, ok := res.([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("Unexpected Lua script result: %v", res)
	}	

	code, err := redisInterfaceToInt64(parts[0])
	if err != nil {
		t.Fatalf("Invalid status code type: %v", err)
	}

	if code !=2{
		return code, ""
	}

	payload, err := redisStringValue(parts[1])
	if err != nil {
		t.Fatalf("Invalid payload type: %v", err)
	}

	return code, payload
}

func addJobToQueue(t *testing.T, ctx context.Context, client redis.UniversalClient, q *RedisQueue, j *job.Job, score int64, withPayload bool) string {
	t.Helper()

	member := q.memberForJob(j)
	if withPayload{
		payload, err := json.Marshal(j)
		if err != nil {
			t.Fatalf("Failed to marshal job payload: %v", err)
		}
		if err := client.HSet(ctx, q.payloadKey, member, payload).Err(); err != nil {
			t.Fatalf("Failed to set job payload in Redis: %v", err)
		}
	}

	if err := client.ZAdd(ctx, q.key, redis.Z{
		Score:  float64(score),
		Member: member,
	}).Err(); err != nil {
		t.Fatalf("Failed to add job to Redis sorted set: %v", err)
	}

	return member
}

func assertQueueState(t *testing.T, ctx context.Context, client redis.UniversalClient, q *RedisQueue, wantedZCard, wantedHLen int64) {
	t.Helper()

	zCard, err := client.ZCard(ctx, q.key).Result()
	if err != nil {
		t.Fatalf("Failed to get ZCard: %v", err)
	}

	if zCard != wantedZCard {
		t.Errorf("Expected ZCard %d, got %d", wantedZCard, zCard)
	}

	hLen, err := client.HLen(ctx, q.payloadKey).Result()
	if err != nil {
		t.Fatalf("Failed to get HLen: %v", err)
	}

	if hLen != wantedHLen {
		t.Errorf("Expected HLen %d, got %d", wantedHLen, hLen)
	}
}

func TestDequeueReadyLua(t *testing.T) {
	client := testutils.GetRedisClient(t)
	defer client.Close()

	ctx := context.Background()

	t.Run("Empty queue return code 0", func(t *testing.T) {
		testutils.ClearRedis(t, client)

		q, err := NewRedisQueue("test-queue", client)
		if err!=nil{
			t.Fatalf("Failed to create RedisQueue: %v", err)
		}

		code, payload := evalDequeueLua(t, client, ctx, q, time.Now().UnixMilli())
		if code != 0 {
			t.Errorf("Expected code 0 for empty queue, got %d", code)
		}
		if payload != "" {
			t.Errorf("Expected empty payload for empty queue, got %s", payload)
		}
	})

	t.Run("No jobs ready returns code 1 without removing", func(t *testing.T) {
		testutils.ClearRedis(t, client)

		q, _ := NewRedisQueue("test-queue", client)

		nowMs := time.Now().UnixMilli()
		j1 := testutils.NewTestJob("j1", 5, 0)
		addJobToQueue(t, ctx, client, q, j1, nowMs+5000, true)

		code, _ := evalDequeueLua(t, client, ctx, q, nowMs)
		if code != 1 {
			t.Errorf("Expected code 1 for no ready jobs, got %d", code)
		}

		assertQueueState(t, ctx, client, q, 1, 1)
	})

	t.Run("Ready job returns pauload and removes entries", func(t *testing.T) {
		testutils.ClearRedis(t, client)

		q, _ := NewRedisQueue("test-queue", client)

		nowMs := time.Now().UnixMilli()
		j1 := testutils.NewTestJob("j1", 5, 0)
		addJobToQueue(t, ctx, client, q, j1, nowMs-1000, true)

		code, payload := evalDequeueLua(t, client, ctx, q, nowMs)
		if code != 2 {
			t.Errorf("Expected code 2 for ready job, got %d", code)
		}

		var gotJob job.Job
		if err := json.Unmarshal([]byte(payload), &gotJob); err != nil {
			t.Fatalf("Failed to unmarshal payload: %v", err)
		}

		if gotJob.ID != j1.ID {
			t.Errorf("Expected job ID %s, got %s", j1.ID, gotJob.ID)
		}

		assertQueueState(t, ctx, client, q, 0, 0)
	})

	t.Run("Missing payload entries are skipped",func(t *testing.T) {
		testutils.ClearRedis(t, client)

		q, _ := NewRedisQueue("test-queue", client)

		nowMs := time.Now().UnixMilli()
		j1 := testutils.NewTestJob("j1", 5, 0)
		j2 := testutils.NewTestJob("j2", 5, 0)

		addJobToQueue(t, ctx, client, q, j1, nowMs-2000, false)
		addJobToQueue(t, ctx, client, q, j2, nowMs-1000, true)

		code, payload := evalDequeueLua(t, client, ctx, q, nowMs)
		if code != 2 {
			t.Errorf("Expected code 2, got %d", code)
		}

		var gotJob job.Job
		if err := json.Unmarshal([]byte(payload), &gotJob); err != nil {
			t.Fatalf("Failed to unmarshal payload: %v", err)
		}

		if gotJob.ID != j2.ID {
			t.Errorf("Expected job ID %s, got %s", j2.ID, gotJob.ID)
		}

		assertQueueState(t, ctx, client, q, 0, 0)
	})

	t.Run("Only missing payload entries return 0", func(t *testing.T) {
		testutils.ClearRedis(t, client)

		q, _ := NewRedisQueue("test-queue", client)

		nowMs := time.Now().UnixMilli()
		j1 := testutils.NewTestJob("j1", 5, 0)
		addJobToQueue(t, ctx, client, q, j1, nowMs-1000, false)

		code, payload := evalDequeueLua(t, client, ctx, q, nowMs)
		if code != 0 {
			t.Errorf("Expected code 0 when only missing payload entries are present, got %d", code)
		}
		if payload != "" {
			t.Errorf("Expected empty payload when only missing payload entries are present, got %s", payload)
		}

		assertQueueState(t, ctx, client, q, 0, 0)
	})
}