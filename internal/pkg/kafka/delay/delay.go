package delay

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"github.com/azusayn/azushop/internal/pkg/kafka"
	"github.com/dnwe/otelsarama"
	"go.opentelemetry.io/otel"
)

const (
	kafkaTopicDelayQueue kafka.TopicType = "delay_queue"
	defaultDelay         time.Duration   = 30 * time.Second
)

type Worker struct {
	publisher kafka.Publisher
	scope     string
	// TODO: support multi-level delay queues.
	delay         time.Duration
	consumerGroup sarama.ConsumerGroup
}

func NewWorker(scope string, publisher kafka.Publisher, consumerGroup sarama.ConsumerGroup) *Worker {
	return &Worker{
		publisher:     publisher,
		consumerGroup: consumerGroup,
		scope:         scope,
		delay:         defaultDelay,
	}
}

func (d *Worker) Run(ctx context.Context) error {
	handler := newConsumerHandler(d.consumerGroup, d.publisher)
	return d.consumerGroup.Consume(
		ctx,
		[]string{string(kafkaTopicDelayQueue.WithScope(d.scope))},
		handler,
	)
}

func (d *Worker) SendMessage(ctx context.Context, message kafka.Message) error {
	msg := &kafka.Message{
		Topic: kafkaTopicDelayQueue.WithScope(d.scope),
		Value: delayMessage{
			Message:   message,
			ExpiresAt: time.Now().Add(d.delay),
		},
	}
	return d.publisher.SendMessages(ctx, []*kafka.Message{msg})
}

type delayMessage struct {
	Message   kafka.Message
	ExpiresAt time.Time
}

type consumerHandler struct {
	consumer  sarama.ConsumerGroup
	publisher kafka.Publisher
}

func newConsumerHandler(consumer sarama.ConsumerGroup, publisher kafka.Publisher) sarama.ConsumerGroupHandler {
	h := &consumerHandler{
		consumer:  consumer,
		publisher: publisher,
	}
	return otelsarama.WrapConsumerGroupHandler(h)
}

func (c *consumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *consumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *consumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	var delayTimer *time.Timer
	for {
		select {
		case <-session.Context().Done():
			return nil

		case msg, ok := <-claim.Messages():
			if !ok || msg == nil || len(msg.Value) == 0 {
				return nil
			}

			var delayMsg delayMessage
			if err := json.Unmarshal(msg.Value, &delayMsg); err != nil {
				slog.Warn("failed to unmarshal delay message", slog.Any("err", err))
				continue
			}

			now := time.Now()
			expiresAt := delayMsg.ExpiresAt
			if now.Before(expiresAt) {
				delayTimer = time.NewTimer(time.Until(expiresAt))
				m := map[string][]int32{claim.Topic(): {claim.Partition()}}
				c.consumer.Pause(m)
				continue

			}

			ctx := otel.GetTextMapPropagator().Extract(
				session.Context(),
				otelsarama.NewConsumerMessageCarrier(msg),
			)

			// re-send the message to the original topic
			if err := c.publisher.SendMessages(ctx, []*kafka.Message{&delayMsg.Message}); err != nil {
				slog.Warn("failed to send message to original topic", slog.Any("err", err), slog.Any("topic", delayMsg.Message.Topic))
				continue
			}

			session.MarkMessage(msg, "")

		case <-timeCh(delayTimer):
			c.consumer.Resume(map[string][]int32{claim.Topic(): {claim.Partition()}})
		}
	}
}

func timeCh(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
