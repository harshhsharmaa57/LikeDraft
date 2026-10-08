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

func shardForUser(userID string) int {
	hash := fnv.New32a()

	_, _ = hash.Write([]byte(userID))

	return int(hash.Sum32() % NumShards)
}

func counterKey(postID int64, shard int) string {
	return fmt.Sprintf("post:%d:likes:%d", postID, shard)
}

func userLikeKey(postID int64, shard int, userID string) string {
	return fmt.Sprintf(
		"post:%d:likes:%d:users:%s",
		postID,
		shard,
		userID,
	)
}

// Atomic operation:
//
// 1. Check whether this user already liked the post.
// 2. If not, record the like.
// 3. Increment the shard counter.
// 4. Return whether this request created a new like.
//
// Lua executes atomically inside Redis.
var likeScript = redis.NewScript(`
local created = redis.call("SETNX", KEYS[1], "1")

if created == 1 then
    local count = redis.call("INCR", KEYS[2])
    return {1, count}
end

local current = redis.call("GET", KEYS[2])

if not current then
    current = "0"
end

return {0, current}
`)

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

	stateKey := userLikeKey(postID, shard, userID)
	counterKey := counterKey(postID, shard)

	result, err := likeScript.Run(
		c.Request.Context(),
		h.redis,
		[]string{
			stateKey,
			counterKey,
		},
	).Result()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to process like",
		})
		return
	}

	values, ok := result.([]interface{})
	if !ok || len(values) != 2 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "invalid redis response",
		})
		return
	}

	created, err := redisInt64(values[0])
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "invalid like state response",
		})
		return
	}

	shardCount, err := redisInt64(values[1])
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "invalid counter response",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"post_id":      postID,
		"user_id":      userID,
		"shard":        shard,
		"created":      created == 1,
		"shard_count":  shardCount,
	})
}

func redisInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil

	case string:
		return strconv.ParseInt(v, 10, 64)

	case []byte:
		return strconv.ParseInt(string(v), 10, 64)

	default:
		return 0, fmt.Errorf("unsupported redis value type %T", value)
	}
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

		count, err := redisInt64(value)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "invalid counter value",
			})
			return
		}

		totalLikes += count
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         postID,
		"title":      title,
		"like_count": totalLikes,
	})
}

// DELETE /posts/:id/likes
//
// Benchmark/reset endpoint.
// It removes all 32 counter keys and all user-like state keys
// belonging to the post.
//
// This endpoint is for experimentation, not normal production use.
func (h *Handler) ResetLikes(c *gin.Context) {
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid post id",
		})
		return
	}

	ctx := context.Background()

	for shard := 0; shard < NumShards; shard++ {
		counter := counterKey(postID, shard)

		if err := h.redis.Del(ctx, counter).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to reset counter",
			})
			return
		}

		pattern := fmt.Sprintf(
			"post:%d:likes:%d:users:*",
			postID,
			shard,
		)

		var cursor uint64

		for {
			keys, nextCursor, err := h.redis.Scan(
				ctx,
				cursor,
				pattern,
				500,
			).Result()

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error": "failed to scan like state",
				})
				return
			}

			if len(keys) > 0 {
				if err := h.redis.Del(ctx, keys...).Err(); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"error": "failed to delete like state",
					})
					return
				}
			}

			cursor = nextCursor

			if cursor == 0 {
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"post_id":    postID,
		"like_count": 0,
	})
}