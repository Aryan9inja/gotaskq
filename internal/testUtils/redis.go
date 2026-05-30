package testutils

import (
	"context"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
)

func GetRedisClient(t *testing.T) redis.UniversalClient {
	t.Helper()

	if os.Getenv("USE_REDIS") != "true" {
		t.Skip("Use Redis is not set to true, skipping redis based tests")
	}

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl == "" {
		t.Skip("Redis Url is not set, skipping redis based tests")
	}

	opts, err := redis.ParseURL(redisUrl)
	if err != nil {
		t.Fatalf("Failed to parse redis url: %v", err)
	}

	opts.DB = redisTestDB(t)

	client := redis.NewClient(opts)

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("Failed to connect to redis: %v", err)
	}

	return client
}

func redisTestDB(t *testing.T) int {
	if raw := os.Getenv("REDIS_TEST_DB"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 15 {
			t.Fatalf("Failed to parse redis test db: %v", err)
		}
		return value
	}

	_, file, _, ok := runtime.Caller(2)
	if !ok {
		return 0
	}

	dir := filepath.Dir(file)
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(dir))
	// Return a value between 1 and 15, reserving 0 for manual testing and debugging
	return int(hasher.Sum32()%15) + 1
}

func ClearRedis(t *testing.T, client redis.UniversalClient) {
	t.Helper()

	ctx := context.Background()
	if err := client.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("Failed to flush redis: %v", err)
	}
}
