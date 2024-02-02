// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package config

import (
	"net/http"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// RemoteClient client is used to APM Sampling Updates from a remote source.
// This is an interface around the client provided by pkg/config/remote to allow for easier testing.
type RemoteClient interface {
	Close()
	Start()
	Subscribe(string, func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
	UpdateApplyStatus(cfgPath string, status state.ApplyStatus)
}

// AgentConfig handles the interpretation of the configuration (with default
// behaviors) in one place. It is also a simple structure to share across all
// the Agent components, with 100% safe and reliable values.
// It is exposed with expvar, so make sure to exclude any sensible field
// from JSON encoding. Use New() to create an instance.
type AgentConfig struct {
	Features map[string]struct{}

	Enabled      bool
	AgentVersion string
	GitCommit    string
	Site         string // the intake site to use (e.g. "datadoghq.com")

	// FargateOrchestrator specifies the name of the Fargate orchestrator. e.g. "ECS", "EKS", "Unknown"
	FargateOrchestrator FargateOrchestratorName

	// Global
	Hostname   string
	DefaultEnv string // the traces will default to this environment
	ConfigPath string // the source of this config, if any

	// Endpoints specifies the set of hosts and API keys where traces and stats
	// will be uploaded to. The first endpoint is the main configuration endpoint;
	// any following ones are read from the 'additional_endpoints' parts of the
	// configuration file, if present.
	Endpoints []*Endpoint

	// Concentrator
	BucketInterval         time.Duration // the size of our pre-aggregation per bucket
	ExtraAggregators       []string      // DEPRECATED
	PeerServiceAggregation bool          // TO BE DEPRECATED - enables/disables stats aggregation for peer.service, used by Concentrator and ClientStatsAggregator
	PeerTagsAggregation    bool          // enables/disables stats aggregation for peer entity tags, used by Concentrator and ClientStatsAggregator
	ComputeStatsBySpanKind bool          // enables/disables the computing of stats based on a span's `span.kind` field
	PeerTags               []string      // additional tags to use for peer entity stats aggregation

	// Sampler configuration
	ExtraSampleRate float64
	TargetTPS       float64
	ErrorTPS        float64
	MaxEPS          float64
	MaxRemoteTPS    float64

	// Rare Sampler configuration
	RareSamplerEnabled        bool
	RareSamplerTPS            int
	RareSamplerCooldownPeriod time.Duration
	RareSamplerCardinality    int

	// Receiver
	ReceiverHost    string
	ReceiverPort    int
	ReceiverSocket  string // if not empty, UDS will be enabled on unix://<receiver_socket>
	ConnectionLimit int    // for rate-limiting, how many unique connections to allow in a lease period (30s)
	ReceiverTimeout int
	MaxRequestBytes int64 // specifies the maximum allowed request size for incoming trace payloads
	TraceBuffer     int   // specifies the number of traces to buffer before blocking.
	Decoders        int   // specifies the number of traces that can be concurrently decoded.
	MaxConnections  int   // specifies the maximum number of concurrent incoming connections allowed.
	DecoderTimeout  int   // specifies the maximum time in milliseconds that the decoders will wait for a turn to accept a payload before returning 429

	WindowsPipeName        string
	PipeBufferSize         int
	PipeSecurityDescriptor string

	GUIPort string // the port of the Datadog Agent GUI (for control access)

	// Writers
	SynchronousFlushing     bool // Mode where traces are only submitted when FlushAsync is called, used for Serverless Extension
	StatsWriter             *WriterConfig
	TraceWriter             *WriterConfig
	ConnectionResetInterval time.Duration // frequency at which outgoing connections are reset. 0 means no reset is performed
	// MaxSenderRetries is the maximum number of retries that a sender will perform
	// before giving up. Note that the sender may not perform all MaxSenderRetries if
	// the agent is under load and the outgoing payload queue is full. In that
	// case, the sender will drop failed payloads when it is unable to enqueue
	// them for another retry.
	MaxSenderRetries int

	// internal telemetry
	StatsdEnabled  bool
	StatsdHost     string
	StatsdPort     int
	StatsdPipeName string // for Windows Pipes
	StatsdSocket   string // for UDS Sockets

	// logging
	LogFilePath   string
	LogThrottling bool

	// watchdog
	MaxMemory        float64       // MaxMemory is the threshold (bytes allocated) above which program panics and exits, to be restarted
	MaxCPU           float64       // MaxCPU is the max UserAvg CPU the program should consume
	WatchdogInterval time.Duration // WatchdogInterval is the delay between 2 watchdog checks

	// http/s proxying
	ProxyURL          *url.URL
	SkipSSLValidation bool

	// filtering
	Ignore map[string][]string

	// ReplaceTags is used to filter out sensitive information from tag values.
	// It maps tag keys to a set of replacements. Only supported in A6.
	ReplaceTags []*ReplaceRule

	// GlobalTags list metadata that will be added to all spans
	GlobalTags map[string]string

	// transaction analytics
	AnalyzedRateByServiceLegacy map[string]float64
	AnalyzedSpansByService      map[string]map[string]float64

	// infrastructure agent binary
	DDAgentBin string

	// Obfuscation holds sensitive data obufscator's configuration.
	Obfuscation *ObfuscationConfig

	// MaxResourceLen the maximum length the resource can have
	MaxResourceLen int

	// RequireTags specifies a list of tags which must be present on the root span in order for a trace to be accepted.
	RequireTags []*Tag

	// RejectTags specifies a list of tags which must be absent on the root span in order for a trace to be accepted.
	RejectTags []*Tag

	// RequireTagsRegex specifies a list of regexp for tags which must be present on the root span in order for a trace to be accepted.
	RequireTagsRegex []*TagRegex

	// RejectTagsRegex specifies a list of regexp for tags which must be absent on the root span in order for a trace to be accepted.
	RejectTagsRegex []*TagRegex

	// OTLPReceiver holds the configuration for OpenTelemetry receiver.
	OTLPReceiver *OTLP

	// ProfilingProxy specifies settings for the profiling proxy.
	ProfilingProxy ProfilingProxyConfig

	// Telemetry settings
	TelemetryConfig *TelemetryConfig

	// EVPProxy contains the settings for the EVPProxy proxy.
	EVPProxy EVPProxy

	// DebuggerProxy contains the settings for the Live Debugger proxy.
	DebuggerProxy DebuggerProxyConfig

	// DebuggerDiagnosticsProxy contains the settings for the Live Debugger diagnostics proxy.
	DebuggerDiagnosticsProxy DebuggerProxyConfig

	// SymDBProxy contains the settings for the Symbol Database proxy.
	SymDBProxy SymDBProxyConfig

	// Proxy specifies a function to return a proxy for a given Request.
	// See (net/http.Transport).Proxy for more details.
	Proxy func(*http.Request) (*url.URL, error) `json:"-"`

	// MaxCatalogEntries specifies the maximum number of services to be added to the priority sampler's
	// catalog. If not set (0) it will default to 5000.
	MaxCatalogEntries int

	// RemoteConfigClient retrieves sampling updates from the remote config backend
	RemoteConfigClient RemoteClient `json:"-"`

	// ContainerTags ...
	ContainerTags func(cid string) ([]string, error) `json:"-"`

	// ContainerProcRoot is the root dir for `proc` info
	ContainerProcRoot string

	// DebugServerPort defines the port used by the debug server
	DebugServerPort int

	// Install Signature
	InstallSignature InstallSignatureConfig

	// Lambda function name
	LambdaFunctionName string
}
