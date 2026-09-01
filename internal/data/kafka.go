package data

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/IBM/sarama"
	"github.com/azusayn/azushop/proto/conf"
	"github.com/dnwe/otelsarama"
	"github.com/pkg/errors"
)

type PaymentStatus string

const (
	PaymentStatusUnspecified PaymentStatus = "unspecified"
	PaymentStatusPending     PaymentStatus = "pending"
	PaymentStatusCancelled   PaymentStatus = "cancelled"
	PaymentStatusPaid        PaymentStatus = "paid"
	PaymentStatusRefunding   PaymentStatus = "refunding"
	PaymentStatusRefunded    PaymentStatus = "refunded"
)

type KafkaProducer struct {
	syncProducer sarama.SyncProducer
}

// TODO(3): async producer.
func NewKafkaProducer(config *conf.Data) (*KafkaProducer, error) {
	brokerAddrs := config.GetKafka().GetBrokerAddrs()
	if len(brokerAddrs) == 0 {
		panic("broker address list is empty")
	}
	syncProducer, err := NewSyncProducer(brokerAddrs)
	if err != nil {
		return nil, err
	}
	return &KafkaProducer{syncProducer: syncProducer}, nil
}

func NewSyncProducer(brokerAddrs []string) (sarama.SyncProducer, error) {
	kafkaConfig := sarama.NewConfig()
	kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
	kafkaConfig.Producer.Return.Successes = true

	producer, err := sarama.NewSyncProducer(brokerAddrs, kafkaConfig)
	if err != nil {
		return nil, err
	}

	return otelsarama.WrapSyncProducer(kafkaConfig, producer), nil
}

func NewConsumerGroup(brokerAddrs []string, groupID string) (sarama.ConsumerGroup, error) {
	consumerConfig := sarama.NewConfig()
	// consumes messages at least once, make sure all the APIs are idempotent.
	consumerConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	consumerConfig.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{
		sarama.NewBalanceStrategyRoundRobin(),
	}
	consumerGroup, err := sarama.NewConsumerGroup(brokerAddrs, groupID, consumerConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create consumer group %q", groupID)
	}
	return consumerGroup, nil
}

type ConsumerHandler struct {
	handlers map[string]func(context.Context, []byte) error
}

func NewConsumerHandler(handlers map[string]func(context.Context, []byte) error) sarama.ConsumerGroupHandler {
	h := &ConsumerHandler{handlers: handlers}
	return otelsarama.WrapConsumerGroupHandler(h)
}

func (c *ConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *ConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

func (c *ConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case <-session.Context().Done():
			return nil
		case msg, ok := <-claim.Messages():
			if !ok || msg == nil {
				return nil
			}
			handler, found := c.handlers[claim.Topic()]
			if !found {
				return fmt.Errorf("handler for topic %q not found", claim.Topic())
			}
			if err := handler(context.TODO(), msg.Value); err != nil {
				slog.Warn(err.Error())
			}
			session.MarkMessage(msg, "")
		}
	}
}
