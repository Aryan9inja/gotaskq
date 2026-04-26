package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/Aryan9inja/gotaskq/config"
	"github.com/Aryan9inja/gotaskq/internal/api"
	"github.com/Aryan9inja/gotaskq/internal/dlq"
	"github.com/Aryan9inja/gotaskq/internal/handler"
	"github.com/Aryan9inja/gotaskq/internal/job"
	"github.com/Aryan9inja/gotaskq/internal/queue"
	"github.com/Aryan9inja/gotaskq/internal/retry"
	"github.com/Aryan9inja/gotaskq/internal/worker"
	"github.com/Aryan9inja/gotaskq/pkg/snowflake"
	"github.com/redis/go-redis/v9"
)

type logHandler struct{}

func (logHandler) Handle(ctx context.Context, job *job.Job) error {
	if job.Type != "logger" {
		return errors.New("logger can only be accessed by logger types")
	}

	fmt.Println("Job started")

	fmt.Println("JobID:", job.ID)
	fmt.Println("Type:", job.Type)
	fmt.Println("Status:", job.Status)

	time.Sleep(30 * time.Second)

	fmt.Println("Job Done")

	return nil
}

func main() {
	log.Println("Starting the service")
	// 1. Load the config
	cfg := config.LoadConfig()

	// 2. Create a queue + job store backend from config
	var (
		jobStore  job.Store
		mainQueue queue.Queue
		dlqStore dlq.DlqInterface
	)

	if cfg.UseRedis {
		if cfg.RedisUrl == "" {
			log.Fatal("USE_REDIS = true but REDIS_URL is empty")
		}

		redisOpt, err := redis.ParseURL(cfg.RedisUrl)
		if err != nil {
			log.Fatalf("invalid REDIS_URL: %v", err)
		}

		redisClient := redis.NewClient(redisOpt)
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			log.Fatalf("failed to connect to redis: %v", err)
		}
		defer func() {
			if err := redisClient.Close(); err != nil {
				log.Printf("failed to close redis client: %v", err)
			}
		}()

		redisStore,err := job.NewRedisStore(redisClient)
		if err != nil {
			log.Fatalf("failed to create redis job store: %v", err)
		}

		redisQueue, err:=queue.NewRedisQueue("default", redisClient)
		if err != nil {
			log.Fatalf("failed to create redis queue: %v", err)
		}

		redisDlq, err := dlq.NewRedisDlq(redisClient)
		if err != nil {
			log.Fatalf("failed to create redis dlq store: %v", err)
		}

		jobStore = redisStore
		mainQueue = redisQueue
		dlqStore = redisDlq
	}else{
		jobStore = job.NewMemoryStore()
		mainQueue = queue.NewMemoryQueue("default")
		dlqStore = nil
	}

	// 3. Create an ID generator
	snowflakeGen := snowflake.New(1)

	// 4. Register the created queue
	queueManager := queue.NewQueueManager()
	err := queueManager.Register(mainQueue)
	if err != nil {
		log.Fatalf("error registering queue: %v", err)
	}

	// 5. Create hanlder register
	handlerRegistry := handler.NewRegistry()

	// 6. Register test handler
	handlerRegistry.Register("logger", logHandler{})

	// 7. Create a retry engine
	retryEngine := retry.NewRetryEngine(jobStore, mainQueue, time.Duration(cfg.MaxDelay), dlqStore)

	// 8. Create a worker pool
	workerPool := worker.NewWorkerPool(
		context.Background(),
		mainQueue, jobStore,
		handlerRegistry,
		retryEngine,
		cfg.NumWorkers,
	)

	// 9. Start worker pool
	workerPool.Start()

	// 10. Create http server
	apiServer := api.NewServer(jobStore, queueManager, snowflakeGen, dlqStore)

	// 11. Spawn server in a goroutine
	go func() {
		err := apiServer.Start(":" + cfg.Port)
		if err != nil {
			log.Fatalf("can't start http server on :%s: %v", cfg.Port, err)
		}
	}()

	// 13. Generate ctx for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Println("Started the service and blocking")
	// 14. Block at done
	<-ctx.Done()

	fmt.Println("Blocking unblocked")
	// 15. Stop worker pool if signal recieved
	workerPool.Stop()
}
