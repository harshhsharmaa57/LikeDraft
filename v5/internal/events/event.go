package events

import (
	"time"
)

const LikeCreated = "like_created"

type LikeEvent struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	UserID    string    `json:"user_id"`
	PostID    int64     `json:"post_id"`
	CreatedAt time.Time `json:"created_at"`
}
