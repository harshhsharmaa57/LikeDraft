package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"

	"like-counter/internal/events"
)

const LikesTopic = "likes"

type Producer struct {
	client *kgo.Client
}

func NewProducer(brokers []string) (*Producer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)

	if err != nil {
		return nil, fmt.Errorf("create kafka producer: %w", err)
	}

	return &Producer{
		client: client,
	}, nil
}

func (p *Producer) PublishLike(
	ctx context.Context,
	event events.LikeEvent,
) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal like event: %w", err)
	}

	record := &kgo.Record{
		Topic: LikesTopic,
		Key:   []byte(event.UserID),
		Value: data,
	}

	result := p.client.ProduceSync(ctx, record)

	if err := result.Err(); err != nil {
		return fmt.Errorf("publish like event: %w", err)
	}

	return nil
}

func (p *Producer) Close() {
	p.client.Close()
}
