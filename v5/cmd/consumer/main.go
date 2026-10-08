package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/harshhsharmaa57/LikeDraft/v5/internal/kafka"
	redisclient "github.com/harshhsharmaa57/LikeDraft/v5/internal/redis"
)

func main() {
	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer cancel()

	redis, err := redisclient.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer redis.Close()

	consumer, err := kafka.NewConsumer(
		[]string{"localhost:9092"},
		redis,
	)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.Close()

	if err := consumer.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
