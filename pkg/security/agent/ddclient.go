package agent

import (
	"context"
	"sync"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DDClient represents a Datadog Log client
type DDClient struct {
	destinationsCtx  *client.DestinationsContext
	auditor          *auditor.Auditor
	pipelineProvider pipeline.Provider
	logSource        *config.LogSource
	msgChan          chan *message.Message
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewDDClientWithLogSource instantiates a new Datadog log client with a Log Source configuration
func NewDDClientWithLogSource(src *config.LogSource) *DDClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &DDClient{
		logSource: src,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Run starts the Datadog log client. It follows the API described in pkg/logs/README.md.
func (ddc *DDClient) Run(wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	// Get Datadog endpoints
	endpoints, err := config.BuildHTTPEndpoints()
	if err != nil {
		log.Errorf("datadog logs client stopped with an error: %v", err)
		return
	}

	httpConnectivity := http.CheckConnectivity(endpoints.Main)
	endpoints, err = config.BuildEndpoints(httpConnectivity)
	if err != nil {
		log.Errorf("datadog logs client stopped with an error: %v", err)
		return
	}

	// Creates a log destination context that will be used by all the senders
	ddc.destinationsCtx = client.NewDestinationsContext()
	ddc.destinationsCtx.Start()
	defer ddc.destinationsCtx.Stop()

	// Sets up the auditor
	ddc.auditor = auditor.New(
		coreconfig.Datadog.GetString("security_agent_config.run_path"),
		health.RegisterLiveness("runtime-security-agent"))
	ddc.auditor.Start()
	defer ddc.auditor.Stop()

	// setup the pipeline provider that provides pairs of processor and sender
	ddc.pipelineProvider = pipeline.NewProvider(
		config.NumberOfPipelines,
		ddc.auditor,
		nil,
		endpoints,
		ddc.destinationsCtx)
	ddc.pipelineProvider.Start()
	ddc.msgChan = ddc.pipelineProvider.NextPipelineChan()
	defer ddc.pipelineProvider.Stop()
	// Wait until context is cancelled
	<-ddc.ctx.Done()
}

// Stop the Datadog logs client
func (ddc *DDClient) Stop() {
	ddc.cancel()
}

// SendLogWithStatusAndTags sends a new log to Datadog with the provided log status and tags
func (ddc *DDClient) SendLogWithStatusAndTags(buf []byte, status string, tags []string) {
	src := config.NewLogSource(ddc.logSource.Config.Source, &config.LogsConfig{
		Type:    ddc.logSource.Config.Type,
		Service: ddc.logSource.Config.Service,
		Source:  ddc.logSource.Config.Source,
		Tags:    tags,
	})
	msg := message.NewMessageWithSource(buf, status, src)
	select {
	case ddc.msgChan <- msg:
		break
	default:
		log.Warn("log client not ready, ignoring message")
		break
	}
}

// SendSecurityEvent sends a security event with the provided status
func (ddc *DDClient) SendSecurityEvent(evt *api.SecurityEventMessage, status string) {
	ddc.SendLogWithStatusAndTags(evt.GetData(), status, evt.GetTags())
}
