package testutils

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

func GetRedisClient(t *testing.T) redis.UniversalClient{
	t.Helper()

	if os.Getenv("USE_REDIS") != "true"{
		t.Skip("Use Redis is not set to true, skipping redis based tests")
	}

	redisUrl := os.Getenv("REDIS_URL")
	if redisUrl == ""{
		t.Skip("Redis Url is not set, skipping redis based tests")
	}

	opts, err := redis.ParseURL(redisUrl)
	if err != nil {
		t.Fatalf("Failed to parse redis url: %v", err)
	}

	client := redis.NewClient(opts)

	ctx := context.Background()
	if err := client.Ping(ctx).Err() ; err!=nil{
		t.Fatalf("Failed to connect to redis: %v", err)
	}

	return client
}

func ClearRedis(t *testing.T, client redis.UniversalClient){
	t.Helper()

	ctx := context.Background()
	if err := client.FlushDB(ctx).Err(); err!=nil{
		t.Fatalf("Failed to flush redis: %v", err)
	}
}