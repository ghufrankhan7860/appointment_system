package services

import (
	"context"
	"fmt"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

// OutboxService hands written outbox events to a publisher. Delivery is at
// least once: an event whose publish succeeded but whose batch failed to commit
// is published again, so publishers must tolerate repeats.
type OutboxService struct {
	outbox OutboxRepository
	tx     TransactionManager
	logger Logger
}

func NewOutboxService(
	outbox OutboxRepository,
	tx TransactionManager,
	logger Logger,
) (*OutboxService, error) {
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &OutboxService{
		outbox: outbox,
		tx:     tx,
		logger: logger,
	}, nil
}

// Publish sends up to limit unpublished events in order and marks the sent
// ones published. Each send gets constant.OutboxSendTimeout, so the publisher
// must honour its context. It stops at the first failed or timed-out publish;
// the events before it stay published and the rest are retried on the next
// call.
func (obs *OutboxService) Publish(
	ctx context.Context,
	limit int,
	publish func(context.Context, *models.OutboxEvent) error,
) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("publish outbox context: %w", err)
	}
	if publish == nil {
		return 0, fmt.Errorf("publish outbox without publisher")
	}
	limit, _ = clampPagination(limit, 0)

	published := 0
	var publishErr error
	err := obs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		events, err := obs.outbox.ListUnpublished(txCtx, limit)
		if err != nil {
			return err
		}

		ids := make([]uint64, 0, len(events))
		for _, event := range events {
			sendCtx, cancel := context.WithTimeout(txCtx, constant.OutboxSendTimeout)
			publishErr = publish(sendCtx, event)
			cancel()
			if publishErr != nil {
				break
			}
			ids = append(ids, event.ID)
		}
		published = len(ids)

		return obs.outbox.MarkPublished(txCtx, ids)
	})
	if err != nil {
		return 0, fmt.Errorf("publish outbox: %w", err)
	}
	if publishErr != nil {
		return published, fmt.Errorf("publish outbox event: %w", publishErr)
	}
	if published > 0 {
		obs.logger.InfoContext(ctx, "outbox events published", "count", published)
	}

	return published, nil
}
