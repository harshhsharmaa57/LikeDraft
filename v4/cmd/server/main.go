package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/harshhsharmaa57/LikeDraft/v4/internal/db"
	"github.com/harshhsharmaa57/LikeDraft/v4/internal/handler"
	redisclient "github.com/harshhsharmaa57/LikeDraft/v4/internal/redis"
)

func main() {
	ctx := context.Background()

	postgres, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("postgres connection failed: %v", err)
	}
	defer postgres.Close()

	redis, err := redisclient.Connect(ctx)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	defer redis.Close()

	h := handler.New(postgres, redis)

	router := gin.Default()

	router.POST("/posts/:id/like", h.Like)
	router.GET("/posts/:id", h.GetPost)

	// Benchmark/reset endpoint.
	router.DELETE("/posts/:id/likes", h.ResetLikes)

	log.Println("like-counter V4 listening on :8080")

	if err := router.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
