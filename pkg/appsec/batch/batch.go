package batch

import (
	"context"
	"sync"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/api/http/v0_1_0/types"
)

type (
	EventBatch struct {
		// Current batch
		// TODO(julio): quick first implementation - move to a lock-free queue asap
		batch agenttypes.RawJSONEventsBatchSlice
		// Mutex protecting unsafe batch operations
		mu sync.Mutex

		// Pool of batches
		pool      sync.Pool
		readyChan agenttypes.RawJSONEventsBatchChan
	}
)

func NewEventBatch(maxBatchLength int) *EventBatch {
	return &EventBatch{
		batch: make(agenttypes.RawJSONEventsBatchSlice, 0, maxBatchLength),
		pool: sync.Pool{
			New: func() interface{} {
				return make(agenttypes.RawJSONEventsBatchSlice, 0, maxBatchLength)
			},
		},
	}
}

func (b *EventBatch) AppendFrom(ctx context.Context, eventsChan agenttypes.RawJSONEventsChan) {
	for {
		select {
		case <-ctx.Done():
			return
		case events, closed := <-eventsChan:
			if closed {
				return
			}
			b.Append(events)
		}
	}
}

func (b *EventBatch) Append(events interface{}) {
	switch actual := events.(type) {
	case types.RawJSONEventSlice:
		b.append(actual)
	default:
		// TODO(julio): log an error
	}
}

func (b *EventBatch) append(events types.RawJSONEventSlice) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.batch) < cap(b.batch) {
		// The batch is not full yet
		b.batch = append(b.batch)
		return
	}

	// The batch is full and is ready to be sent
	ready := b.batch
	// Try adding it to the channel without blocking.
	// If not possible, create an extra goroutine to do it and possibly block
	// until room is available in the channel.
	select {
	case b.readyChan <- ready:
	default:
		// The channel is full
		go func() {
			b.readyChan <- ready
		}()
	}

	// Replace the current batch
	b.newBatch()
}

// newBatch recreates a batch slice out of the batch pool
func (b *EventBatch) newBatch() {
	b.batch = b.pool.New().(agenttypes.RawJSONEventsBatchSlice)
}

// Put the batch back into the allocation pool.
func (b *EventBatch) Put(batch agenttypes.RawJSONEventsBatchSlice) {
	// Reset the length to 0
	batch = batch[0:0]
	b.pool.Put(batch)
}

// Chan returns the channel of batches ready to be sent to the backend.
func (b *EventBatch) Chan() agenttypes.RawJSONEventsBatchChan {
	return b.readyChan
}
