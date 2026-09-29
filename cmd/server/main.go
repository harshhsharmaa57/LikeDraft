package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/harshhsharmaa57/like-counter/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	db *pgxpool.Pool
}

type LikeResponse struct {
	PostID    int64 `json:"post_id"`
	LikeCount int64 `json:"like_count"`
}

func (s *Server) likePost(w http.ResponseWriter, r *http.Request) {
	postIDStr := r.PathValue("id")

	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var likeCount int64

	err = s.db.QueryRow(
		ctx,
		`
		UPDATE posts
		SET like_count = like_count + 1
		WHERE id = $1
		RETURNING like_count
		`,
		postID,
	).Scan(&likeCount)

	if err != nil {
		http.Error(w, "failed to like post", http.StatusInternalServerError)
		return
	}

	response := LikeResponse{
		PostID:    postID,
		LikeCount: likeCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}


func (s *Server) getPost(w http.ResponseWriter, r *http.Request) {
	postIDStr := r.PathValue("id")

	postID, err := strconv.ParseInt(postIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid post id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var title string
	var likeCount int64

	err = s.db.QueryRow(
		ctx,
		`
		SELECT title, like_count
		FROM posts
		WHERE id = $1
		`,
		postID,
	).Scan(&title, &likeCount)

	if err != nil {
		http.Error(w, "post not found", http.StatusNotFound)
		return
	}

	response := map[string]any{
		"id":         postID,
		"title":      title,
		"like_count": likeCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	ctx := context.Background()

	pool, err := db.Connect(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	server := &Server{
		db: pool,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /posts/{id}/like", server.likePost)
	mux.HandleFunc("GET /posts/{id}", server.getPost)
	log.Println("server running on :8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
