// TODO: move to internal/pkg/kafka package
package data

import (
	"github.com/IBM/sarama"
	"github.com/azusayn/azushop/internal/pkg/kafka"
	"github.com/azusayn/azushop/proto/conf"
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
	syncProducer, err := kafka.NewSyncProducer(brokerAddrs)
	if err != nil {
		return nil, err
	}
	return &KafkaProducer{syncProducer: syncProducer}, nil
}
