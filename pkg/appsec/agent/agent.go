package agent

import (
	"context"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/api/http"
	"github.com/DataDog/datadog-agent/pkg/appsec/backend"
	"github.com/DataDog/datadog-agent/pkg/appsec/batch"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

type Agent struct {
	// Agent API servers
	httpSrv http.Server

	// Event pipeline stage data
	eventsChan agenttypes.RawJSONEventsChan
	eventBatch *batch.EventBatch

	backendClient backend.Client
}

func NewAgent(conf *config.AgentConfig) *Agent {
	// TODO(julio): check the channel length with proper load testing
	eventsChan := make(agenttypes.RawJSONEventsChan, 1000)

	return &Agent{
		httpSrv:    http.NewServer(conf, eventsChan),
		eventBatch: batch.NewEventBatch(10),
	}
}

func (a *Agent) Start(ctx context.Context) {
	a.startEventPipeline(ctx)

	// Serve the agent API
	a.serve()

	<-ctx.Done()
}

func (a *Agent) serve() {
	a.httpSrv.Start()
}

func (a *Agent) startEventPipeline(ctx context.Context) {
	// Send event batches to Datadog's backend
	go a.backendClient.SendEventsFrom(ctx, a.eventBatch.Chan(), a.eventBatch.Put)
	// Batch the events sent to the agent API
	go a.eventBatch.AppendFrom(ctx, a.eventsChan)
}
