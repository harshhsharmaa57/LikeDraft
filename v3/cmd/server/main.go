package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	db "github.com/harshhsharmaa57/LikeDraft/v2/internal/db"
	redisdb "github.com/harshhsharmaa57/LikeDraft/v2/internal/redis"
)

type Server struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

type LikeResponse struct {
	PostID    int64 `json:"post_id"`
	LikeCount int64 `json:"like_count"`
}

type PostResponse struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	LikeCount int64  `json:"like_count"`
}

func main() {
	ctx := context.Background()

	// -----------------------------------------
	// PostgreSQL
	// -----------------------------------------

	pgPool, err := db.Connect(ctx)
	if err != nil {
		log.Fatalf("postgres connection failed: %v", err)
	}

	defer pgPool.Close()

	log.Println("connected to PostgreSQL")

	// -----------------------------------------
	// Redis
	// -----------------------------------------

	redisClient, err := redisdb.Connect(ctx)
	if err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}

	defer redisClient.Close()

	log.Println("connected to Redis")

	// -----------------------------------------
	// Server
	// -----------------------------------------

	server := &Server{
		db:    pgPool,
		redis: redisClient,
	}

	// -----------------------------------------
	// Routes
	// -----------------------------------------

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", server.health)

	mux.HandleFunc("GET /posts/{id}", server.getPost)

	mux.HandleFunc("POST /posts/{id}/like", server.likePost)

	// -----------------------------------------
	// HTTP server
	// -----------------------------------------

	httpServer := &http.Server{
		Addr:    ":8080",
		Handler: mux,

		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Println("server running on http://localhost:8080")

	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

// --------------------------------------------------
// Health Check
// --------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Check PostgreSQL.
	if err := s.db.Ping(ctx); err != nil {
		http.Error(
			w,
			"postgres unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	// Check Redis.
	if err := s.redis.Ping(ctx).Err(); err != nil {
		http.Error(
			w,
			"redis unavailable",
			http.StatusServiceUnavailable,
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// --------------------------------------------------
// Like Post
// --------------------------------------------------

func (s *Server) likePost(w http.ResponseWriter, r *http.Request) {
	postIDStr := r.PathValue("id")

	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		http.Error(
			w,
			"invalid post id",
			http.StatusBadRequest,
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		2*time.Second,
	)
	defer cancel()

	// Redis key.
	//
	// Example:
	//
	// post:1:likes
	//
	key := fmt.Sprintf("post:%d:likes", postID)

	// Atomic Redis increment.
	likeCount, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		log.Printf(
			"redis increment failed for post %d: %v",
			postID,
			err,
		)

		http.Error(
			w,
			"failed to increment like count",
			http.StatusInternalServerError,
		)

		return
	}

	response := LikeResponse{
		PostID:    postID,
		LikeCount: likeCount,
	}

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

// --------------------------------------------------
// Get Post
// --------------------------------------------------

func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	postIDStr := r.PathValue("id")

	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		http.Error(
			w,
			"invalid post id",
			http.StatusBadRequest,
		)
		return
	}

	ctx, cancel := context.WithTimeout(
		r.Context(),
		2*time.Second,
	)
	defer cancel()

	// Get post information from PostgreSQL.
	var (
		title string
	)

	err = s.db.QueryRow(
		ctx,
		`
		SELECT title
		FROM posts
		WHERE id = $1
		`,
		postID,
	).Scan(&title)

	if err != nil {
		http.Error(
			w,
			"post not found",
			http.StatusNotFound,
		)

		return
	}

	// Get like count from Redis.
	key := fmt.Sprintf(
		"post:%d:likes",
		postID,
	)

	likeCount, err := s.redis.Get(
		ctx,
		key,
	).Int64()

	if err != nil {

		// Redis returns redis.Nil when the key
		// doesn't exist.
		//
		// In that case we treat the count as zero.

		if err == redis.Nil {
			likeCount = 0
		} else {
			log.Printf(
				"redis get failed for post %d: %v",
				postID,
				err,
			)

			http.Error(
				w,
				"failed to retrieve like count",
				http.StatusInternalServerError,
			)

			return
		}
	}

	response := PostResponse{
		ID:        postID,
		Title:     title,
		LikeCount: likeCount,
	}

	writeJSON(
		w,
		http.StatusOK,
		response,
	)
}

// --------------------------------------------------
// JSON helper
// --------------------------------------------------

func writeJSON(
	w http.ResponseWriter,
	status int,
	data any,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)

	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf(
			"failed to encode response: %v",
			err,
		)
	}
}
