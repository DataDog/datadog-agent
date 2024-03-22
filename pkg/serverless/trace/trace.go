// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/serverless-init/cloudservice"
	compcorecfg "github.com/DataDog/datadog-agent/comp/core/config"
	comptracecfg "github.com/DataDog/datadog-agent/comp/trace/config"
	ddConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// ServerlessTraceAgent represents a trace agent in a serverless context
type ServerlessTraceAgent struct {
	ta           *agent.Agent
	spanModifier *spanModifier
	cancel       context.CancelFunc
}

// Load abstracts the file configuration loading
type Load interface {
	Load() (*config.AgentConfig, error)
}

// LoadConfig is implementing Load to retrieve the config
type LoadConfig struct {
	Path string
}

// httpURLMetaKey is the key of the span meta containing the HTTP URL
const httpURLMetaKey = "http.url"

// tcpRemoteHostMetaKey is the key of the span meta containing the TCP remote host i.e 127.0.0.1
const tcpRemoteHostMetaKey = "tcp.remote.host"

// tcpRemotePortMetaKey is the the key of the span meta containing the TCP remote port e.g 8124
const tcpRemotePortMetaKey = "tcp.remote.port"

// dnsAddressMetaKey is the key of the span meta containing the DNS address
const dnsAddressMetaKey = "dns.address"

// lambdaRuntimeUrlPrefix is the first part of a URL for a call to the Lambda runtime API
const lambdaRuntimeURLPrefix = "http://127.0.0.1:9001"

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
	} else if c == nil {
		return nil, fmt.Errorf("No error, but no configuration component was produced - bailing out")
	}
	return comptracecfg.LoadConfigFile(l.Path, c)
}

// Start starts the agent
//
//nolint:revive // TODO(SERV) Fix revive linter
func (s *ServerlessTraceAgent) Start(enabled bool, loadConfig Load, lambdaSpanChan chan<- *pb.Span, coldStartSpanId uint64) {
	if enabled {
		// Set the serverless config option which will be used to determine if
		// hostname should be resolved. Skipping hostname resolution saves >1s
		// in load time between gRPC calls and agent commands.
		ddConfig.Datadog.Set("serverless.enabled", true, model.SourceAgentRuntime)

		tc, confErr := loadConfig.Load()
		if confErr != nil {
			log.Errorf("Unable to load trace agent config: %s", confErr)
		} else {
			context, cancel := context.WithCancel(context.Background())
			tc.Hostname = ""
			tc.SynchronousFlushing = true
			s.ta = agent.NewAgent(context, tc, telemetry.NewNoopCollector(), &statsd.NoOpClient{})
			s.spanModifier = &spanModifier{
				coldStartSpanId: coldStartSpanId,
				lambdaSpanChan:  lambdaSpanChan,
				ddOrigin:        getDDOrigin(),
			}

			s.ta.ModifySpan = s.spanModifier.ModifySpan
			s.ta.DiscardSpan = filterSpanFromLambdaLibraryOrRuntime
			s.cancel = cancel
			go s.ta.Run()
		}
	}
}

// Flush performs a synchronous flushing in the trace agent
func (s *ServerlessTraceAgent) Flush() {
	if s.Get() != nil {
		s.ta.FlushSync()
	}
}

// Get returns the trace agent instance
func (s *ServerlessTraceAgent) Get() *agent.Agent {
	return s.ta
}

// SetTags sets the tags to the trace agent config and span processor
func (s *ServerlessTraceAgent) SetTags(tagMap map[string]string) {
	if s.Get() != nil {
		s.ta.SetGlobalTagsUnsafe(tagMap)
		s.spanModifier.tags = tagMap
	} else {
		log.Debug("could not set tags as the trace agent has not been initialized")
	}
}

// Stop stops the trace agent
func (s *ServerlessTraceAgent) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

//nolint:revive // TODO(SERV) Fix revive linter
func (s *ServerlessTraceAgent) SetSpanModifier(fn func(*pb.TraceChunk, *pb.Span)) {
	s.ta.ModifySpan = fn
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
