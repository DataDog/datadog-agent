package backend

import (
	"context"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
)

type Client struct {
	// FIXME
}

func NewClient() Client {
	// FIXME: create an HTTP client to the backend
	return Client{}
}

func (c Client) SendEventsFrom(ctx context.Context, ready agenttypes.RawJSONEventsBatchChan, release func(batch agenttypes.RawJSONEventsBatchSlice)) {
	select {
	case <-ctx.Done():
		return
	case batch := <-ready:
		// FIXME: send batch
		release(batch)
	}
}
