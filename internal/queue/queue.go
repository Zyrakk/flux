package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	log "github.com/sirupsen/logrus"
)

// Subjects for NATS messaging.
const (
	SubjectArticlesNew       = "articles.new"
	SubjectArticlesProcessed = "articles.processed"
	SubjectBriefingGenerate  = "briefing.generate"
)

// Stream names.
const (
	StreamArticles = "ARTICLES"
	StreamBriefing = "BRIEFING"
)

// Queue wraps a NATS JetStream connection.
type Queue struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

// MessageHandler is a callback for processing received messages.
type MessageHandler func(data []byte) error

// New connects to NATS and sets up JetStream streams.
func New(natsURL string) (*Queue, error) {
	conn, err := nats.Connect(natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(60),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.WithError(err).Warn("NATS disconnected")
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to NATS: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("getting JetStream context: %w", err)
	}

	q := &Queue{conn: conn, js: js}
	if err := q.ensureStreams(); err != nil {
		conn.Close()
		return nil, err
	}

	log.Info("Connected to NATS JetStream")
	return q, nil
}

// ensureStreams creates the required streams if they don't exist.
func (q *Queue) ensureStreams() error {
	streams := []nats.StreamConfig{
		{
			Name:      StreamArticles,
			Subjects:  []string{"articles.>"},
			Retention: nats.WorkQueuePolicy,
			MaxAge:    72 * time.Hour,
			Storage:   nats.FileStorage,
		},
		{
			Name:      StreamBriefing,
			Subjects:  []string{"briefing.>"},
			Retention: nats.WorkQueuePolicy,
			MaxAge:    24 * time.Hour,
			Storage:   nats.FileStorage,
		},
	}

	for _, cfg := range streams {
		if _, err := q.js.StreamInfo(cfg.Name); err != nil {
			if _, err := q.js.AddStream(&cfg); err != nil {
				return fmt.Errorf("creating stream %s: %w", cfg.Name, err)
			}
			log.WithField("stream", cfg.Name).Info("Created NATS stream")
		}
	}
	return nil
}

// Publish serializes data as JSON and publishes to the given subject.
func (q *Queue) Publish(subject string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshalling message: %w", err)
	}

	_, err = q.js.Publish(subject, payload)
	if err != nil {
		return fmt.Errorf("publishing to %s: %w", subject, err)
	}
	return nil
}

// Subscribe creates a durable pull subscription and processes messages with the handler.
func (q *Queue) Subscribe(ctx context.Context, subject, durable string, handler MessageHandler) error {
	sub, err := q.js.PullSubscribe(subject, durable)
	if err != nil {
		return fmt.Errorf("subscribing to %s: %w", subject, err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				log.WithField("subject", subject).Info("Subscription shutting down")
				return
			default:
			}

			msgs, err := sub.Fetch(10, nats.MaxWait(5*time.Second))
			if err != nil {
				if err == nats.ErrTimeout {
					continue
				}
				log.WithError(err).WithField("subject", subject).Error("Error fetching messages")
				time.Sleep(time.Second)
				continue
			}

			for _, msg := range msgs {
				if err := handler(msg.Data); err != nil {
					log.WithError(err).WithField("subject", subject).Error("Error processing message")
					if err := msg.Nak(); err != nil {
						log.WithError(err).Warn("Failed to NAK message")
					}
					continue
				}
				if err := msg.Ack(); err != nil {
					log.WithError(err).Warn("Failed to ACK message")
				}
			}
		}
	}()

	return nil
}

// Close gracefully closes the NATS connection.
func (q *Queue) Close() {
	if err := q.conn.Drain(); err != nil {
		log.WithError(err).Warn("Failed to drain NATS connection")
	}
}
