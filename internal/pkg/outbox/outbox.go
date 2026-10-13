package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"
	"uuid"

	"github.com/azusayn/azushop/internal/pkg/telemetry"
)

// TODO: make it configurable.
const (
	outboxBatchSize     int = 100
	outboxMaxRetryCount int = 5
	outboxPollInterval      = time.Second * 1
	maxGoroutines       int = 20
)

type OutboxMessage struct {
	ID          uuid.UUID
	EventType   string
	Headers     json.RawMessage
	Payload     json.RawMessage
	RetryCount  int
	CreatedAt   time.Time
	NextRetryAt time.Time
	SentAt      *time.Time
}

type OutboxStore interface {
	// order_outbox
	CreateOutboxMessage(ctx context.Context, eventType string, payload json.RawMessage) error
	ListDueOutboxMessages(ctx context.Context, limit int) ([]*OutboxMessage, error)
	MarkOutboxMessagesSent(ctx context.Context, ids []uuid.UUID) error
	MarkOutboxMessagesFailed(ctx context.Context, ids []uuid.UUID) error
}

// TODO(1): the outbox package should own the transaction.
type OutboxTransaction interface {
	Transaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type DispatchFunc func(ctx context.Context, message *OutboxMessage) error

type Outbox struct {
	store    OutboxStore
	dispatch DispatchFunc
	logger   *slog.Logger
	tx       OutboxTransaction
}

type Option func(*Outbox)

func NewOutbox(
	dispatch DispatchFunc,
	tx OutboxTransaction,
	opts ...Option,
) *Outbox {
	outbox := &Outbox{
		store:    newStore(),
		dispatch: dispatch,
		tx:       tx,
	}

	for _, opt := range opts {
		opt(outbox)
	}

	if outbox.logger == nil {
		outbox.logger = slog.Default()
	}

	return outbox
}

func (o *Outbox) Run(ctx context.Context) error {
	ticker := time.NewTicker(outboxPollInterval)

	for {
		select {
		case <-ticker.C:
			if err := o.tx.Transaction(ctx, func(ctx context.Context) error {
				return o.processOutboxMessages(ctx)
			}); err != nil {
				slog.Warn("failed to process outbox messages", slog.Any("err", err))
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (o *Outbox) processOutboxMessages(ctx context.Context) error {
	messages, err := o.store.ListDueOutboxMessages(ctx, outboxBatchSize)
	if err != nil {
		return err
	}

	sentIDs := make([]uuid.UUID, 0)
	failedIDs := make([]uuid.UUID, 0)
	var mu sync.Mutex

	markID := func(id uuid.UUID, success bool) {
		mu.Lock()
		defer mu.Unlock()
		if success {
			sentIDs = append(sentIDs, id)
			return
		}
		failedIDs = append(failedIDs, id)
	}

	var wg sync.WaitGroup
	limitCh := make(chan struct{}, maxGoroutines)

	for _, message := range messages {
		limitCh <- struct{}{}
		wg.Go(func() {
			defer func() { <-limitCh }()

			msgCtx, err := telemetry.InjectTraceHeaderBytes(ctx, message.Headers)
			if err != nil {
				slog.ErrorContext(ctx, "failed to inject headers into context", slog.Any("message_id", message.ID), slog.Any("err", err))
				markID(message.ID, false)
				return
			}

			if err := o.dispatch(msgCtx, message); err != nil {
				slog.ErrorContext(msgCtx, "failed to send outbox message", slog.Any("msg", message), slog.Any("err", err))
				markID(message.ID, false)
				return
			}

			markID(message.ID, true)
		})
	}

	wg.Wait()

	return errors.Join(
		o.store.MarkOutboxMessagesFailed(ctx, failedIDs),
		o.store.MarkOutboxMessagesSent(ctx, sentIDs),
	)
}

func (o *Outbox) Save(ctx context.Context, eventType string, payload json.RawMessage) error {
	return o.store.CreateOutboxMessage(ctx, eventType, payload)
}
