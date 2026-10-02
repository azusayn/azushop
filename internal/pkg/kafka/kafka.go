package kafka

import (
	"context"
)

type TopicType string

type Message struct {
	Topic TopicType
	Value any
}

func (t TopicType) WithScope(scope string) TopicType {
	return TopicType(scope + "." + string(t))
}

type Publisher interface {
	SendMessages(ctx context.Context, messages []*Message) error
}

type HandlerFunc func(context.Context, []byte) error

type Subscriber interface {
	// Subscribe handles messages for given topics synchronously.
	Subscribe(ctx context.Context, handlers map[TopicType]HandlerFunc) error
}

type ConsumerHandler struct {
	handlers map[TopicType]HandlerFunc
}
