package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"uuid"

	"github.com/azusayn/azushop/internal/pkg/db"
	"github.com/azusayn/azushop/internal/pkg/telemetry"
	"gorm.io/gorm"
)

type gormOutboxMessage struct {
	ID          uuid.UUID       `gorm:"column:id"`
	EventType   string          `gorm:"column:event_type"`
	Payload     json.RawMessage `gorm:"column:payload"`
	RetryCount  int             `gorm:"column:retry_count"`
	CreatedAt   time.Time       `gorm:"column:created_at"`
	Headers     json.RawMessage `gorm:"column:headers"`
	NextRetryAt time.Time       `gorm:"column:next_retry_at"`
	SentAt      *time.Time      `gorm:"column:sent_at"`
}

type store struct{}

func newStore() *store {
	return &store{}
}

func (s *store) CreateOutboxMessage(ctx context.Context, eventType string, payload json.RawMessage) error {
	traceBytes, err := telemetry.ExtractTraceHeaderBytes(ctx)
	if err != nil {
		return err
	}

	outboxMsg := &gormOutboxMessage{
		ID:        uuid.NewV7(),
		EventType: eventType,
		Payload:   payload,
		Headers:   traceBytes,
	}

	// TODO(4): metrics.
	ctx = telemetry.WithUnsampledSpanContext(ctx)

	return db.GetTransaction(ctx).WithContext(ctx).Create(outboxMsg).Error
}

// returns messages that are eligible for processing.
func (s *store) ListDueOutboxMessages(ctx context.Context, limit int) ([]*OutboxMessage, error) {
	// TODO(4): metrics.
	ctx = telemetry.WithUnsampledSpanContext(ctx)

	// TODO(4): composite index?
	var rows []*gormOutboxMessage
	if err := db.
		GetTransaction(ctx).
		WithContext(ctx).
		Where("sent_at IS NULL").
		Where("retry_count < ? AND next_retry_at < NOW()", outboxMaxRetryCount).
		Order("next_retry_at ASC").
		Limit(limit).
		Find(&rows).
		Error; err != nil {
		return nil, err
	}

	messages := make([]*OutboxMessage, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, convertToOutboxMessage(row))
	}
	return messages, nil
}

func (s *store) MarkOutboxMessagesSent(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	// TODO(4): metrics.
	ctx = telemetry.WithUnsampledSpanContext(ctx)

	if err := db.GetTransaction(ctx).
		WithContext(ctx).
		Model(&gormOutboxMessage{}).
		Where("id IN ?", ids).
		Update("sent_at", time.Now()).
		Error; err != nil {
		return fmt.Errorf("failed to mark outbox message (%v) sent: %w", ids, err)
	}

	return nil
}

func (s *store) MarkOutboxMessagesFailed(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}

	// TODO(4): metrics.
	ctx = telemetry.WithUnsampledSpanContext(ctx)

	if err := db.GetTransaction(ctx).
		WithContext(ctx).
		Model(&gormOutboxMessage{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"retry_count": gorm.Expr("retry_count + 1"),
			// exponential backoff.
			"next_retry_at": gorm.Expr("NOW() + make_interval(secs => POWER(2, retry_count))"),
		}).Error; err != nil {
		return fmt.Errorf("failed to mark outbox message (%v) failed: %w", ids, err)
	}

	return nil
}

func convertToOutboxMessage(msg *gormOutboxMessage) *OutboxMessage {
	return &OutboxMessage{
		ID:          msg.ID,
		EventType:   msg.EventType,
		Headers:     msg.Headers,
		Payload:     msg.Payload,
		RetryCount:  msg.RetryCount,
		CreatedAt:   msg.CreatedAt,
		NextRetryAt: msg.NextRetryAt,
		SentAt:      msg.SentAt,
	}
}
