package kafka

import (
	"context"
	"encoding/json"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

const ObservationCreatedTopic = "observation.created"

type Producer struct {
	writer *kafkago.Writer
}

func NewProducer(broker string) *Producer {
	return &Producer{
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(broker),
			Topic:                  ObservationCreatedTopic,
			Balancer:               &kafkago.LeastBytes{},
			AllowAutoTopicCreation: true,
		},
	}
}

func (p *Producer) PublishObservationCreated(ctx context.Context, observation *models.Observation) error {
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}

	return p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(observation.ID),
		Value: payload,
	})
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
