package data

import (
	"azushop/internal/conf"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type PaymentStatus string

const (
	PaymentStatusUnspcified PaymentStatus = "unspecified"
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusCancelled  PaymentStatus = "cancelled"
	PaymentStatusPaid       PaymentStatus = "paid"
	PaymentStatusRefunding  PaymentStatus = "refunding"
	PaymentStatusRefunded   PaymentStatus = "refunded"
)

type PaymentStatusMessage struct {
	OrderID int64
	Status  PaymentStatus
}

type OrderItem struct {
	SkuID    uuid.UUID
	Quantity int64
}

type OrderCreatedMessage struct {
	OrderID    int64
	OrderItems []*OrderItem
}

type OrderCancelledMessage struct {
	OrderID     int64
	ExpiredTime time.Time
}

type KafkaProducer struct {
	syncProducer sarama.SyncProducer
}

// TODO: async producer.
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
	return sarama.NewSyncProducer(brokerAddrs, kafkaConfig)
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
	handler func([]byte) error
}

func NewConsumerHandler(handler func([]byte) error) sarama.ConsumerGroupHandler {
	return &ConsumerHandler{handler: handler}
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
			if !ok {
				return nil
			}
			if err := c.handler(msg.Value); err != nil {
				slog.Warn(err.Error())
			}
			session.MarkMessage(msg, "")
		}
	}
}
