// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	compcorecfg "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	zstd "github.com/DataDog/datadog-agent/comp/trace/compression/impl-zstd"
	comptracecfg "github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
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

// lambdaRuntimeUrlPrefix is the first part of a URL for a call to the Lambda runtime API. The value may be replaced if `AWS_LAMBDA_RUNTIME_API` is set.
var lambdaRuntimeURLPrefix = "http://127.0.0.1:9001"

// lambdaExtensionURLPrefix is the first part of a URL for a call from the Datadog Lambda Library to the Lambda Extension
const lambdaExtensionURLPrefix = "http://127.0.0.1:8124"

// lambdaStatsDURLPrefix is the first part of a URL for a call from Statsd
const lambdaStatsDURLPrefix = "http://127.0.0.1:8125"

// dnsNonRoutableAddressURLPrefix is the first part of a URL from the non-routable address for DNS traces
const dnsNonRoutableAddressURLPrefix = "0.0.0.0"

// dnsLocalHostAddressURLPrefix is the first part of a URL from the localhost address for DNS traces
const dnsLocalHostAddressURLPrefix = "127.0.0.1"

// awsXrayDaemonAddressURLPrefix is the first part of a URL from the _AWS_XRAY_DAEMON_ADDRESS for DNS traces
const awsXrayDaemonAddressURLPrefix = "169.254.79.129"

const invocationSpanResource = "dd-tracer-serverless-span"

// Load loads the config from a file path
func (l *LoadConfig) Load() (*config.AgentConfig, error) {
	c, err := compcorecfg.NewServerlessConfig(l.Path)
	if err != nil {
		return nil, err
	}
	return comptracecfg.LoadConfigFile(l.Path, c, l.Tagger)
}

// StartServerlessTraceAgentArgs are the arguments for the StartServerlessTraceAgent method
type StartServerlessTraceAgentArgs struct {
	Enabled               bool
	LoadConfig            Load
	LambdaSpanChan        chan<- *pb.Span
	ColdStartSpanID       uint64
	AzureContainerAppTags string
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

		tc, confErr := args.LoadConfig.Load()
		if confErr != nil {
			log.Errorf("Unable to load trace agent config: %s", confErr)
		} else {
			context, cancel := context.WithCancel(context.Background())
			tc.Hostname = ""
			tc.SynchronousFlushing = true
			tc.AzureContainerAppTags = args.AzureContainerAppTags
			ta := agent.NewAgent(context, tc, telemetry.NewNoopCollector(), &statsd.NoOpClient{}, zstd.NewComponent())
			ta.SpanModifier = &spanModifier{
				coldStartSpanId: args.ColdStartSpanID,
				lambdaSpanChan:  args.LambdaSpanChan,
				ddOrigin:        getDDOrigin(),
			}

			ta.DiscardSpan = filterSpanFromLambdaLibraryOrRuntime
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

// filterSpanFromLambdaLibraryOrRuntime returns true if a span was generated by internal HTTP calls within the Datadog
// Lambda Library or the Lambda runtime
func filterSpanFromLambdaLibraryOrRuntime(span *pb.Span) bool {
	// Filters out HTTP calls
	if httpURL, ok := span.Meta[httpURLMetaKey]; ok {
		if strings.HasPrefix(httpURL, lambdaExtensionURLPrefix) {
			log.Debugf("Detected span with http url %s, removing it", httpURL)
			return true
		}

		if strings.HasPrefix(httpURL, lambdaStatsDURLPrefix) {
			log.Debugf("Detected span with http url %s, removing it", httpURL)
			return true
		}

		if strings.HasPrefix(httpURL, lambdaRuntimeURLPrefix) {
			log.Debugf("Detected span with http url %s, removing it", httpURL)
			return true
		}
	}

	// Filers out TCP spans
	if tcpHost, ok := span.Meta[tcpRemoteHostMetaKey]; ok {
		if tcpPort, ok := span.Meta[tcpRemotePortMetaKey]; ok {
			tcpLambdaURLPrefix := fmt.Sprint("http://" + tcpHost + ":" + tcpPort)
			if strings.HasPrefix(tcpLambdaURLPrefix, lambdaExtensionURLPrefix) {
				log.Debugf("Detected span with tcp url %s, removing it", tcpLambdaURLPrefix)
				return true
			}

			if strings.HasPrefix(tcpLambdaURLPrefix, lambdaStatsDURLPrefix) {
				log.Debugf("Detected span with tcp url %s, removing it", tcpLambdaURLPrefix)
				return true
			}

			if strings.HasPrefix(tcpLambdaURLPrefix, lambdaRuntimeURLPrefix) {
				log.Debugf("Detected span with tcp url %s, removing it", tcpLambdaURLPrefix)
				return true
			}
		}
	}

	// Filters out DNS spans
	if dnsAddress, ok := span.Meta[dnsAddressMetaKey]; ok {
		if strings.HasPrefix(dnsAddress, dnsNonRoutableAddressURLPrefix) ||
			strings.HasPrefix(dnsAddress, dnsLocalHostAddressURLPrefix) ||
			strings.HasPrefix(dnsAddress, awsXrayDaemonAddressURLPrefix) {
			log.Debugf("Detected span with dns url %s, removing it", dnsAddress)
			return true
		}
	}

	// Filter out the serverless span from the tracer
	if span != nil && span.Resource == invocationSpanResource {
		log.Debugf("Detected invocation span from tracer, removing it")
		return true
	}
	return false
}

// getDDOrigin returns the value for the _dd.origin tag based on the cloud service type
func getDDOrigin() string {
	origin := ddOriginTagValue
	if cloudServiceOrigin := cloudservice.GetCloudServiceType().GetOrigin(); cloudServiceOrigin != "local" {
		origin = cloudServiceOrigin
	}
	return origin
}

type noopTraceAgent struct{}

func (t noopTraceAgent) Stop()                               {}
func (t noopTraceAgent) Flush()                              {}
func (t noopTraceAgent) Process(*api.Payload)                {}
func (t noopTraceAgent) SetTags(map[string]string)           {}
func (t noopTraceAgent) SetTargetTPS(float64)                {}
func (t noopTraceAgent) SetSpanModifier(agent.SpanModifier)  {}
func (t noopTraceAgent) GetSpanModifier() agent.SpanModifier { return nil }

func init() {
	if lambdaRuntime := os.Getenv("AWS_LAMBDA_RUNTIME_API"); lambdaRuntime != "" {
		lambdaRuntimeURLPrefix = fmt.Sprintf("http://%s", lambdaRuntime)
	}
}
