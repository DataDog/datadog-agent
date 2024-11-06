// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

// ServiceName specifies the service name used in the operating system.
const ServiceName = "datadog-trace-agent"

// ErrMissingAPIKey is returned when the config could not be validated due to missing API key.
var ErrMissingAPIKey = errors.New("you must specify an API Key, either via a configuration file or the DD_API_KEY env var")

// Endpoint specifies an endpoint that the trace agent will write data (traces, stats & services) to.
type Endpoint struct {
	APIKey string `json:"-"` // never marshal this
	Host   string

	// NoProxy will be set to true when the proxy setting for the trace API endpoint
	// needs to be ignored (e.g. it is part of the "no_proxy" list in the yaml settings).
	NoProxy bool
}

// TelemetryEndpointPrefix specifies the prefix of the telemetry endpoint URL.
const TelemetryEndpointPrefix = "https://instrumentation-telemetry-intake."

// OTLP holds the configuration for the OpenTelemetry receiver.
type OTLP struct {
	// BindHost specifies the host to bind the receiver to.
	BindHost string `mapstructure:"-"`

	// GRPCPort specifies the port to use for the plain HTTP receiver.
	// If unset (or 0), the receiver will be off.
	GRPCPort int `mapstructure:"grpc_port"`

	// SpanNameRemappings is the map of datadog span names and preferred name to map to. This can be used to
	// automatically map Datadog Span Operation Names to an updated value. All entries should be key/value pairs.
	SpanNameRemappings map[string]string `mapstructure:"span_name_remappings"`

	// SpanNameAsResourceName specifies whether the OpenTelemetry span's name should be
	// used as the Datadog span's operation name. By default (when this is false), the
	// operation name is deduced from a combination between the instrumentation scope
	// name and the span kind.
	//
	// For context, the OpenTelemetry 'Span Name' is equivalent to the Datadog 'resource name'.
	// The Datadog Span's Operation Name equivalent in OpenTelemetry does not exist, but the span's
	// kind comes close.
	SpanNameAsResourceName bool `mapstructure:"span_name_as_resource_name"`

	// MaxRequestBytes specifies the maximum number of bytes that will be read
	// from an incoming HTTP request.
	MaxRequestBytes int64 `mapstructure:"-"`

	// ProbabilisticSampling specifies the percentage of traces to ingest. Exceptions are made for errors
	// and rare traces (outliers) if "RareSamplerEnabled" is true. Invalid values are equivalent to 100.
	// If spans have the "sampling.priority" attribute set, probabilistic sampling is skipped and the user's
	// decision is followed.
	ProbabilisticSampling float64

	// AttributesTranslator specifies an OTLP to Datadog attributes translator.
	AttributesTranslator *attributes.Translator `mapstructure:"-"`
}

// ObfuscationConfig holds the configuration for obfuscating sensitive data
// for various span types.
type ObfuscationConfig struct {
	// ES holds the obfuscation configuration for ElasticSearch bodies.
	ES obfuscate.JSONConfig `mapstructure:"elasticsearch"`

	// OpenSearch holds the obfuscation configuration for OpenSearch bodies.
	OpenSearch obfuscate.JSONConfig `mapstructure:"opensearch"`

	// Mongo holds the obfuscation configuration for MongoDB queries.
	Mongo obfuscate.JSONConfig `mapstructure:"mongodb"`

	// SQLExecPlan holds the obfuscation configuration for SQL Exec Plans. This is strictly for safety related obfuscation,
	// not normalization. Normalization of exec plans is configured in SQLExecPlanNormalize.
	SQLExecPlan obfuscate.JSONConfig `mapstructure:"sql_exec_plan"`

	// SQLExecPlanNormalize holds the normalization configuration for SQL Exec Plans.
	SQLExecPlanNormalize obfuscate.JSONConfig `mapstructure:"sql_exec_plan_normalize"`

	// HTTP holds the obfuscation settings for HTTP URLs.
	HTTP obfuscate.HTTPConfig `mapstructure:"http"`

	// RemoveStackTraces specifies whether stack traces should be removed.
	// More specifically "error.stack" tag values will be cleared.
	RemoveStackTraces bool `mapstructure:"remove_stack_traces"`

	// Redis holds the configuration for obfuscating the "redis.raw_command" tag
	// for spans of type "redis".
	Redis obfuscate.RedisConfig `mapstructure:"redis"`

	// Memcached holds the configuration for obfuscating the "memcached.command" tag
	// for spans of type "memcached".
	Memcached obfuscate.MemcachedConfig `mapstructure:"memcached"`

	// CreditCards holds the configuration for obfuscating credit cards.
	CreditCards obfuscate.CreditCardsConfig `mapstructure:"credit_cards"`
}

func obfuscationMode(enabled bool) obfuscate.ObfuscationMode {
	if enabled {
		return obfuscate.ObfuscateOnly
	}
	return ""
}

// Export returns an obfuscate.Config matching o.
func (o *ObfuscationConfig) Export(conf *AgentConfig) obfuscate.Config {
	return obfuscate.Config{
		SQL: obfuscate.SQLConfig{
			TableNames:       conf.HasFeature("table_names"),
			ReplaceDigits:    conf.HasFeature("quantize_sql_tables") || conf.HasFeature("replace_sql_digits"),
			KeepSQLAlias:     conf.HasFeature("keep_sql_alias"),
			DollarQuotedFunc: conf.HasFeature("dollar_quoted_func"),
			Cache:            conf.HasFeature("sql_cache"),
			ObfuscationMode:  obfuscationMode(conf.HasFeature("sqllexer")),
		},
		ES:                   o.ES,
		OpenSearch:           o.OpenSearch,
		Mongo:                o.Mongo,
		SQLExecPlan:          o.SQLExecPlan,
		SQLExecPlanNormalize: o.SQLExecPlanNormalize,
		HTTP:                 o.HTTP,
		Redis:                o.Redis,
		Memcached:            o.Memcached,
		CreditCard:           o.CreditCards,
		Logger:               new(debugLogger),
	}
}

type debugLogger struct{}

func (debugLogger) Debugf(format string, params ...interface{}) {
	log.Debugf(format, params...)
}

// Enablable can represent any option that has an "enabled" boolean sub-field.
type Enablable struct {
	Enabled bool `mapstructure:"enabled"`
}

// TelemetryConfig holds Instrumentation telemetry Endpoints information
type TelemetryConfig struct {
	Enabled   bool `mapstructure:"enabled"`
	Endpoints []*Endpoint
}

// ReplaceRule specifies a replace rule.
type ReplaceRule struct {
	// Name specifies the name of the tag that the replace rule addresses. However,
	// some exceptions apply such as:
	// • "resource.name" will target the resource
	// • "*" will target all tags and the resource
	Name string `mapstructure:"name"`

	// Pattern specifies the regexp pattern to be used when replacing. It must compile.
	Pattern string `mapstructure:"pattern"`

	// Re holds the compiled Pattern and is only used internally.
	Re *regexp.Regexp `mapstructure:"-"`

	// Repl specifies the replacement string to be used when Pattern matches.
	Repl string `mapstructure:"repl"`
}

// WriterConfig specifies configuration for an API writer.
type WriterConfig struct {
	// ConnectionLimit specifies the maximum number of concurrent outgoing
	// connections allowed for the sender.
	ConnectionLimit int `mapstructure:"connection_limit"`

	// QueueSize specifies the maximum number or payloads allowed to be queued
	// in the sender.
	QueueSize int `mapstructure:"queue_size"`

	// FlushPeriodSeconds specifies the frequency at which the writer's buffer
	// will be flushed to the sender, in seconds. Fractions are permitted.
	FlushPeriodSeconds float64 `mapstructure:"flush_period_seconds"`
}

// FargateOrchestratorName is a Fargate orchestrator name.
type FargateOrchestratorName string

const (
	// OrchestratorECS represents AWS ECS
	OrchestratorECS FargateOrchestratorName = "ECS"
	// OrchestratorEKS represents AWS EKS
	OrchestratorEKS FargateOrchestratorName = "EKS"
	// OrchestratorUnknown is used when we cannot retrieve the orchestrator
	OrchestratorUnknown FargateOrchestratorName = "Unknown"
)

// ProfilingProxyConfig ...
type ProfilingProxyConfig struct {
	// DDURL ...
	DDURL string
	// AdditionalEndpoints ...
	AdditionalEndpoints map[string][]string
}

// EVPProxy contains the settings for the EVPProxy proxy.
type EVPProxy struct {
	// Enabled reports whether EVPProxy is enabled (true by default).
	Enabled bool
	// DDURL is the Datadog site to forward payloads to (defaults to the Site setting if not set).
	DDURL string
	// APIKey is the main API Key (defaults to the main API key).
	APIKey string `json:"-"` // Never marshal this field
	// ApplicationKey to be used for requests with the X-Datadog-NeedsAppKey set (defaults to the top-level Application Key).
	ApplicationKey string `json:"-"` // Never marshal this field
	// AdditionalEndpoints is a map of additional Datadog sites to API keys.
	AdditionalEndpoints map[string][]string
	// MaxPayloadSize indicates the size at which payloads will be rejected, in bytes.
	MaxPayloadSize int64
	// ReceiverTimeout indicates the maximum time an EVPProxy request can take. Value in seconds.
	ReceiverTimeout int
}

// InstallSignatureConfig contains the information on how the agent was installed
// and a unique identifier that distinguishes this agent from others.
type InstallSignatureConfig struct {
	Found       bool   `json:"-"`
	InstallID   string `json:"install_id"`
	InstallType string `json:"install_type"`
	InstallTime int64  `json:"install_time"`
}

// DebuggerProxyConfig ...
type DebuggerProxyConfig struct {
	// DDURL ...
	DDURL string
	// APIKey ...
	APIKey string `json:"-"` // Never marshal this field
	// AdditionalEndpoints is a map of additional Datadog sites to API keys.
	AdditionalEndpoints map[string][]string `json:"-"` // Never marshal this field
}

// SymDBProxyConfig ...
type SymDBProxyConfig struct {
	// DDURL ...
	DDURL string
	// APIKey ...
	APIKey string `json:"-"` // Never marshal this field
	// AdditionalEndpoints is a map of additional Datadog endpoints to API keys.
	AdditionalEndpoints map[string][]string `json:"-"` // Never marshal this field
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

	// Probabilistic Sampler configuration
	ProbabilisticSamplerEnabled            bool
	ProbabilisticSamplerHashSeed           uint32
	ProbabilisticSamplerSamplingPercentage float32

	// Receiver
	ReceiverEnabled bool // specifies whether Receiver listeners are enabled. Unless OTLPReceiver is used, this should always be true.
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
	// HTTP client used in writer connections. If nil, default client values will be used.
	HTTPClientFunc func() *http.Client `json:"-"`

	// internal telemetry
	StatsdEnabled  bool
	StatsdHost     string
	StatsdPort     int
	StatsdPipeName string // for Windows Pipes
	StatsdSocket   string // for UDS Sockets

	// logging
	LogFilePath string

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

// RemoteClient client is used to APM Sampling Updates from a remote source.
// This is an interface around the client provided by pkg/config/remote to allow for easier testing.
type RemoteClient interface {
	Close()
	Start()
	Subscribe(string, func(update map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)))
	UpdateApplyStatus(cfgPath string, status state.ApplyStatus)
}

// Tag represents a key/value pair.
type Tag struct {
	K, V string
}

// TagRegex represents a key/value regex pattern pair.
type TagRegex struct {
	K string
	V *regexp.Regexp
}

// New returns a configuration with the default values.
func New() *AgentConfig {
	return &AgentConfig{
		Enabled:             true,
		DefaultEnv:          "none",
		Endpoints:           []*Endpoint{{Host: "https://trace.agent.datadoghq.com"}},
		FargateOrchestrator: OrchestratorUnknown,
		Site:                "datadoghq.com",
		MaxCatalogEntries:   5000,

		BucketInterval: time.Duration(10) * time.Second,

		ExtraSampleRate: 1.0,
		TargetTPS:       10,
		ErrorTPS:        10,
		MaxEPS:          200,
		MaxRemoteTPS:    100,

		RareSamplerEnabled:        false,
		RareSamplerTPS:            5,
		RareSamplerCooldownPeriod: 5 * time.Minute,
		RareSamplerCardinality:    200,

		ReceiverEnabled:        true,
		ReceiverHost:           "localhost",
		ReceiverPort:           8126,
		MaxRequestBytes:        25 * 1024 * 1024, // 25MB
		PipeBufferSize:         1_000_000,
		PipeSecurityDescriptor: "D:AI(A;;GA;;;WD)",
		GUIPort:                "5002",

		StatsWriter:             new(WriterConfig),
		TraceWriter:             new(WriterConfig),
		ConnectionResetInterval: 0, // disabled
		MaxSenderRetries:        4,

		StatsdHost:    "localhost",
		StatsdPort:    8125,
		StatsdEnabled: true,

		LambdaFunctionName: os.Getenv("AWS_LAMBDA_FUNCTION_NAME"),

		MaxMemory:        5e8, // 500 Mb, should rarely go above 50 Mb
		MaxCPU:           0.5, // 50%, well behaving agents keep below 5%
		WatchdogInterval: 10 * time.Second,

		Ignore:                      make(map[string][]string),
		AnalyzedRateByServiceLegacy: make(map[string]float64),
		AnalyzedSpansByService:      make(map[string]map[string]float64),
		Obfuscation:                 &ObfuscationConfig{},
		MaxResourceLen:              5000,

		GlobalTags: computeGlobalTags(),

		Proxy:         http.ProxyFromEnvironment,
		OTLPReceiver:  &OTLP{},
		ContainerTags: noopContainerTagsFunc,
		TelemetryConfig: &TelemetryConfig{
			Endpoints: []*Endpoint{{Host: TelemetryEndpointPrefix + "datadoghq.com"}},
		},
		EVPProxy: EVPProxy{
			Enabled:        true,
			MaxPayloadSize: 5 * 1024 * 1024,
		},

		Features:               make(map[string]struct{}),
		PeerTagsAggregation:    true,
		ComputeStatsBySpanKind: true,
	}
}

func computeGlobalTags() map[string]string {
	if inAzureAppServices() {
		return traceutil.GetAppServicesTags()
	}
	return make(map[string]string)
}

// ErrContainerTagsFuncNotDefined is returned when the containerTags function is not defined.
var ErrContainerTagsFuncNotDefined = errors.New("containerTags function not defined")

func noopContainerTagsFunc(_ string) ([]string, error) {
	return nil, ErrContainerTagsFuncNotDefined
}

// APIKey returns the first (main) endpoint's API key.
func (c *AgentConfig) APIKey() string {
	if len(c.Endpoints) == 0 {
		return ""
	}
	return c.Endpoints[0].APIKey
}

// UpdateAPIKey updates the API Key associated with the main endpoint.
func (c *AgentConfig) UpdateAPIKey(val string) {
	if len(c.Endpoints) == 0 {
		return
	}
	c.Endpoints[0].APIKey = val
}

// NewHTTPClient returns a new http.Client to be used for outgoing connections to the
// Datadog API.
func (c *AgentConfig) NewHTTPClient() *ResetClient {
	// If a custom HTTPClientFunc been set, use it. Otherwise use default client values
	if c.HTTPClientFunc != nil {
		return NewResetClient(c.ConnectionResetInterval, c.HTTPClientFunc)
	}
	return NewResetClient(c.ConnectionResetInterval, func() *http.Client {
		return &http.Client{
			Timeout:   10 * time.Second,
			Transport: c.NewHTTPTransport(),
		}
	})
}

// NewHTTPTransport returns a new http.Transport to be used for outgoing connections to
// the Datadog API.
func (c *AgentConfig) NewHTTPTransport() *http.Transport {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLValidation},
		// below field values are from http.DefaultTransport (go1.12)
		Proxy: c.Proxy,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return transport
}

// HasFeature returns true if the agent has the given feature flag.
func (c *AgentConfig) HasFeature(feat string) bool {
	_, ok := c.Features[feat]
	return ok
}

// AllFeatures returns a slice of all the feature flags the agent has.
func (c *AgentConfig) AllFeatures() []string {
	feats := []string{}
	for feat := range c.Features {
		feats = append(feats, feat)
	}
	return feats
}

// ConfiguredPeerTags returns the set of peer tags that should be used
// for aggregation based on the various config values and the base set of tags.
func (c *AgentConfig) ConfiguredPeerTags() []string {
	if !c.PeerTagsAggregation {
		return nil
	}
	return preparePeerTags(append(basePeerTags, c.PeerTags...))
}

func inAzureAppServices() bool {
	_, existsLinux := os.LookupEnv("WEBSITE_STACK")
	_, existsWin := os.LookupEnv("WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED")
	return existsLinux || existsWin
}
