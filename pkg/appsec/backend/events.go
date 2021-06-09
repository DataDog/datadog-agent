package backend

import (
	"context"
	"errors"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/backend/api"
)

type EventService Client

func (s *EventService) unwrap() *Client { return (*Client)(s) }

func (s *EventService) SendBatch(ctx context.Context, b api.Batch) error {
	if len(b) == 0 {
		return errors.New("unexpected empty batch")
	}
	c := s.unwrap()
	r, err := c.newRequest("POST", "batches", b)
	if err != nil {
		return err
	}
	return c.do(ctx, r, nil)
}

func (s *EventService) SendBatchesFrom(ctx context.Context, ready agenttypes.RawJSONEventsBatchChan, release func(batch agenttypes.RawJSONEventsBatchSlice)) {
	select {
	case <-ctx.Done():
		return
	case batch := <-ready:
		// Perform this long-running IO in a separate goroutine to keep going
		go func() {
			_ = s.SendBatch(ctx, api.Batch(batch))
			// TODO: handle the previous error (log, metrics...)
		}()
		release(batch)
	}
}
