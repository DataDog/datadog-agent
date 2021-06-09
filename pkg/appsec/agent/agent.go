package agent

import (
	"context"
	"errors"

	agenttypes "github.com/DataDog/datadog-agent/pkg/appsec/agent/types"
	"github.com/DataDog/datadog-agent/pkg/appsec/api/http"
	"github.com/DataDog/datadog-agent/pkg/appsec/backend"
	"github.com/DataDog/datadog-agent/pkg/appsec/batch"
	appsecconfig "github.com/DataDog/datadog-agent/pkg/appsec/config"
)

type Agent struct {
	// Agent API servers
	httpSrv http.Server

	// Event pipeline stage data
	eventsChan agenttypes.RawJSONEventsChan
	eventBatch *batch.EventBatch

	backendClient *backend.Client
}

var ErrAgentDisabled = errors.New(`AppSec agent disabled. Set the environment variable
DD_APPSEC_ENABLED=true or add "appsec_config.enabled: true" entry
to your datadog.yaml.`)

func NewAgent(cfg *appsecconfig.Config) (*Agent, error) {
	if !cfg.Enabled {
		return nil, ErrAgentDisabled
	}
	// TODO(julio): check the channel length with proper load testing
	eventsChan := make(agenttypes.RawJSONEventsChan, 1000)

	backendClient, err := backend.NewClient(cfg.BackendAPIKey, cfg.BackendAPIBaseURL, cfg.BackendAPIProxy)
	if err != nil {
		return nil, err
	}

	return &Agent{
		httpSrv:       http.NewServer(cfg, eventsChan),
		eventBatch:    batch.NewEventBatch(10),
		backendClient: backendClient,
	}, nil
}

func (a *Agent) Start(ctx context.Context) {
	// Start the event pipeline
	a.startEventPipeline(ctx)
	// Serve the agent API
	a.serve()
}

func (a *Agent) serve() {
	a.httpSrv.Start()
}

func (a *Agent) startEventPipeline(ctx context.Context) {
	// Send event batches to Datadog's backend
	go a.backendClient.EventService().SendBatchesFrom(ctx, a.eventBatch.Chan(), a.eventBatch.Put)
	// Batch the events sent to the agent API
	go a.eventBatch.AppendFrom(ctx, a.eventsChan)
}
