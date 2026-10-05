package handler

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const NumShards = 32

type Handler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func New(db *pgxpool.Pool, redisClient *redis.Client) *Handler {
	return &Handler{
		db:    db,
		redis: redisClient,
	}
}

func counterKey(postID int64, shard int) string {
	return fmt.Sprintf("post:%d:likes:%d", postID, shard)
}

// shardForUser deterministically maps a user ID to one of 32 shards.
func shardForUser(userID string) int {
	hash := fnv.New32a()

	_, _ = hash.Write([]byte(userID))

	return int(hash.Sum32() % NumShards)
}

// POST /posts/:id/like
func (h *Handler) Like(c *gin.Context) {
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid post id",
		})
		return
	}

	userID := c.GetHeader("X-User-ID")

	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "X-User-ID header is required",
		})
		return
	}

	shard := shardForUser(userID)
	key := counterKey(postID, shard)

	count, err := h.redis.Incr(c.Request.Context(), key).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to increment like counter",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"post_id":    postID,
		"shard":      shard,
		"like_count": count,
	})
}

// GET /posts/:id
func (h *Handler) GetPost(c *gin.Context) {
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid post id",
		})
		return
	}

	ctx := c.Request.Context()

	var title string

	err = h.db.QueryRow(
		ctx,
		"SELECT title FROM posts WHERE id = $1",
		postID,
	).Scan(&title)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "post not found",
		})
		return
	}

	keys := make([]string, NumShards)

	for shard := 0; shard < NumShards; shard++ {
		keys[shard] = counterKey(postID, shard)
	}

	values, err := h.redis.MGet(ctx, keys...).Result()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to read like counters",
		})
		return
	}

	var totalLikes int64

	for _, value := range values {
		if value == nil {
			continue
		}

		switch v := value.(type) {
		case string:
			count, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "invalid counter value",
				})
				return
			}

			totalLikes += count

		case []byte:
			count, err := strconv.ParseInt(string(v), 10, 64)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "invalid counter value",
				})
				return
			}

			totalLikes += count
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"post_id":    postID,
		"title":      title,
		"like_count": totalLikes,
	})
}

// DELETE /posts/:id/likes
//
// Benchmark/reset helper. Remove this endpoint later if you want a
// production-style API without administrative reset functionality.
func (h *Handler) ResetLikes(c *gin.Context) {
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid post id",
		})
		return
	}

	ctx := context.Background()

	keys := make([]string, NumShards)

	for shard := 0; shard < NumShards; shard++ {
		keys[shard] = counterKey(postID, shard)
	}

	if err := h.redis.Del(ctx, keys...).Err(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to reset counters",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"post_id":    postID,
		"like_count": 0,
	})
}
