package biz

import (
	"context"
)

type DelayMsgRelaySubscriber interface {
	SubscribeDelayMessage(ctx context.Context, handler func(orderID int64) error) error
}

type DelayMsgRelayPublisher interface {
	PublishOrderCancelled(ctx context.Context, orderID int64) error
}

type DelayMsgRealyUsecase struct {
	subscriber DelayMsgRelaySubscriber
	publisher  DelayMsgRelayPublisher
}

func NewDelayMsgRealyUsecase(
	subscriber DelayMsgRelaySubscriber,
	publisher DelayMsgRelayPublisher,
) *DelayMsgRealyUsecase {
	return &DelayMsgRealyUsecase{
		subscriber: subscriber,
		publisher:  publisher,
	}
}

func (uc *DelayMsgRealyUsecase) HandleDelayMessage(ctx context.Context) error {
	return uc.subscriber.SubscribeDelayMessage(ctx, func(orderID int64) error {
		return uc.publisher.PublishOrderCancelled(ctx, orderID)
	})
}
