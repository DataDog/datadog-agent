package config

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/osutil"
	"github.com/StackVista/stackstate-agent/pkg/trace/writer/backoff"
	writerconfig "github.com/StackVista/stackstate-agent/pkg/trace/writer/config"
	log "github.com/cihub/seelog"
)

// apiEndpointPrefix is the URL prefix prepended to the default site value from YamlAgentConfig.
const apiEndpointPrefix = "https://trace.agent."

// ObfuscationConfig holds the configuration for obfuscating sensitive data
// for various span types.
type ObfuscationConfig struct {
	// ES holds the obfuscation configuration for ElasticSearch bodies.
	ES JSONObfuscationConfig `mapstructure:"elasticsearch"`

	// Mongo holds the obfuscation configuration for MongoDB queries.
	Mongo JSONObfuscationConfig `mapstructure:"mongodb"`

	// HTTP holds the obfuscation settings for HTTP URLs.
	HTTP HTTPObfuscationConfig `mapstructure:"http"`

	// RemoveStackTraces specifies whether stack traces should be removed.
	// More specifically "error.stack" tag values will be cleared.
	RemoveStackTraces bool `mapstructure:"remove_stack_traces"`

	// Redis holds the configuration for obfuscating the "redis.raw_command" tag
	// for spans of type "redis".
	Redis Enablable `mapstructure:"redis"`

	// Memcached holds the configuration for obfuscating the "memcached.command" tag
	// for spans of type "memcached".
	Memcached Enablable `mapstructure:"memcached"`
}

// HTTPObfuscationConfig holds the configuration settings for HTTP obfuscation.
type HTTPObfuscationConfig struct {
	// RemoveQueryStrings determines query strings to be removed from HTTP URLs.
	RemoveQueryString bool `mapstructure:"remove_query_string"`

	// RemovePathDigits determines digits in path segments to be obfuscated.
	RemovePathDigits bool `mapstructure:"remove_paths_with_digits"`
}

// Enablable can represent any option that has an "enabled" boolean sub-field.
type Enablable struct {
	Enabled bool `mapstructure:"enabled"`
}

// JSONObfuscationConfig holds the obfuscation configuration for sensitive
// data found in JSON objects.
type JSONObfuscationConfig struct {
	// Enabled will specify whether obfuscation should be enabled.
	Enabled bool `mapstructure:"enabled"`

	// KeepValues will specify a set of keys for which their values will
	// not be obfuscated.
	KeepValues []string `mapstructure:"keep_values"`
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

type traceWriter struct {
	MaxSpansPerPayload     int                    `mapstructure:"max_spans_per_payload"`
	FlushPeriod            float64                `mapstructure:"flush_period_seconds"`
	UpdateInfoPeriod       int                    `mapstructure:"update_info_period_seconds"`
	QueueablePayloadSender queueablePayloadSender `mapstructure:"queue"`
}

type serviceWriter struct {
	UpdateInfoPeriod       int                    `mapstructure:"update_info_period_seconds"`
	FlushPeriod            int                    `mapstructure:"flush_period_seconds"`
	QueueablePayloadSender queueablePayloadSender `mapstructure:"queue"`
}

type statsWriter struct {
	MaxEntriesPerPayload   int                    `mapstructure:"max_entries_per_payload"`
	UpdateInfoPeriod       int                    `mapstructure:"update_info_period_seconds"`
	QueueablePayloadSender queueablePayloadSender `mapstructure:"queue"`
}

type queueablePayloadSender struct {
	MaxAge            int   `mapstructure:"max_age_seconds"`
	MaxQueuedBytes    int64 `mapstructure:"max_bytes"`
	MaxQueuedPayloads int   `mapstructure:"max_payloads"`
	BackoffDuration   int   `mapstructure:"exp_backoff_max_duration_seconds"`
	BackoffBase       int   `mapstructure:"exp_backoff_base_milliseconds"`
	BackoffGrowth     int   `mapstructure:"exp_backoff_growth_base"`
}

func (c *AgentConfig) applyDatadogConfig() error {
	if len(c.Endpoints) == 0 {
		c.Endpoints = []*Endpoint{{}}
	}
	if config.Datadog.IsSet("api_key") {
		c.Endpoints[0].APIKey = config.Datadog.GetString("api_key")
	}
	if config.Datadog.IsSet("hostname") {
		c.Hostname = config.Datadog.GetString("hostname")
	}
	if config.Datadog.IsSet("log_level") {
		c.LogLevel = config.Datadog.GetString("log_level")
	}
	if config.Datadog.IsSet("dogstatsd_port") {
		c.StatsdPort = config.Datadog.GetInt("dogstatsd_port")
	}

	site := config.Datadog.GetString("site")
	if site != "" {
		c.Endpoints[0].Host = apiEndpointPrefix + site
	}
	if host := config.Datadog.GetString("apm_config.apm_sts_url"); host != "" {
		c.Endpoints[0].Host = host
		if site != "" {
			log.Infof("'site' and 'apm_sts_url' are both set, using endpoint: %q", host)
		}
	}
	for url, keys := range config.Datadog.GetStringMapStringSlice("apm_config.additional_endpoints") {
		if len(keys) == 0 {
			log.Errorf("'additional_endpoints' entries must have at least one API key present")
			continue
		}
		for _, key := range keys {
			c.Endpoints = append(c.Endpoints, &Endpoint{Host: url, APIKey: key})
		}
	}

	proxyList := config.Datadog.GetStringSlice("proxy.no_proxy")
	noProxy := make(map[string]bool, len(proxyList))
	for _, host := range proxyList {
		// map of hosts that need to be skipped by proxy
		noProxy[host] = true
	}
	for _, e := range c.Endpoints {
		e.NoProxy = noProxy[e.Host]
	}
	if addr := config.Datadog.GetString("proxy.https"); addr != "" {
		url, err := url.Parse(addr)
		if err == nil {
			c.ProxyURL = url
		} else {
			log.Errorf("Failed to parse proxy URL from proxy.https configuration: %s", err)
		}
	}

	if config.Datadog.IsSet("skip_ssl_validation") {
		c.SkipSSLValidation = config.Datadog.GetBool("skip_ssl_validation")
	}
	if config.Datadog.IsSet("apm_config.enabled") {
		c.Enabled = config.Datadog.GetBool("apm_config.enabled")
	}
	if config.Datadog.IsSet("apm_config.log_file") {
		c.LogFilePath = config.Datadog.GetString("apm_config.log_file")
	}
	if config.Datadog.IsSet("apm_config.env") {
		c.DefaultEnv = config.Datadog.GetString("apm_config.env")
	}
	if config.Datadog.IsSet("apm_config.receiver_port") {
		c.ReceiverPort = config.Datadog.GetInt("apm_config.receiver_port")
	}
	if config.Datadog.IsSet("apm_config.connection_limit") {
		c.ConnectionLimit = config.Datadog.GetInt("apm_config.connection_limit")
	}
	if config.Datadog.IsSet("apm_config.extra_sample_rate") {
		c.ExtraSampleRate = config.Datadog.GetFloat64("apm_config.extra_sample_rate")
	}
	if config.Datadog.IsSet("apm_config.max_events_per_second") {
		c.MaxEPS = config.Datadog.GetFloat64("apm_config.max_events_per_second")
	}
	if config.Datadog.IsSet("apm_config.max_traces_per_second") {
		c.MaxTPS = config.Datadog.GetFloat64("apm_config.max_traces_per_second")
	}
	if config.Datadog.IsSet("apm_config.ignore_resources") {
		c.Ignore["resource"] = config.Datadog.GetStringSlice("apm_config.ignore_resources")
	}

	if config.Datadog.IsSet("apm_config.replace_tags") {
		rt := make([]*ReplaceRule, 0)
		err := config.Datadog.UnmarshalKey("apm_config.replace_tags", &rt)
		if err == nil {
			err := compileReplaceRules(rt)
			if err != nil {
				osutil.Exitf("replace_tags: %s", err)
			}
			c.ReplaceTags = rt
		}
	}

	if config.Datadog.IsSet("bind_host") {
		host := config.Datadog.GetString("bind_host")
		c.StatsdHost = host
		c.ReceiverHost = host
	}
	if config.Datadog.IsSet("apm_config.apm_non_local_traffic") {
		if config.Datadog.GetBool("apm_config.apm_non_local_traffic") {
			c.ReceiverHost = "0.0.0.0"
		}
	}

	if config.Datadog.IsSet("apm_config.obfuscation") {
		var o ObfuscationConfig
		err := config.Datadog.UnmarshalKey("apm_config.obfuscation", &o)
		if err == nil {
			c.Obfuscation = &o
			if c.Obfuscation.RemoveStackTraces {
				c.addReplaceRule("error.stack", `(?s).*`, "?")
			}
		}
	}

	// undocumented
	if config.Datadog.IsSet("apm_config.max_cpu_percent") {
		c.MaxCPU = config.Datadog.GetFloat64("apm_config.max_cpu_percent") / 100
	}
	if config.Datadog.IsSet("apm_config.max_memory") {
		c.MaxMemory = config.Datadog.GetFloat64("apm_config.max_memory")
	}
	if config.Datadog.IsSet("apm_config.max_connections") {
		c.MaxConnections = config.Datadog.GetInt("apm_config.max_connections")
	}

	// undocumented
	c.ServiceWriterConfig = readServiceWriterConfigYaml()
	c.StatsWriterConfig = readStatsWriterConfigYaml()
	c.TraceWriterConfig = readTraceWriterConfigYaml()

	// undocumented deprecated
	if config.Datadog.IsSet("apm_config.analyzed_rate_by_service") {
		rateByService := make(map[string]float64)
		if err := config.Datadog.UnmarshalKey("apm_config.analyzed_rate_by_service", &rateByService); err != nil {
			return err
		}
		c.AnalyzedRateByServiceLegacy = rateByService
		if len(rateByService) > 0 {
			log.Warn("analyzed_rate_by_service is deprecated, please use analyzed_spans instead")
		}
	}
	// undocumeted
	if config.Datadog.IsSet("apm_config.analyzed_spans") {
		rateBySpan := make(map[string]float64)
		if err := config.Datadog.UnmarshalKey("apm_config.analyzed_spans", &rateBySpan); err != nil {
			return err
		}
		for key, rate := range rateBySpan {
			serviceName, operationName, err := parseServiceAndOp(key)
			if err != nil {
				log.Errorf("Error when parsing names", err)
				continue
			}

			if _, ok := c.AnalyzedSpansByService[serviceName]; !ok {
				c.AnalyzedSpansByService[serviceName] = make(map[string]float64)
			}
			c.AnalyzedSpansByService[serviceName][operationName] = rate
		}
	}

	// undocumented
	if config.Datadog.IsSet("apm_config.sts_agent_bin") {
		c.DDAgentBin = config.Datadog.GetString("apm_config.sts_agent_bin")
	}

	return c.loadDeprecatedValues()
}

// loadDeprecatedValues loads a set of deprecated values which are kept for
// backwards compatibility with Agent 5. These should eventually be removed.
// TODO(x): remove them gradually or fully in a future release.
func (c *AgentConfig) loadDeprecatedValues() error {
	cfg := config.Datadog
	if cfg.IsSet("apm_config.api_key") {
		c.Endpoints[0].APIKey = config.Datadog.GetString("apm_config.api_key")
	}
	if cfg.IsSet("apm_config.log_level") {
		c.LogLevel = config.Datadog.GetString("apm_config.log_level")
	}
	if v := cfg.GetString("apm_config.extra_aggregators"); len(v) > 0 {
		aggs, err := splitString(v, ',')
		if err != nil {
			return err
		}
		c.ExtraAggregators = append(c.ExtraAggregators, aggs...)
	}
	if !cfg.GetBool("apm_config.log_throttling") {
		c.LogThrottlingEnabled = false
	}
	if cfg.IsSet("apm_config.bucket_size_seconds") {
		d := time.Duration(cfg.GetInt("apm_config.bucket_size_seconds"))
		c.BucketInterval = d * time.Second
	}
	if cfg.IsSet("apm_config.receiver_timeout") {
		c.ReceiverTimeout = cfg.GetInt("apm_config.receiver_timeout")
	}
	if cfg.IsSet("apm_config.watchdog_check_delay") {
		d := time.Duration(cfg.GetInt("apm_config.watchdog_check_delay"))
		c.WatchdogInterval = d * time.Second
	}
	return nil
}

// addReplaceRule adds the specified replace rule to the agent configuration. If the pattern fails
// to compile as valid regexp, it exits the application with status code 1.
func (c *AgentConfig) addReplaceRule(tag, pattern, repl string) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		osutil.Exitf("error adding replace rule: %s", err)
	}
	c.ReplaceTags = append(c.ReplaceTags, &ReplaceRule{
		Name:    tag,
		Pattern: pattern,
		Re:      re,
		Repl:    repl,
	})
}

func readServiceWriterConfigYaml() writerconfig.ServiceWriterConfig {
	w := serviceWriter{}
	c := writerconfig.DefaultServiceWriterConfig()

	if err := config.Datadog.UnmarshalKey("apm_config.service_writer", &w); err == nil {
		if w.FlushPeriod > 0 {
			c.FlushPeriod = getDuration(w.FlushPeriod)
		}
		if w.UpdateInfoPeriod > 0 {
			c.UpdateInfoPeriod = getDuration(w.UpdateInfoPeriod)
		}
		c.SenderConfig = readQueueablePayloadSenderConfigYaml(w.QueueablePayloadSender)
	}
	return c
}

func readStatsWriterConfigYaml() writerconfig.StatsWriterConfig {
	w := statsWriter{}
	c := writerconfig.DefaultStatsWriterConfig()

	if err := config.Datadog.UnmarshalKey("apm_config.stats_writer", &w); err == nil {
		if w.MaxEntriesPerPayload > 0 {
			c.MaxEntriesPerPayload = w.MaxEntriesPerPayload
		}
		if w.UpdateInfoPeriod > 0 {
			c.UpdateInfoPeriod = getDuration(w.UpdateInfoPeriod)
		}
		c.SenderConfig = readQueueablePayloadSenderConfigYaml(w.QueueablePayloadSender)
	}
	return c
}

func readTraceWriterConfigYaml() writerconfig.TraceWriterConfig {
	w := traceWriter{}
	c := writerconfig.DefaultTraceWriterConfig()

	if err := config.Datadog.UnmarshalKey("apm_config.trace_writer", &w); err == nil {
		if w.MaxSpansPerPayload > 0 {
			c.MaxSpansPerPayload = w.MaxSpansPerPayload
		}
		if w.FlushPeriod > 0 {
			c.FlushPeriod = time.Duration(w.FlushPeriod*1000) * time.Millisecond
		}
		if w.UpdateInfoPeriod > 0 {
			c.UpdateInfoPeriod = getDuration(w.UpdateInfoPeriod)
		}
		c.SenderConfig = readQueueablePayloadSenderConfigYaml(w.QueueablePayloadSender)
	}
	return c
}

func readQueueablePayloadSenderConfigYaml(yc queueablePayloadSender) writerconfig.QueuablePayloadSenderConf {
	c := writerconfig.DefaultQueuablePayloadSenderConf()

	if yc.MaxAge != 0 {
		c.MaxAge = getDuration(yc.MaxAge)
	}

	if yc.MaxQueuedBytes != 0 {
		c.MaxQueuedBytes = yc.MaxQueuedBytes
	}

	if yc.MaxQueuedPayloads != 0 {
		c.MaxQueuedPayloads = yc.MaxQueuedPayloads
	}

	c.ExponentialBackoff = readExponentialBackoffConfigYaml(yc)

	return c
}

func readExponentialBackoffConfigYaml(yc queueablePayloadSender) backoff.ExponentialConfig {
	c := backoff.DefaultExponentialConfig()

	if yc.BackoffDuration > 0 {
		c.MaxDuration = getDuration(yc.BackoffDuration)
	}
	if yc.BackoffBase > 0 {
		c.Base = time.Duration(yc.BackoffBase) * time.Millisecond
	}
	if yc.BackoffGrowth > 0 {
		c.GrowthBase = yc.BackoffGrowth
	}

	return c
}

// compileReplaceRules compiles the regular expressions found in the replace rules.
// If it fails it returns the first error.
func compileReplaceRules(rules []*ReplaceRule) error {
	for _, r := range rules {
		if r.Name == "" {
			return errors.New(`all rules must have a "name" property (use "*" to target all)`)
		}
		if r.Pattern == "" {
			return errors.New(`all rules must have a "pattern"`)
		}
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return fmt.Errorf("key %q: %s", r.Name, err)
		}
		r.Re = re
	}
	return nil
}

// getDuration returns the duration of the provided value in seconds
func getDuration(seconds int) time.Duration {
	return time.Duration(seconds) * time.Second
}

func parseServiceAndOp(name string) (string, string, error) {
	splits := strings.Split(name, "|")
	if len(splits) != 2 {
		return "", "", fmt.Errorf("Bad format for operation name and service name in: %s, it should have format: service_name|operation_name", name)
	}
	return splits[0], splits[1], nil
}

func splitString(s string, sep rune) ([]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.TrimLeadingSpace = true
	r.LazyQuotes = true
	r.Comma = sep

	return r.Read()
}
