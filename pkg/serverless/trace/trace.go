// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	remotecfg "github.com/DataDog/datadog-agent/cmd/trace-agent/config/remote"
	compcorecfg "github.com/DataDog/datadog-agent/comp/core/config"
	authtokennoneimpl "github.com/DataDog/datadog-agent/comp/core/ipc/impl-none"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	comptracecfg "github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	serverlessmodifier "github.com/DataDog/datadog-agent/pkg/serverless/trace/modifier"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/stats"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/timing"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerlessTraceAgent represents a trace agent in a serverless context
type ServerlessTraceAgent interface {
	Stop()
	Flush()
	Process(p *api.Payload)
	SetTags(map[string]string)
	SetTargetTPS(float64)
	SetSpanModifier(agent.SpanModifier)
	GetSpanModifier() agent.SpanModifier
}

// Load abstracts the file configuration loading
type Load interface {
	Load() (*config.AgentConfig, error)
}

// LoadConfig is implementing Load to retrieve the config
type LoadConfig struct {
	Path   string
	Tagger tagger.Component
}

// httpURLMetaKey is the key of the span meta containing the HTTP URL
const httpURLMetaKey = "http.url"

// tcpRemoteHostMetaKey is the key of the span meta containing the TCP remote host i.e 127.0.0.1
const tcpRemoteHostMetaKey = "tcp.remote.host"

// tcpRemotePortMetaKey is the the key of the span meta containing the TCP remote port e.g 8124
const tcpRemotePortMetaKey = "tcp.remote.port"

// dnsAddressMetaKey is the key of the span meta containing the DNS address
const dnsAddressMetaKey = "dns.address"

// disableTraceStatsEnvVar is the environment variable to disable trace stats computation in serverless-init
const disableTraceStatsEnvVar = "DD_SERVERLESS_INIT_DISABLE_TRACE_STATS"

// agentURLPrefix is the first part of a URL for internal agent communication (port 8124)
const agentURLPrefix = "http://127.0.0.1:8124"

// statsDURLPrefix is the first part of a URL for a call from Statsd
const statsDURLPrefix = "http://127.0.0.1:8125"

// dnsNonRoutableAddressURLPrefix is the first part of a URL from the non-routable address for DNS traces
const dnsNonRoutableAddressURLPrefix = "0.0.0.0"

// dnsLocalHostAddressURLPrefix is the first part of a URL from the localhost address for DNS traces
const dnsLocalHostAddressURLPrefix = "127.0.0.1"

// Load loads the config from a file path
func (l *LoadConfig) Load() (*config.AgentConfig, error) {
	c, err := compcorecfg.NewServerlessConfig(l.Path)
	if err != nil {
		return nil, err
	}

	return comptracecfg.LoadConfigFile(l.Path, c, l.Tagger, authtokennoneimpl.NewNoopIPC().Comp)
}

// StartServerlessTraceAgentArgs are the arguments for the StartServerlessTraceAgent method
type StartServerlessTraceAgentArgs struct {
	Enabled               bool
	LoadConfig            Load
	AdditionalProfileTags map[string]string
	FunctionTags          string
	RCService             *remoteconfig.CoreAgentService
}

// Start starts the agent
//
//nolint:revive // TODO(SERV) Fix revive linter
func StartServerlessTraceAgent(args StartServerlessTraceAgentArgs) ServerlessTraceAgent {
	if args.Enabled {
		// Set the serverless config option which will be used to determine if
		// hostname should be resolved. Skipping hostname resolution saves >1s
		// in load time between gRPC calls and agent commands.
		pkgconfigsetup.Datadog().Set("serverless.enabled", true, model.SourceAgentRuntime)

		// Turn off apm_sampling as it's not supported and generates debug logs
		pkgconfigsetup.Datadog().Set("remote_configuration.apm_sampling.enabled", false, model.SourceAgentRuntime)

		tc, confErr := args.LoadConfig.Load()
		if confErr != nil {
			log.Errorf("Unable to load trace agent config: %s", confErr)
		} else {
			context, cancel := context.WithCancel(context.Background())
			tc.Hostname = ""
			tc.SynchronousFlushing = true
			tc.AdditionalProfileTags = args.AdditionalProfileTags
			ta := agent.NewAgent(context, tc, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, zstd.NewComponent())

			// Check if trace stats should be disabled for serverless
			if disabled, _ := strconv.ParseBool(os.Getenv(disableTraceStatsEnvVar)); disabled {
				log.Debug("Trace stats computation disabled for serverless via DD_SERVERLESS_INIT_DISABLE_TRACE_STATS")
				ta.Concentrator = &noopConcentrator{}
			}

			ta.SpanModifier = &spanModifier{
				ddOrigin: getDDOrigin(),
			}
			ta.TracerPayloadModifier = serverlessmodifier.NewTracerPayloadModifier(args.FunctionTags)

			ta.DiscardSpan = filterSpan
			startTraceAgentConfigEndpoint(args.RCService, tc)
			go ta.Run()
			return &serverlessTraceAgent{
				ta:     ta,
				cancel: cancel,
			}
		}
	} else {
		log.Info("Trace agent is disabled")
	}
	return noopTraceAgent{}
}

func startTraceAgentConfigEndpoint(rcService *remoteconfig.CoreAgentService, tc *config.AgentConfig) {
	if configUtils.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) && rcService != nil {
		statsdNoopClient := &statsd.NoOpClient{}
		api.AttachEndpoint(api.Endpoint{
			Pattern: "/v0.7/config",
			Handler: func(r *api.HTTPReceiver) http.Handler {
				return remotecfg.ConfigHandler(r, rcService, tc, statsdNoopClient, timing.New(statsdNoopClient))
			},
		})
	} else {
		log.Debug("Not starting trace agent config endpoint")
	}
}

type serverlessTraceAgent struct {
	ta     *agent.Agent
	cancel context.CancelFunc
}

// Flush performs a synchronous flushing in the trace agent
func (t *serverlessTraceAgent) Flush() {
	t.ta.FlushSync()
}

// Process processes a payload in the trace agent.
func (t *serverlessTraceAgent) Process(p *api.Payload) {
	t.ta.Process(p)
}

type taggable interface {
	SetTags(tags map[string]string)
}

// SetTags sets the tags to the trace agent config and span processor
func (t *serverlessTraceAgent) SetTags(tags map[string]string) {
	t.ta.SetGlobalTagsUnsafe(tags)
	if tagger, ok := t.ta.SpanModifier.(taggable); ok {
		tagger.SetTags(tags)
	}
}

// Stop stops the trace agent
func (t *serverlessTraceAgent) Stop() {
	t.cancel()
}

// SetTargetTPS sets the target TPS to the trace agent.
func (t *serverlessTraceAgent) SetTargetTPS(tps float64) {
	t.ta.PrioritySampler.UpdateTargetTPS(tps)
}

// SetSpanModifier sets the span modifier to the trace agent.
func (t *serverlessTraceAgent) SetSpanModifier(sm agent.SpanModifier) {
	t.ta.SpanModifier = sm
}

// GetSpanModifier returns the span modifier from the trace agent.
func (t *serverlessTraceAgent) GetSpanModifier() agent.SpanModifier {
	return t.ta.SpanModifier
}

// filterSpan returns true if a span was generated by internal infrastructure calls
// that should not be visible to customers (e.g., StatsD, internal DNS lookups, serverless agent communication)
func filterSpan(span *pb.Span) bool {
	// Filters out HTTP calls to internal infrastructure
	if httpURL, ok := span.Meta[httpURLMetaKey]; ok {
		if strings.HasPrefix(httpURL, agentURLPrefix) {
			log.Debugf("Detected span with http url %s, removing it", httpURL)
			return true
		}

		if strings.HasPrefix(httpURL, statsDURLPrefix) {
			log.Debugf("Detected span with http url %s, removing it", httpURL)
			return true
		}
	}

	// Filters out TCP spans to internal infrastructure
	if tcpHost, ok := span.Meta[tcpRemoteHostMetaKey]; ok {
		if tcpPort, ok := span.Meta[tcpRemotePortMetaKey]; ok {
			tcpURLPrefix := "http://" + tcpHost + ":" + tcpPort
			if strings.HasPrefix(tcpURLPrefix, agentURLPrefix) {
				log.Debugf("Detected span with tcp url %s, removing it", tcpURLPrefix)
				return true
			}

			if strings.HasPrefix(tcpURLPrefix, statsDURLPrefix) {
				log.Debugf("Detected span with tcp url %s, removing it", tcpURLPrefix)
				return true
			}
		}
	}

	// Filters out DNS spans to localhost and internal addresses
	if dnsAddress, ok := span.Meta[dnsAddressMetaKey]; ok {
		if strings.HasPrefix(dnsAddress, dnsNonRoutableAddressURLPrefix) ||
			strings.HasPrefix(dnsAddress, dnsLocalHostAddressURLPrefix) {
			log.Debugf("Detected span with dns url %s, removing it", dnsAddress)
			return true
		}
	}

	return false
}

// getDDOrigin returns the value for the _dd.origin tag based on the cloud service type
func getDDOrigin() string {
	return cloudservice.GetCloudServiceType().GetOrigin()
}

// noopConcentrator is a no-op implementation of agent.Concentrator interface
type noopConcentrator struct{}

func (c noopConcentrator) Start()              {}
func (c noopConcentrator) Stop()               {}
func (c noopConcentrator) Add(stats.Input)     {}
func (c noopConcentrator) AddV1(stats.InputV1) {}

type noopTraceAgent struct{}

func (t noopTraceAgent) Stop()                               {}
func (t noopTraceAgent) Flush()                              {}
func (t noopTraceAgent) Process(*api.Payload)                {}
func (t noopTraceAgent) SetTags(map[string]string)           {}
func (t noopTraceAgent) SetTargetTPS(float64)                {}
func (t noopTraceAgent) SetSpanModifier(agent.SpanModifier)  {}
func (t noopTraceAgent) GetSpanModifier() agent.SpanModifier { return nil }
