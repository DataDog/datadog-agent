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
	"os"
	"regexp"
	"time"

	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"

	"github.com/DataDog/datadog-agent/pkg/obfuscate"
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
	CreditCards CreditCardsConfig `mapstructure:"credit_cards"`
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
		},
		ES:                   o.ES,
		Mongo:                o.Mongo,
		SQLExecPlan:          o.SQLExecPlan,
		SQLExecPlanNormalize: o.SQLExecPlanNormalize,
		HTTP:                 o.HTTP,
		Redis:                o.Redis,
		Memcached:            o.Memcached,
		Logger:               new(debugLogger),
	}
}

type debugLogger struct{}

func (debugLogger) Debugf(format string, params ...interface{}) {
	log.Debugf(format, params...)
}

// CreditCardsConfig holds the configuration for credit card obfuscation in
// (Meta) tags.
type CreditCardsConfig struct {
	// Enabled specifies whether this feature should be enabled.
	Enabled bool `mapstructure:"enabled"`

	// Luhn specifies whether Luhn checksum validation should be enabled.
	// https://dev.to/shiraazm/goluhn-a-simple-library-for-generating-calculating-and-verifying-luhn-numbers-588j
	// It reduces false positives, but increases the CPU time X3.
	Luhn bool `mapstructure:"luhn"`
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

		ReceiverHost:           "localhost",
		ReceiverPort:           8126,
		MaxRequestBytes:        25 * 1024 * 1024, // 25MB
		PipeBufferSize:         1_000_000,
		PipeSecurityDescriptor: "D:AI(A;;GA;;;WD)",
		GUIPort:                "5002",

		StatsWriter:             new(WriterConfig),
		TraceWriter:             new(WriterConfig),
		ConnectionResetInterval: 0, // disabled

		StatsdHost:    "localhost",
		StatsdPort:    8125,
		StatsdEnabled: true,

		LogThrottling:      true,
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

		Features: make(map[string]struct{}),
	}
}

func computeGlobalTags() map[string]string {
	if inAzureAppServices() {
		return traceutil.GetAppServicesTags()
	}
	return make(map[string]string)
}

func noopContainerTagsFunc(_ string) ([]string, error) {
	return nil, errors.New("ContainerTags function not defined")
}

// APIKey returns the first (main) endpoint's API key.
func (c *AgentConfig) APIKey() string {
	if len(c.Endpoints) == 0 {
		return ""
	}
	return c.Endpoints[0].APIKey
}

// NewHTTPClient returns a new http.Client to be used for outgoing connections to the
// Datadog API.
func (c *AgentConfig) NewHTTPClient() *ResetClient {
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
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return transport
}

//nolint:revive // TODO(APM) Fix revive linter
func (c *AgentConfig) HasFeature(feat string) bool {
	_, ok := c.Features[feat]
	return ok
}

//nolint:revive // TODO(APM) Fix revive linter
func (c *AgentConfig) AllFeatures() []string {
	feats := []string{}
	for feat := range c.Features {
		feats = append(feats, feat)
	}
	return feats
}

//nolint:revive // TODO(APM) Fix revive linter
func inAzureAppServices() bool {
	_, existsLinux := os.LookupEnv("APPSVC_RUN_ZIP")
	_, existsWin := os.LookupEnv("WEBSITE_APPSERVICEAPPLOGS_TRACE_ENABLED")
	return existsLinux || existsWin
}
