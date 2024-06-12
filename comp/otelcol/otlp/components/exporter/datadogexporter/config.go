// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package datadogexporter

import (
	"encoding"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"
)

var (
	errUnsetAPIKey   = errors.New("api.key is not set")
	errNoMetadata    = errors.New("only_metadata can't be enabled when host_metadata::enabled = false or host_metadata::hostname_source != first_resource")
	errEmptyEndpoint = errors.New("endpoint cannot be empty")
)

const (
	// DefaultSite is the default site of the Datadog intake to send data to
	DefaultSite = "datadoghq.com"
)

// APIConfig defines the API configuration options
type APIConfig struct {
	// Key is the Datadog API key to associate your Agent's data with your organization.
	// Create a new API key here: https://app.datadoghq.com/account/settings
	Key configopaque.String `mapstructure:"key"`

	// Site is the site of the Datadog intake to send data to.
	// The default value is "datadoghq.com".
	Site string `mapstructure:"site"`

	// FailOnInvalidKey states whether to exit at startup on invalid API key.
	// The default value is false.
	FailOnInvalidKey bool `mapstructure:"fail_on_invalid_key"`
}

// TracesConfig defines the traces exporter specific configuration options
type TracesConfig struct {
	// TCPAddr.Endpoint is the host of the Datadog intake server to send traces to.
	// If unset, the value is obtained from the Site.
	confignet.TCPAddrConfig `mapstructure:",squash"`

	// ignored resources
	// A blacklist of regular expressions can be provided to disable certain traces based on their resource name
	// all entries must be surrounded by double quotes and separated by commas.
	// ignore_resources: ["(GET|POST) /healthcheck"]
	IgnoreResources []string `mapstructure:"ignore_resources"`

	// SpanNameRemappings is the map of datadog span names and preferred name to map to. This can be used to
	// automatically map Datadog Span Operation Names to an updated value. All entries should be key/value pairs.
	// span_name_remappings:
	//   io.opentelemetry.javaagent.spring.client: spring.client
	//   instrumentation:express.server: express
	//   go.opentelemetry.io_contrib_instrumentation_net_http_otelhttp.client: http.client
	SpanNameRemappings map[string]string `mapstructure:"span_name_remappings"`

	// If set to true the OpenTelemetry span name will used in the Datadog resource name.
	// If set to false the resource name will be filled with the instrumentation library name + span kind.
	// The default value is `true`.
	SpanNameAsResourceName bool `mapstructure:"span_name_as_resource_name"`

	// If set to true, root spans and spans with a server or consumer `span.kind` will be marked as top-level.
	// Additionally, spans with a client or producer `span.kind` will have stats computed.
	// Enabling this config option may increase the number of spans that generate trace metrics, and may change which spans appear as top-level in Datadog.
	// ComputeTopLevelBySpanKind needs to be enabled in both the Datadog connector and Datadog exporter configs if both components are being used.
	// The default value is `true`.
	ComputeTopLevelBySpanKind bool `mapstructure:"compute_top_level_by_span_kind"`

	// TraceBuffer specifies the number of Datadog Agent TracerPayloads to buffer before dropping.
	// The default value is 0, meaning the Datadog Agent TracerPayloads are unbuffered.
	TraceBuffer int `mapstructure:"trace_buffer"`
}

// LogsConfig defines logs exporter specific configuration
type LogsConfig struct {
	// TCPAddr.Endpoint is the host of the Datadog intake server to send logs to.
	// If unset, the value is obtained from the Site.
	confignet.TCPAddrConfig `mapstructure:",squash"`

	// DumpPayloads report whether payloads should be dumped when logging level is debug.
	DumpPayloads bool `mapstructure:"dump_payloads"`
}

// TagsConfig defines the tag-related configuration
// It is embedded in the configuration
type TagsConfig struct {
	// Hostname is the fallback hostname used for payloads without hostname-identifying attributes.
	// This option will NOT change the hostname applied to your metrics, traces and logs if they already have hostname-identifying attributes.
	// If unset, the hostname will be determined automatically. See https://docs.datadoghq.com/opentelemetry/schema_semantics/hostname/?tab=datadogexporter#fallback-hostname-logic for details.
	//
	// Prefer using the `datadog.host.name` resource attribute over using this setting.
	// See https://docs.datadoghq.com/opentelemetry/schema_semantics/hostname/?tab=datadogexporter#general-hostname-semantic-conventions for details.
	Hostname string `mapstructure:"hostname"`
}

// HostnameSource is the source for the hostname of host metadata.
type HostnameSource string

const (
	// HostnameSourceFirstResource picks the host metadata hostname from the resource
	// attributes on the first OTLP payload that gets to the exporter. If it is lacking any
	// hostname-like attributes, it will fallback to 'config_or_system' behavior (see below).
	//
	// Do not use this hostname source if receiving data from multiple hosts.
	HostnameSourceFirstResource HostnameSource = "first_resource"

	// HostnameSourceConfigOrSystem picks the host metadata hostname from the 'hostname' setting,
	// and if this is empty, from available system APIs and cloud provider endpoints.
	HostnameSourceConfigOrSystem HostnameSource = "config_or_system"
)

var _ encoding.TextUnmarshaler = (*HostnameSource)(nil)

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (sm *HostnameSource) UnmarshalText(in []byte) error {
	switch mode := HostnameSource(in); mode {
	case HostnameSourceFirstResource,
		HostnameSourceConfigOrSystem:
		*sm = mode
		return nil
	default:
		return fmt.Errorf("invalid host metadata hostname source %q", mode)
	}
}

// HostMetadataConfig defines the host metadata related configuration.
// Host metadata is the information used for populating the infrastructure list,
// the host map and providing host tags functionality.
//
// The exporter will send host metadata for a single host, whose name is chosen
// according to `host_metadata::hostname_source`.
type HostMetadataConfig struct {
	// Enabled enables the host metadata functionality.
	Enabled bool `mapstructure:"enabled"`

	// HostnameSource is the source for the hostname of host metadata.
	// This hostname is used for identifying the infrastructure list, host map and host tag information related to the host where the Datadog exporter is running.
	// Changing this setting will not change the host used to tag your metrics, traces and logs in any way.
	// For remote hosts, see https://docs.datadoghq.com/opentelemetry/schema_semantics/host_metadata/.
	//
	// Valid values are 'first_resource' and 'config_or_system':
	// - 'first_resource' picks the host metadata hostname from the resource
	//    attributes on the first OTLP payload that gets to the exporter.
	//    If the first payload lacks hostname-like attributes, it will fallback to 'config_or_system'.
	//    **Do not use this hostname source if receiving data from multiple hosts**.
	// - 'config_or_system' picks the host metadata hostname from the 'hostname' setting,
	//    If this is empty it will use available system APIs and cloud provider endpoints.
	//
	// The default is 'config_or_system'.
	HostnameSource HostnameSource `mapstructure:"hostname_source"`

	// Tags is a list of host tags.
	// These tags will be attached to telemetry signals that have the host metadata hostname.
	// To attach tags to telemetry signals regardless of the host, use a processor instead.
	Tags []string `mapstructure:"tags"`
}

// Config defines configuration for the Datadog exporter.
type Config struct {
	confighttp.ClientConfig      `mapstructure:",squash"` // squash ensures fields are correctly decoded in embedded struct.
	exporterhelper.QueueSettings `mapstructure:"sending_queue"`
	configretry.BackOffConfig    `mapstructure:"retry_on_failure"`

	TagsConfig `mapstructure:",squash"`

	// API defines the Datadog API configuration.
	API APIConfig `mapstructure:"api"`

	// Metrics defines the Metrics exporter specific configuration
	Metrics serializerexporter.MetricsConfig `mapstructure:"metrics"`

	// Traces defines the Traces exporter specific configuration
	Traces TracesConfig `mapstructure:"traces"`

	// Logs defines the Logs exporter specific configuration
	Logs LogsConfig `mapstructure:"logs"`

	// HostMetadata defines the host metadata specific configuration
	HostMetadata HostMetadataConfig `mapstructure:"host_metadata"`

	// OnlyMetadata defines whether to only send metadata
	// This is useful for agent-collector setups, so that
	// metadata about a host is sent to the backend even
	// when telemetry data is reported via a different host.
	//
	// This flag is incompatible with disabling host metadata,
	// `use_resource_metadata`, or `host_metadata::hostname_source != first_resource`
	OnlyMetadata bool `mapstructure:"only_metadata"`

	// Non-fatal warnings found during configuration loading.
	warnings []error
}

// logWarnings logs warning messages that were generated on unmarshaling.
func (c *Config) logWarnings(logger *zap.Logger) {
	for _, err := range c.warnings {
		logger.Warn(fmt.Sprintf("%v", err))
	}
}

var _ component.Config = (*Config)(nil)

// Validate the configuration for errors. This is required by component.Config.
func (c *Config) Validate() error {
	if err := validateClientConfig(c.ClientConfig); err != nil {
		return err
	}

	if c.OnlyMetadata && (!c.HostMetadata.Enabled || c.HostMetadata.HostnameSource != HostnameSourceFirstResource) {
		return errNoMetadata
	}

	if err := validate.ValidHostname(c.Hostname); c.Hostname != "" && err != nil {
		return fmt.Errorf("hostname field is invalid: %w", err)
	}

	if c.API.Key == "" {
		return errUnsetAPIKey
	}

	if c.Traces.IgnoreResources != nil {
		for _, entry := range c.Traces.IgnoreResources {
			_, err := regexp.Compile(entry)
			if err != nil {
				return fmt.Errorf("'%s' is not valid resource filter regular expression", entry)
			}
		}
	}

	if c.Traces.SpanNameRemappings != nil {
		for key, value := range c.Traces.SpanNameRemappings {
			if value == "" {
				return fmt.Errorf("'%s' is not valid value for span name remapping", value)
			}
			if key == "" {
				return fmt.Errorf("'%s' is not valid key for span name remapping", key)
			}
		}
	}

	exp := serializerexporter.ExporterConfig{
		Metrics: c.Metrics,
	}
	if err := exp.Validate(); err != nil {
		return err
	}
	err := c.Metrics.HistConfig.Validate()
	if err != nil {
		return err
	}

	return nil
}

func validateClientConfig(cfg confighttp.ClientConfig) error {
	var unsupported []string
	if cfg.Auth != nil {
		unsupported = append(unsupported, "auth")
	}
	if cfg.Endpoint != "" {
		unsupported = append(unsupported, "endpoint")
	}
	if cfg.Compression != "" {
		unsupported = append(unsupported, "compression")
	}
	if cfg.ProxyURL != "" {
		unsupported = append(unsupported, "proxy_url")
	}
	if cfg.Headers != nil {
		unsupported = append(unsupported, "headers")
	}
	if cfg.HTTP2ReadIdleTimeout != 0 {
		unsupported = append(unsupported, "http2_read_idle_timeout")
	}
	if cfg.HTTP2PingTimeout != 0 {
		unsupported = append(unsupported, "http2_ping_timeout")
	}

	if len(unsupported) > 0 {
		return fmt.Errorf("these confighttp client configs are currently not respected by Datadog exporter: %s", strings.Join(unsupported, ", "))
	}
	return nil
}

var _ error = (*renameError)(nil)

// renameError is an error related to a renamed setting.
type renameError struct {
	// oldName of the configuration option.
	oldName string
	// newName of the configuration option.
	newName string
	// issueNumber on opentelemetry-collector-contrib for tracking
	issueNumber uint
}

// List of settings that have been removed, but for which we keep a custom error.
var removedSettings = []renameError{
	{
		oldName:     "metrics::send_monotonic_counter",
		newName:     "metrics::sums::cumulative_monotonic_mode",
		issueNumber: 8489,
	},
	{
		oldName:     "tags",
		newName:     "host_metadata::tags",
		issueNumber: 9099,
	},
	{
		oldName:     "send_metadata",
		newName:     "host_metadata::enabled",
		issueNumber: 9099,
	},
	{
		oldName:     "use_resource_metadata",
		newName:     "host_metadata::hostname_source",
		issueNumber: 9099,
	},
	{
		oldName:     "metrics::report_quantiles",
		newName:     "metrics::summaries::mode",
		issueNumber: 8845,
	},
	{
		oldName:     "metrics::instrumentation_library_metadata_as_tags",
		newName:     "metrics::instrumentation_scope_as_tags",
		issueNumber: 11135,
	},
}

// Error implements the error interface.
func (e renameError) Error() string {
	return fmt.Sprintf(
		"%q was removed in favor of %q. See https://github.com/open-telemetry/opentelemetry-collector-contrib/issues/%d",
		e.oldName,
		e.newName,
		e.issueNumber,
	)
}

func handleRemovedSettings(configMap *confmap.Conf) error {
	var errs []error
	for _, removedErr := range removedSettings {
		if configMap.IsSet(removedErr.oldName) {
			errs = append(errs, removedErr)
		}
	}

	return errors.Join(errs...)
}

var _ confmap.Unmarshaler = (*Config)(nil)

// Unmarshal a configuration map into the configuration struct.
func (c *Config) Unmarshal(configMap *confmap.Conf) error {
	if err := handleRemovedSettings(configMap); err != nil {
		return err
	}

	err := configMap.Unmarshal(c)
	if err != nil {
		return err
	}

	// Add deprecation warnings for deprecated settings.
	renamingWarnings, err := handleRenamedSettings(configMap, c)
	if err != nil {
		return err
	}
	c.warnings = append(c.warnings, renamingWarnings...)

	c.API.Key = configopaque.String(strings.TrimSpace(string(c.API.Key)))

	if !configMap.IsSet("traces::endpoint") {
		c.Traces.TCPAddrConfig.Endpoint = fmt.Sprintf("https://trace.agent.%s", c.API.Site)
	}
	if !configMap.IsSet("logs::endpoint") {
		c.Logs.TCPAddrConfig.Endpoint = fmt.Sprintf("https://http-intake.logs.%s", c.API.Site)
	}

	// Return an error if an endpoint is explicitly set to ""
	if c.Traces.TCPAddrConfig.Endpoint == "" || c.Logs.TCPAddrConfig.Endpoint == "" {
		return errEmptyEndpoint
	}

	const (
		initialValueSetting = "metrics::sums::initial_cumulative_monotonic_value"
		cumulMonoMode       = "metrics::sums::cumulative_monotonic_mode"
	)
	if configMap.IsSet(initialValueSetting) && c.Metrics.SumConfig.CumulativeMonotonicMode != serializerexporter.CumulativeMonotonicSumModeToDelta {
		return fmt.Errorf("%q can only be configured when %q is set to %q",
			initialValueSetting, cumulMonoMode, serializerexporter.CumulativeMonotonicSumModeToDelta)
	}

	return nil
}
