package retry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/azusayn/azushop/internal/pkg/kafka"
)

const (
	kafkaTopicRetryQueue      kafka.TopicType = "retry_queue"
	kafkaTopicDeadLetterQueue kafka.TopicType = "dead_letter_queue"
)

type Message struct {
	EventType  string
	retryCount int
	Message    any
}

type Retrier struct {
	tasks         sync.Map
	publisher     kafka.Publisher
	subscriber    kafka.Subscriber
	logger        *slog.Logger
	retryInterval time.Duration
	maxRetryCount int
	scope         string
}

type Option func(*Retrier)

func WithLogger(logger *slog.Logger) Option {
	return func(r *Retrier) {
		if logger != nil {
			r.logger = logger
		}
	}
}

func WithRetryInterval(d time.Duration) Option {
	return func(r *Retrier) {
		if d > 0 {
			r.retryInterval = d
		}
	}
}

func WithMaxRetryCount(n int) Option {
	return func(r *Retrier) {
		if n > 0 {
			r.maxRetryCount = n
		}
	}
}

func NewRetrier(
	scope string,
	publisher kafka.Publisher,
	subscriber kafka.Subscriber,
	opts ...Option,
) *Retrier {
	r := &Retrier{
		publisher:     publisher,
		subscriber:    subscriber,
		logger:        slog.Default(),
		retryInterval: 50 * time.Millisecond,
		maxRetryCount: 3,
		scope:         scope,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Run consumes the retry queue until ctx is done.
func (r *Retrier) Run(ctx context.Context) error {
	handler := func(ctx context.Context, b []byte) error {
		var retryMessage *Message
		if err := json.Unmarshal(b, &retryMessage); err != nil {
			return err
		}

		var handler func(context.Context) error
		if v, ok := r.tasks.Load(retryMessage.EventType); !ok {
			return fmt.Errorf("handler not found for event type %q", retryMessage.EventType)
		} else {
			if handler, ok = v.(func(context.Context) error); !ok {
				return fmt.Errorf("invalid handler type for event type %q", retryMessage.EventType)
			}
		}

		// TODO: internal retry logic here
		if err := r.once(ctx, retryMessage, handler); err != nil {
			return err
		}
		return nil
	}

	return r.subscriber.Subscribe(ctx, map[kafka.TopicType]kafka.HandlerFunc{
		kafkaTopicRetryQueue.WithScope(r.scope): handler,
	})
}

func (r *Retrier) Submit(
	ctx context.Context,
	msg *Message,
	fn func(context.Context) error,
) error {
	r.tasks.Store(msg.EventType, fn)
	return r.once(ctx, msg, fn)
}

// once calls fn and performs short-interval a retry for transient failures if fn fails.
func (r *Retrier) once(
	ctx context.Context,
	msg *Message,
	fn func(context.Context) error,
) error {
	if err := fn(ctx); err == nil {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(r.retryInterval):
	}

	if msg.retryCount > r.maxRetryCount {
		r.logger.ErrorContext(ctx, fmt.Sprintf("max retries (%d) exceeded", r.maxRetryCount))
		if err := r.publisher.SendMessages(ctx, []*kafka.Message{{
			Topic: kafkaTopicDeadLetterQueue.WithScope(r.scope),
			Value: msg,
		}}); err != nil {
			r.logger.ErrorContext(ctx, "failed to send message to dead letter queue", slog.Any("msg", msg))
		}
		return nil
	}

	msg.retryCount++

	// TODO: support memory queue for retry messages
	return r.publisher.SendMessages(ctx, []*kafka.Message{{
		Topic: kafkaTopicRetryQueue.WithScope(r.scope),
		Value: msg,
	}})
}
