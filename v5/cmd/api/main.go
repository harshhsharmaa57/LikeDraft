package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"like-counter/internal/db"
	"like-counter/internal/handler"
	kafkapkg "like-counter/internal/kafka"
	redisclient "like-counter/internal/redis"
)

func main() {
	ctx := context.Background()

	dbPool, err := db.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer dbPool.Close()

	redis, err := redisclient.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer redis.Close()

	producer, err := kafkapkg.NewProducer(
		[]string{"localhost:9092"},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer producer.Close()

	h := handler.New(
		dbPool,
		redis,
		producer,
	)

	router := gin.Default()

	router.POST(
		"/posts/:id/like",
		h.Like,
	)

	router.GET(
		"/posts/:id",
		h.GetPost,
	)

	router.DELETE(
		"/posts/:id/likes",
		h.ResetLikes,
	)

	log.Println("API listening on :8080")

	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
