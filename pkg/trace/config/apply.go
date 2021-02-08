// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/davecgh/go-spew/spew"
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

	// SQLExecPlan holds the obfuscation configuration for SQL Exec Plans. This is strictly for safety related obfuscation,
	// not normalization. Normalization of exec plans is configured in SQLExecPlanNormalize.
	SQLExecPlan JSONObfuscationConfig `mapstructure:"sql_exec_plan"`

	// SQLExecPlanNormalize holds the normalization configuration for SQL Exec Plans.
	SQLExecPlanNormalize JSONObfuscationConfig `mapstructure:"sql_exec_plan_normalize"`

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

	// ObfuscateSQLValues will specify a set of keys for which their values
	// will be passed through SQL obfuscation
	ObfuscateSQLValues []string `mapstructure:"obfuscate_sql_values"`
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
	if host := config.Datadog.GetString("apm_config.apm_dd_url"); host != "" {
		c.Endpoints[0].Host = host
		if site != "" {
			log.Infof("'site' and 'apm_dd_url' are both set, using endpoint: %q", host)
		}
	}
	if config.Datadog.IsSet("apm_config.additional_endpoints") {
		for url, keys := range config.Datadog.GetStringMapStringSlice("apm_config.additional_endpoints") {
			if len(keys) == 0 {
				log.Errorf("'additional_endpoints' entries must have at least one API key present")
				continue
			}
			for _, key := range keys {
				key = config.SanitizeAPIKey(key)
				c.Endpoints = append(c.Endpoints, &Endpoint{Host: url, APIKey: key})
			}
		}
	}

	if config.Datadog.IsSet("proxy.no_proxy") {
		proxyList := config.Datadog.GetStringSlice("proxy.no_proxy")
		noProxy := make(map[string]bool, len(proxyList))
		for _, host := range proxyList {
			// map of hosts that need to be skipped by proxy
			noProxy[host] = true
		}
		for _, e := range c.Endpoints {
			e.NoProxy = noProxy[e.Host]
		}
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
		log.Debugf("Setting DefaultEnv to %q (from apm_config.env)", c.DefaultEnv)
	} else if config.Datadog.IsSet("env") {
		c.DefaultEnv = config.Datadog.GetString("env")
		log.Debugf("Setting DefaultEnv to %q (from 'env' config option)", c.DefaultEnv)
	} else if config.Datadog.IsSet("tags") {
		for _, tag := range config.Datadog.GetStringSlice("tags") {
			if strings.HasPrefix(tag, "env:") {
				c.DefaultEnv = strings.TrimPrefix(tag, "env:")
				log.Debugf("Setting DefaultEnv to %q (from `env:` entry under the 'tags' config option: %q)", c.DefaultEnv, tag)
				break
			}
		}
	}
	if config.Datadog.IsSet("apm_config.receiver_port") {
		c.ReceiverPort = config.Datadog.GetInt("apm_config.receiver_port")
	}
	if config.Datadog.IsSet("apm_config.receiver_socket") {
		c.ReceiverSocket = config.Datadog.GetString("apm_config.receiver_socket")
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
		c.TargetTPS = config.Datadog.GetFloat64("apm_config.max_traces_per_second")
	}
	if k := "apm_config.ignore_resources"; config.Datadog.IsSet(k) {
		c.Ignore["resource"] = config.Datadog.GetStringSlice(k)
	}
	if k := "apm_config.max_payload_size"; config.Datadog.IsSet(k) {
		c.MaxRequestBytes = config.Datadog.GetInt64(k)
	}
	if k := "apm_config.replace_tags"; config.Datadog.IsSet(k) {
		rt := make([]*ReplaceRule, 0)
		if err := config.Datadog.UnmarshalKey(k, &rt); err != nil {
			log.Errorf("Bad format for %q it should be of the form '[{\"name\": \"tag_name\",\"pattern\":\"pattern\",\"repl\":\"replace_str\"}]', error: %v", "apm_config.replace_tags", err)
		} else {
			err := compileReplaceRules(rt)
			if err != nil {
				osutil.Exitf("replace_tags: %s", err)
			}
			c.ReplaceTags = rt
		}
	}

	if config.Datadog.IsSet("bind_host") || config.Datadog.IsSet("apm_config.apm_non_local_traffic") {
		if config.Datadog.IsSet("bind_host") {
			host := config.Datadog.GetString("bind_host")
			c.StatsdHost = host
			c.ReceiverHost = host
		}

		if config.Datadog.IsSet("apm_config.apm_non_local_traffic") && config.Datadog.GetBool("apm_config.apm_non_local_traffic") {
			c.ReceiverHost = "0.0.0.0"
		}
	} else if config.IsContainerized() {
		// Automatically activate non local traffic in containerized environment if no explicit config set
		log.Info("Activating non-local traffic automatically in containerized environment, trace-agent will listen on 0.0.0.0")
		c.ReceiverHost = "0.0.0.0"
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

	if config.Datadog.IsSet("apm_config.filter_tags.require") {
		tags := config.Datadog.GetStringSlice("apm_config.filter_tags.require")
		for _, tag := range tags {
			c.RequireTags = append(c.RequireTags, splitTag(tag))
		}
	}

	if config.Datadog.IsSet("apm_config.filter_tags.reject") {
		tags := config.Datadog.GetStringSlice("apm_config.filter_tags.reject")
		for _, tag := range tags {
			c.RejectTags = append(c.RejectTags, splitTag(tag))
			spew.Dump(c.RejectTags)
		}
	}

	// undocumented
	if config.Datadog.IsSet("apm_config.max_cpu_percent") {
		c.MaxCPU = config.Datadog.GetFloat64("apm_config.max_cpu_percent") / 100
	}
	if config.Datadog.IsSet("apm_config.max_memory") {
		c.MaxMemory = config.Datadog.GetFloat64("apm_config.max_memory")
	}

	// undocumented writers
	for key, cfg := range map[string]*WriterConfig{
		"apm_config.trace_writer": c.TraceWriter,
		"apm_config.stats_writer": c.StatsWriter,
	} {
		if err := config.Datadog.UnmarshalKey(key, cfg); err != nil {
			log.Errorf("Error reading writer config %q: %v", key, err)
		}
	}
	if config.Datadog.IsSet("apm_config.connection_reset_interval") {
		c.ConnectionResetInterval = getDuration(config.Datadog.GetInt("apm_config.connection_reset_interval"))
	}

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
	if k := "apm_config.analyzed_spans"; config.Datadog.IsSet(k) {
		for key, rate := range config.Datadog.GetStringMap("apm_config.analyzed_spans") {
			serviceName, operationName, err := parseServiceAndOp(key)
			if err != nil {
				log.Errorf("Error parsing names: %v", err)
				continue
			}
			if floatrate, err := toFloat64(rate); err != nil {
				log.Errorf("Invalid value for apm_config.analyzed_spans: %v", err)
			} else {
				if _, ok := c.AnalyzedSpansByService[serviceName]; !ok {
					c.AnalyzedSpansByService[serviceName] = make(map[string]float64)
				}
				c.AnalyzedSpansByService[serviceName][operationName] = floatrate
			}
		}
	}

	// undocumented
	if config.Datadog.IsSet("apm_config.dd_agent_bin") {
		c.DDAgentBin = config.Datadog.GetString("apm_config.dd_agent_bin")
	}

	if err := c.loadDeprecatedValues(); err != nil {
		return err
	}

	if strings.ToLower(c.LogLevel) == "debug" && !config.Datadog.IsSet("apm_config.log_throttling") {
		// if we are in "debug mode" and log throttling behavior was not
		// set by the user, disable it
		c.LogThrottling = false
	}

	return nil
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
	if cfg.IsSet("apm_config.log_throttling") {
		c.LogThrottling = cfg.GetBool("apm_config.log_throttling")
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

func toFloat64(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, err
		}
		return f, nil
	default:
		return 0, fmt.Errorf("%v can not be converted to float64", val)
	}
}

func splitTag(tag string) *Tag {
	parts := strings.SplitN(tag, ":", 2)
	kv := &Tag{
		K: strings.TrimSpace(parts[0]),
	}
	if len(parts) > 1 {
		if v := strings.TrimSpace(parts[1]); v != "" {
			kv.V = v
		}
	}
	return kv
}
