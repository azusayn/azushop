package data

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"
	"go.opentelemetry.io/otel"

	"github.com/azusayn/azushop/internal/biz"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/google/wire"
)

var DelayMsgRelayDataProviderSet = wire.NewSet(
	NewDelayRelayPublisher,
	NewDelayMsgRelaySubscriber,
)

type DelayMsgRelayPublisher struct {
	kafkaProducer *KafkaProducer
}

func NewDelayRelayPublisher(config *conf.Data) (biz.DelayMsgRelayPublisher, error) {
	producer, err := NewKafkaProducer(config)
	if err != nil {
		return nil, err
	}
	return &DelayMsgRelayPublisher{kafkaProducer: producer}, nil
}

func (p *DelayMsgRelayPublisher) PublishOrderCancelled(ctx context.Context, orderID int64) error {
	producer := p.kafkaProducer.syncProducer
	orderCancelledMsg := &biz.OrderCancelledMessage{
		OrderID: orderID,
	}
	payload, err := json.Marshal(orderCancelledMsg)
	if err != nil {
		return err
	}
	prodMsg := sarama.ProducerMessage{
		Topic: string(biz.KafkaTopicOrderCancelled),
		Value: sarama.ByteEncoder(payload),
	}
	otel.GetTextMapPropagator().Inject(ctx, otelsarama.NewProducerMessageCarrier(&prodMsg))
	_, _, err = producer.SendMessage(&prodMsg)
	return err
}

type DelayMsgRelaySubscriber struct {
	delayMessageRelaySub sarama.ConsumerGroup
}

func NewDelayMsgRelaySubscriber(config *conf.Data) (biz.DelayMsgRelaySubscriber, error) {
	sub, err := NewConsumerGroup(config.GetKafka().GetBrokerAddrs(), "delay.message")
	if err != nil {
		return nil, err
	}
	return &DelayMsgRelaySubscriber{delayMessageRelaySub: sub}, nil
}

func (s *DelayMsgRelaySubscriber) SubscribeDelayMessage(ctx context.Context, handler func(context.Context, int64) error) error {
	topics := []string{string(biz.KafkaTopicOrderCancelledDelay)}
	consumer := s.delayMessageRelaySub
	consumerHandler := NewDelayConsumerHandler(consumer, handler)
	for {
		err := s.delayMessageRelaySub.Consume(ctx, topics, consumerHandler)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

type DelayConsumerHandler struct {
	handler  func(context.Context, int64) error
	consumer sarama.ConsumerGroup
}

func NewDelayConsumerHandler(consumer sarama.ConsumerGroup, handler func(context.Context, int64) error) sarama.ConsumerGroupHandler {
	h := &DelayConsumerHandler{
		handler:  handler,
		consumer: consumer,
	}
	return otelsarama.WrapConsumerGroupHandler(h)
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
			var orderCancelMsg biz.OrderCancelledMessage
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
			ctx := otel.GetTextMapPropagator().Extract(
				session.Context(),
				otelsarama.NewConsumerMessageCarrier(msg),
			)
			if err := c.handler(ctx, orderCancelMsg.OrderID); err != nil {
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
