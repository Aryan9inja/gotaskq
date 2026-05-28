package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/Aryan9inja/gotaskq/taskq"
)

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Struct handler are useful when the handler has dependecies such as
// a mailer, database client, logger, or domain services
type emailHandler struct {
	fromAddress string
}

func (h emailHandler) Handle(ctx context.Context, job *taskq.Job) error {
	var payload EmailPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		fmt.Printf("Sending email from %s to %s with subject %s and body %s\n", h.fromAddress, payload.To, payload.Subject, payload.Body)
		return nil
	}
}

type ReportPayload struct {
	AccountID string `json:"account_id"`
	Format    string `json:"format"`
	Requested string `json:"requested"`
}

func main() {
	server, err := taskq.New(taskq.Options{
		Addr:          ":8080",
		NumWorkers:    2,
		MaxRetryDelay: 10 * time.Second,

		// To use Redis pass configuration directly
		// UseRedis: true,
		// RedisUrl: "redis://localhost:6379",
	})
	if err != nil {
		log.Fatalf("failed to create taskq server: %v", err)
	}

	// Pattern 1: register a struct that implements taskq.Handler interface
	if err := server.Register("email", emailHandler{fromAddress: "notifications@xyz.com"}); err != nil {
		log.Fatalf("failed to register email handler: %v", err)
	}

	// Pattern 2: register a function using taskq.HandlerFunc adapter
	if err := server.RegisterFunc("report.genrate", func(ctx context.Context, job *taskq.Job) error {
		var payload ReportPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return err
		}

		log.Printf("Generating report for account %s in format %s requested at %s\n", payload.AccountID, payload.Format, payload.Requested)
		return nil
	}); err != nil {
		log.Fatalf("failed to register report generation handler: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	select {
	case <-ctx.Done():
		if err := server.Stop(); err != nil {
			log.Fatalf("failed to stop server: %v", err)
		}
	case err := <-errCh:
		if err != nil {
			log.Fatalf("server error: %v", err)
		}
	}
}
