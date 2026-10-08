package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/harshhsharmaa57/LikeDraft/v5/internal/events"
	"github.com/redis/go-redis/v9"
	"github.com/twmb/franz-go/pkg/kgo"
)

const (
	ConsumerGroup = "like-counter-aggregator"
	NumShards     = 32
)

const likeScript = `
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
`

type Consumer struct {
	client *kgo.Client
	redis  *redis.Client
}

func NewConsumer(
	brokers []string,
	redisClient *redis.Client,
) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(ConsumerGroup),
		kgo.ConsumeTopics(LikesTopic),
		kgo.DisableAutoCommit(),
	)

	if err != nil {
		return nil, fmt.Errorf("create kafka consumer: %w", err)
	}

	return &Consumer{
		client: client,
		redis:  redisClient,
	}, nil
}

func (c *Consumer) Run(ctx context.Context) error {
	log.Println("like consumer started")

	for {
		fetches := c.client.PollFetches(ctx)

		if fetches.IsClientClosed() {
			return nil
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			for _, err := range errs {
				log.Printf(
					"kafka fetch error: topic=%s partition=%d error=%v",
					err.Topic,
					err.Partition,
					err.Err,
				)
			}

			continue
		}

		fetches.EachRecord(func(record *kgo.Record) {
			if err := c.processRecord(ctx, record); err != nil {
				log.Printf(
					"failed to process record topic=%s partition=%d offset=%d: %v",
					record.Topic,
					record.Partition,
					record.Offset,
					err,
				)

				return
			}

			c.client.MarkCommitRecords(record)
		})

		if err := c.client.CommitRecords(ctx); err != nil {
			log.Printf("kafka commit error: %v", err)
		}
	}
}

func (c *Consumer) processRecord(
	ctx context.Context,
	record *kgo.Record,
) error {
	var event events.LikeEvent

	if err := json.Unmarshal(record.Value, &event); err != nil {
		return fmt.Errorf("decode like event: %w", err)
	}

	if event.EventType != events.LikeCreated {
		return fmt.Errorf("unknown event type: %s", event.EventType)
	}

	shard := shardForUser(event.UserID)

	userKey := fmt.Sprintf(
		"post:%d:likes:%d:users:%s",
		event.PostID,
		shard,
		event.UserID,
	)

	counterKey := fmt.Sprintf(
		"post:%d:likes:%d",
		event.PostID,
		shard,
	)

	_, err := c.redis.Eval(
		ctx,
		likeScript,
		[]string{userKey, counterKey},
	).Result()

	if err != nil {
		return fmt.Errorf("redis like aggregation failed: %w", err)
	}

	return nil
}

func (c *Consumer) Close() {
	c.client.Close()
}

func shardForUser(userID string) int {
	var hash uint32 = 2166136261

	for i := 0; i < len(userID); i++ {
		hash ^= uint32(userID[i])
		hash *= 16777619
	}

	return int(hash % NumShards)
}

