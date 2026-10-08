package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/harshhsharmaa57/LikeDraft/v5/internal/events"
	kafkapkg "github.com/harshhsharmaa57/LikeDraft/v5/internal/kafka"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const NumShards = 32

type Handler struct {
	db       *pgxpool.Pool
	redis    *redis.Client
	producer *kafkapkg.Producer
}

func New(
	db *pgxpool.Pool,
	redisClient *redis.Client,
	producer *kafkapkg.Producer,
) *Handler {
	return &Handler{
		db:       db,
		redis:    redisClient,
		producer: producer,
	}
}

func (h *Handler) Like(c *gin.Context) {
	postID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)

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

	event := events.LikeEvent{
		EventID:   newEventID(),
		EventType: events.LikeCreated,
		UserID:    userID,
		PostID:    postID,
		CreatedAt: time.Now().UTC(),
	}

	if err := h.producer.PublishLike(
		c.Request.Context(),
		event,
	); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to publish like event",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"accepted": true,
		"event_id": event.EventID,
		"post_id":  postID,
		"user_id":  userID,
	})
}

func (h *Handler) GetPost(c *gin.Context) {
	postID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid post id",
		})
		return
	}

	var title string

	err = h.db.QueryRow(
		c.Request.Context(),
		"SELECT title FROM posts WHERE id = $1",
		postID,
	).Scan(&title)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "post not found",
		})
		return
	}

	keys := make([]string, 0, NumShards)

	for shard := 0; shard < NumShards; shard++ {
		keys = append(
			keys,
			fmt.Sprintf(
				"post:%d:likes:%d",
				postID,
				shard,
			),
		)
	}

	values, err := h.redis.MGet(
		c.Request.Context(),
		keys...,
	).Result()

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to read like counters",
		})
		return
	}

	var total int64

	for _, value := range values {
		switch v := value.(type) {
		case string:
			n, err := strconv.ParseInt(v, 10, 64)
			if err == nil {
				total += n
			}

		case nil:
			// Missing shard means zero.

		default:
			_ = v
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         postID,
		"like_count": total,
		"title":      title,
	})
}

func (h *Handler) ResetLikes(c *gin.Context) {
	postID, err := strconv.ParseInt(
		c.Param("id"),
		10,
		64,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid post id",
		})
		return
	}

	ctx := c.Request.Context()

	for shard := 0; shard < NumShards; shard++ {
		counterKey := fmt.Sprintf(
			"post:%d:likes:%d",
			postID,
			shard,
		)

		if err := h.redis.Del(ctx, counterKey).Err(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to delete counter",
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
				if err := h.redis.Del(
					ctx,
					keys...,
				).Err(); err != nil {
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
		"like_count": 0,
		"post_id":    postID,
	})
}

func newEventID() string {
	return fmt.Sprintf(
		"%d-%d",
		time.Now().UnixNano(),
		time.Now().UnixNano(),
	)
}

func _contextUsed(_ context.Context) {}
