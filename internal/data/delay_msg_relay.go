package data

import (
	"azushop/internal/biz"
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
)

type DelayMsgRelayPublisher struct {
	data *Data
}

func NewDelayRelayPublisher(data *Data) biz.DelayMsgRelayPublisher {
	return &DelayMsgRelayPublisher{data: data}
}

func (p *DelayMsgRelayPublisher) PublishOrderCancelled(ctx context.Context, orderID int64) error {
	producer := p.data.GetKafkaProducer()
	orderCancelledMsg := &OrderCancelledMessage{
		OrderID: orderID,
	}
	payload, err := json.Marshal(orderCancelledMsg)
	if err != nil {
		return err
	}
	prodMsg := sarama.ProducerMessage{
		Topic: biz.KafkaTopicOrderCancelled,
		Value: sarama.ByteEncoder(payload),
	}
	_, _, err = producer.SendMessage(&prodMsg)
	return err
}

type DelayMsgRelaySubscriber struct {
	data *Data
}

func NewDelayMsgRelaySubscriber(data *Data) biz.DelayMsgRelaySubscriber {
	return &DelayMsgRelaySubscriber{data: data}
}

func (s *DelayMsgRelaySubscriber) SubscribeDelayMessage(ctx context.Context, handler func(orderID int64) error) error {
	topics := []string{biz.KafkaTopicOrderCancelledDelay}
	consumer := s.data.GetOrderConsumer()
	consumerHandler := NewDelayConsumerHandler(consumer, func(orderID int64) error {
		return handler(orderID)
	})
	for {
		err := s.data.GetOrderConsumer().Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

type DelayConsumerHandler struct {
	handler  func(orderID int64) error
	consumer sarama.ConsumerGroup
}

func NewDelayConsumerHandler(consumer sarama.ConsumerGroup, handler func(orderID int64) error) sarama.ConsumerGroupHandler {
	return &DelayConsumerHandler{
		handler:  handler,
		consumer: consumer,
	}
}

func (c *DelayConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *DelayConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *DelayConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	var delayTimer *time.Timer
	for {
		select {
		case <-session.Context().Done():
			return nil

		case msg, ok := <-claim.Messages():
			if !ok || msg == nil || len(msg.Value) == 0 {
				return nil
			}
			var orderCancelMsg OrderCancelledMessage
			if err := json.Unmarshal(msg.Value, &orderCancelMsg); err != nil {
				slog.Warn(err.Error())
				continue
			}
			now := time.Now()
			expiredTime := orderCancelMsg.ExpiredTime
			if now.Before(expiredTime) {
				delayTimer = time.NewTimer(time.Until(expiredTime))
				m := map[string][]int32{claim.Topic(): {claim.Partition()}}
				c.consumer.Pause(m)
				continue
			}
			if err := c.handler(orderCancelMsg.OrderID); err != nil {
				slog.Warn(err.Error())
			}
			session.MarkMessage(msg, "")

		case <-timeC(delayTimer):
			m := map[string][]int32{claim.Topic(): {claim.Partition()}}
			c.consumer.Resume(m)
		}
	}
}

func timeC(t *time.Timer) <-chan time.Time {
	if t == nil {
		return nil
	}
	return t.C
}
