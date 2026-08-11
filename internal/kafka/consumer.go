package kafka

import (
	"context"
	"encoding/json"
	"log"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

type normalRange struct {
	min float64
	max float64
}

// Normal ranges by LOINC code. Add new codes here to extend anomaly detection.
var normalRanges = map[string]normalRange{
	"8480-6": {min: 90, max: 140}, // systolic blood pressure
}

type Consumer struct {
	reader *kafkago.Reader
}

func NewConsumer(broker string) *Consumer {
	return &Consumer{
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: []string{broker},
			Topic:   ObservationCreatedTopic,
			GroupID: "fhir-health-service-anomaly-detector",
		}),
	}
}

func (c *Consumer) Run(ctx context.Context) {
	for {
		msg, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("kafka consumer read error: %v", err)
			continue
		}

		var observation models.Observation
		if err := json.Unmarshal(msg.Value, &observation); err != nil {
			log.Printf("kafka consumer: failed to unmarshal observation: %v", err)
			continue
		}

		checkAnomaly(&observation)
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}

func checkAnomaly(observation *models.Observation) {
	if observation.ValueQuantity == nil {
		return
	}

	code := observationCode(observation)
	r, ok := normalRanges[code]
	if !ok {
		return
	}

	value := observation.ValueQuantity.Value
	if value < r.min || value > r.max {
		log.Printf("ALERT: abnormal observation for patient %s: value %v outside normal range [%v, %v] (code %s)",
			observation.Subject.Reference, value, r.min, r.max, code)
	}
}

func observationCode(observation *models.Observation) string {
	for _, coding := range observation.Code.Coding {
		if coding.System == "http://loinc.org" && coding.Code != "" {
			return coding.Code
		}
	}
	if len(observation.Code.Coding) > 0 {
		return observation.Code.Coding[0].Code
	}
	return ""
}
