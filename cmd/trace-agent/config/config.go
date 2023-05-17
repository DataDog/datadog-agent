// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/internal/osutil"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/grpc"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// apiEndpointPrefix is the URL prefix prepended to the default site value from YamlAgentConfig.
	apiEndpointPrefix = "https://trace.agent."
	// rcClientName is the default name for remote configuration clients in the trace agent
	rcClientName = "trace-agent"
)

const (
	// rcClientPollInterval is the default poll interval for remote configuration clients. 1 second ensures that
	// clients remain up to date without paying too much of a performance cost (polls that contain no updates are cheap)
	rcClientPollInterval = time.Second * 1
)

// LoadConfigFile returns a new configuration based on the given path. The path must not necessarily exist
// and a valid configuration can be returned based on defaults and environment variables. If a
// valid configuration can not be obtained, an error is returned.
func LoadConfigFile(path string) (*config.AgentConfig, error) {
	cfg, err := prepareConfig(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		log.Infof("Loaded configuration: %s", cfg.ConfigPath)
	}
	if err := applyDatadogConfig(cfg); err != nil {
		log.Error(err)
	}
	return cfg, validate(cfg)
}

func prepareConfig(path string) (*config.AgentConfig, error) {
	cfg := config.New()
	cfg.LogFilePath = DefaultLogFilePath
	cfg.DDAgentBin = defaultDDAgentBin
	cfg.AgentVersion = version.AgentVersion
	cfg.GitCommit = version.Commit
	coreconfig.Datadog.SetConfigFile(path)
	if _, err := coreconfig.Load(); err != nil {
		return cfg, err
	}
	orch := fargate.GetOrchestrator() // Needs to be after loading config, because it relies on feature auto-detection
	cfg.FargateOrchestrator = config.FargateOrchestratorName(orch)
	if p := coreconfig.Datadog.GetProxies(); p != nil {
		cfg.Proxy = httputils.GetProxyTransportFunc(p)
	}
	cfg.ConfigPath = path
	if coreconfig.Datadog.GetBool("remote_configuration.enabled") && coreconfig.Datadog.GetBool("remote_configuration.apm_sampling.enabled") {
		client, err := remote.NewGRPCClient(rcClientName, version.AgentVersion, []data.Product{data.ProductAPMSampling}, rcClientPollInterval)
		if err != nil {
			log.Errorf("Error when subscribing to remote config management %v", err)
		} else {
			cfg.RemoteSamplingClient = client
		}
	}
	cfg.ContainerTags = containerTagsFunc
	cfg.ContainerProcRoot = coreconfig.Datadog.GetString("container_proc_root")
	return cfg, nil
}

func containerTagsFunc(cid string) ([]string, error) {
	return tagger.Tag("container_id://"+cid, collectors.HighCardinality)
}

// appendEndpoints appends any endpoint configuration found at the given cfgKey.
// The format for cfgKey should be a map which has the URL as a key and one or
// more API keys as an array value.
func appendEndpoints(endpoints []*config.Endpoint, cfgKey string) []*config.Endpoint {
	if !coreconfig.Datadog.IsSet(cfgKey) {
		return endpoints
	}
	for url, keys := range coreconfig.Datadog.GetStringMapStringSlice(cfgKey) {
		if len(keys) == 0 {
			log.Errorf("'%s' entries must have at least one API key present", cfgKey)
			continue
		}
		for _, key := range keys {
			endpoints = append(endpoints, &config.Endpoint{Host: url, APIKey: coreconfig.SanitizeAPIKey(key)})
		}
	}
	return endpoints
}

func applyDatadogConfig(c *config.AgentConfig) error {
	if len(c.Endpoints) == 0 {
		c.Endpoints = []*config.Endpoint{{}}
	}
	if coreconfig.Datadog.IsSet("api_key") {
		c.Endpoints[0].APIKey = coreconfig.SanitizeAPIKey(coreconfig.Datadog.GetString("api_key"))
	}
	if coreconfig.Datadog.IsSet("hostname") {
		c.Hostname = coreconfig.Datadog.GetString("hostname")
	}
	if coreconfig.Datadog.IsSet("dogstatsd_port") {
		c.StatsdPort = coreconfig.Datadog.GetInt("dogstatsd_port")
	}

	obsPipelineEnabled, prefix := isObsPipelineEnabled()
	if obsPipelineEnabled {
		if host := coreconfig.Datadog.GetString(fmt.Sprintf("%s.traces.url", prefix)); host == "" {
			log.Errorf("%s.traces.enabled but %s.traces.url is empty.", prefix, prefix)
		} else {
			c.Endpoints[0].Host = host
		}
	} else {
		c.Endpoints[0].Host = utils.GetMainEndpoint(coreconfig.Datadog, apiEndpointPrefix, "apm_config.apm_dd_url")
	}
	c.Endpoints = appendEndpoints(c.Endpoints, "apm_config.additional_endpoints")

	if coreconfig.Datadog.IsSet("proxy.no_proxy") {
		proxyList := coreconfig.Datadog.GetStringSlice("proxy.no_proxy")
		noProxy := make(map[string]bool, len(proxyList))
		for _, host := range proxyList {
			// map of hosts that need to be skipped by proxy
			noProxy[host] = true
		}
		for _, e := range c.Endpoints {
			e.NoProxy = noProxy[e.Host]
		}
	}
	if addr := coreconfig.Datadog.GetString("proxy.https"); addr != "" {
		url, err := url.Parse(addr)
		if err == nil {
			c.ProxyURL = url
		} else {
			log.Errorf("Failed to parse proxy URL from proxy.https configuration: %s", err)
		}
	}

	if coreconfig.Datadog.IsSet("skip_ssl_validation") {
		c.SkipSSLValidation = coreconfig.Datadog.GetBool("skip_ssl_validation")
	}
	if coreconfig.Datadog.IsSet("apm_config.enabled") {
		c.Enabled = coreconfig.Datadog.GetBool("apm_config.enabled")
	}
	if coreconfig.Datadog.IsSet("apm_config.log_file") {
		c.LogFilePath = coreconfig.Datadog.GetString("apm_config.log_file")
	}

	if env := coreconfig.GetTraceAgentDefaultEnv(); env != "" {
		c.DefaultEnv = env
	}

	prevEnv := c.DefaultEnv
	c.DefaultEnv = traceutil.NormalizeTag(c.DefaultEnv)
	if c.DefaultEnv != prevEnv {
		log.Debugf("Normalized DefaultEnv from %q to %q", prevEnv, c.DefaultEnv)
	}
	if coreconfig.Datadog.IsSet("apm_config.receiver_port") {
		c.ReceiverPort = coreconfig.Datadog.GetInt("apm_config.receiver_port")
	}
	if coreconfig.Datadog.IsSet("apm_config.receiver_socket") {
		c.ReceiverSocket = coreconfig.Datadog.GetString("apm_config.receiver_socket")
	}
	if coreconfig.Datadog.IsSet("apm_config.connection_limit") {
		c.ConnectionLimit = coreconfig.Datadog.GetInt("apm_config.connection_limit")
	}
	c.PeerServiceAggregation = coreconfig.Datadog.GetBool("apm_config.peer_service_aggregation")
	c.ComputeStatsBySpanKind = coreconfig.Datadog.GetBool("apm_config.compute_stats_by_span_kind")
	if coreconfig.Datadog.IsSet("apm_config.extra_sample_rate") {
		c.ExtraSampleRate = coreconfig.Datadog.GetFloat64("apm_config.extra_sample_rate")
	}
	if coreconfig.Datadog.IsSet("apm_config.max_events_per_second") {
		c.MaxEPS = coreconfig.Datadog.GetFloat64("apm_config.max_events_per_second")
	}
	if coreconfig.Datadog.IsSet("apm_config.max_traces_per_second") {
		c.TargetTPS = coreconfig.Datadog.GetFloat64("apm_config.max_traces_per_second")
	}
	if coreconfig.Datadog.IsSet("apm_config.errors_per_second") {
		c.ErrorTPS = coreconfig.Datadog.GetFloat64("apm_config.errors_per_second")
	}
	if coreconfig.Datadog.IsSet("apm_config.enable_rare_sampler") {
		c.RareSamplerEnabled = coreconfig.Datadog.GetBool("apm_config.enable_rare_sampler")
	}
	if coreconfig.Datadog.IsSet("apm_config.rare_sampler.tps") {
		c.RareSamplerTPS = coreconfig.Datadog.GetInt("apm_config.rare_sampler.tps")
	}
	if coreconfig.Datadog.IsSet("apm_config.rare_sampler.cooldown") {
		c.RareSamplerCooldownPeriod = coreconfig.Datadog.GetDuration("apm_config.rare_sampler.cooldown")
	}
	if coreconfig.Datadog.IsSet("apm_config.rare_sampler.cardinality") {
		c.RareSamplerCardinality = coreconfig.Datadog.GetInt("apm_config.rare_sampler.cardinality")
	}

	if coreconfig.Datadog.IsSet("apm_config.max_remote_traces_per_second") {
		c.MaxRemoteTPS = coreconfig.Datadog.GetFloat64("apm_config.max_remote_traces_per_second")
	}
	if k := "apm_config.features"; coreconfig.Datadog.IsSet(k) {
		feats := coreconfig.Datadog.GetStringSlice(k)
		for _, f := range feats {
			c.Features[f] = struct{}{}
		}
		if c.HasFeature("big_resource") {
			c.MaxResourceLen = 15_000
		}
		log.Debug("Found APM feature flags: %v", c.Features)
	}

	if k := "apm_config.ignore_resources"; coreconfig.Datadog.IsSet(k) {
		c.Ignore["resource"] = coreconfig.Datadog.GetStringSlice(k)
	}
	if k := "apm_config.max_payload_size"; coreconfig.Datadog.IsSet(k) {
		c.MaxRequestBytes = coreconfig.Datadog.GetInt64(k)
	}
	if k := "apm_config.replace_tags"; coreconfig.Datadog.IsSet(k) {
		rt := make([]*config.ReplaceRule, 0)
		if err := coreconfig.Datadog.UnmarshalKey(k, &rt); err != nil {
			log.Errorf("Bad format for %q it should be of the form '[{\"name\": \"tag_name\",\"pattern\":\"pattern\",\"repl\":\"replace_str\"}]', error: %v", "apm_config.replace_tags", err)
		} else {
			err := compileReplaceRules(rt)
			if err != nil {
				osutil.Exitf("replace_tags: %s", err)
			}
			c.ReplaceTags = rt
		}
	}

	if coreconfig.Datadog.IsSet("bind_host") || coreconfig.Datadog.IsSet("apm_config.apm_non_local_traffic") {
		if coreconfig.Datadog.IsSet("bind_host") {
			host := coreconfig.Datadog.GetString("bind_host")
			c.StatsdHost = host
			c.ReceiverHost = host
		}

		if coreconfig.Datadog.IsSet("apm_config.apm_non_local_traffic") && coreconfig.Datadog.GetBool("apm_config.apm_non_local_traffic") {
			c.ReceiverHost = "0.0.0.0"
		}
	} else if coreconfig.IsContainerized() {
		// Automatically activate non local traffic in containerized environment if no explicit config set
		log.Info("Activating non-local traffic automatically in containerized environment, trace-agent will listen on 0.0.0.0")
		c.ReceiverHost = "0.0.0.0"
	}
	c.StatsdPipeName = coreconfig.Datadog.GetString("dogstatsd_pipe_name")
	c.StatsdSocket = coreconfig.Datadog.GetString("dogstatsd_socket")
	c.WindowsPipeName = coreconfig.Datadog.GetString("apm_config.windows_pipe_name")
	c.PipeBufferSize = coreconfig.Datadog.GetInt("apm_config.windows_pipe_buffer_size")
	c.PipeSecurityDescriptor = coreconfig.Datadog.GetString("apm_config.windows_pipe_security_descriptor")
	c.GUIPort = coreconfig.Datadog.GetString("GUI_port")

	var grpcPort int
	if otlp.IsEnabled(coreconfig.Datadog) {
		grpcPort = coreconfig.Datadog.GetInt(coreconfig.OTLPTracePort)
	}
	c.OTLPReceiver = &config.OTLP{
		BindHost:               c.ReceiverHost,
		GRPCPort:               grpcPort,
		MaxRequestBytes:        c.MaxRequestBytes,
		SpanNameRemappings:     coreconfig.Datadog.GetStringMapString("otlp_config.traces.span_name_remappings"),
		SpanNameAsResourceName: coreconfig.Datadog.GetBool("otlp_config.traces.span_name_as_resource_name"),
		ProbabilisticSampling:  coreconfig.Datadog.GetFloat64("otlp_config.traces.probabilistic_sampler.sampling_percentage"),
	}

	if coreconfig.Datadog.GetBool("apm_config.telemetry.enabled") {
		c.TelemetryConfig.Enabled = true
		c.TelemetryConfig.Endpoints = []*config.Endpoint{{
			Host:   utils.GetMainEndpoint(coreconfig.Datadog, config.TelemetryEndpointPrefix, "apm_config.telemetry.dd_url"),
			APIKey: c.Endpoints[0].APIKey,
		}}
		c.TelemetryConfig.Endpoints = appendEndpoints(c.TelemetryConfig.Endpoints, "apm_config.telemetry.additional_endpoints")
	}
	c.Obfuscation = new(config.ObfuscationConfig)
	if coreconfig.Datadog.IsSet("apm_config.obfuscation") {
		var o config.ObfuscationConfig
		err := coreconfig.Datadog.UnmarshalKey("apm_config.obfuscation", &o)
		if err == nil {
			c.Obfuscation = &o
			if o.RemoveStackTraces {
				addReplaceRule(c, "error.stack", `(?s).*`, "?")
			}
		}
	}
	{
		// TODO(x): There is an issue with coreconfig.Datadog.IsSet("apm_config.obfuscation"), probably coming from Viper,
		// where it returns false even is "apm_config.obfuscation.credit_cards.enabled" is set via an environment
		// variable, so we need a temporary workaround by specifically setting env. var. accessible fields.
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.credit_cards.enabled") {
			c.Obfuscation.CreditCards.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.credit_cards.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.credit_cards.luhn") {
			c.Obfuscation.CreditCards.Luhn = coreconfig.Datadog.GetBool("apm_config.obfuscation.credit_cards.luhn")
		}
	}

	if coreconfig.Datadog.IsSet("apm_config.filter_tags.require") {
		tags := coreconfig.Datadog.GetStringSlice("apm_config.filter_tags.require")
		for _, tag := range tags {
			c.RequireTags = append(c.RequireTags, splitTag(tag))
		}
	}
	if coreconfig.Datadog.IsSet("apm_config.filter_tags.reject") {
		tags := coreconfig.Datadog.GetStringSlice("apm_config.filter_tags.reject")
		for _, tag := range tags {
			c.RejectTags = append(c.RejectTags, splitTag(tag))
		}
	}

	setMaxMemCPU(c, coreconfig.IsContainerized())

	// undocumented writers
	for key, cfg := range map[string]*config.WriterConfig{
		"apm_config.trace_writer": c.TraceWriter,
		"apm_config.stats_writer": c.StatsWriter,
	} {
		if err := coreconfig.Datadog.UnmarshalKey(key, cfg); err != nil {
			log.Errorf("Error reading writer config %q: %v", key, err)
		}
	}
	if coreconfig.Datadog.IsSet("apm_config.connection_reset_interval") {
		c.ConnectionResetInterval = getDuration(coreconfig.Datadog.GetInt("apm_config.connection_reset_interval"))
	}
	if coreconfig.Datadog.IsSet("apm_config.sync_flushing") {
		c.SynchronousFlushing = coreconfig.Datadog.GetBool("apm_config.sync_flushing")
	}

	// undocumented deprecated
	if coreconfig.Datadog.IsSet("apm_config.analyzed_rate_by_service") {
		rateByService := make(map[string]float64)
		if err := coreconfig.Datadog.UnmarshalKey("apm_config.analyzed_rate_by_service", &rateByService); err != nil {
			return err
		}
		c.AnalyzedRateByServiceLegacy = rateByService
		if len(rateByService) > 0 {
			log.Warn("analyzed_rate_by_service is deprecated, please use analyzed_spans instead")
		}
	}
	// undocumented
	if k := "apm_config.analyzed_spans"; coreconfig.Datadog.IsSet(k) {
		for key, rate := range coreconfig.Datadog.GetStringMap("apm_config.analyzed_spans") {
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
	if coreconfig.Datadog.IsSet("apm_config.dd_agent_bin") {
		c.DDAgentBin = coreconfig.Datadog.GetString("apm_config.dd_agent_bin")
	}

	if err := loadDeprecatedValues(c); err != nil {
		return err
	}

	if strings.ToLower(coreconfig.Datadog.GetString("log_level")) == "debug" && !coreconfig.Datadog.IsSet("apm_config.log_throttling") {
		// if we are in "debug mode" and log throttling behavior was not
		// set by the user, disable it
		c.LogThrottling = false
	}
	c.Site = coreconfig.Datadog.GetString("site")
	if c.Site == "" {
		c.Site = coreconfig.DefaultSite
	}
	if k := "use_dogstatsd"; coreconfig.Datadog.IsSet(k) {
		c.StatsdEnabled = coreconfig.Datadog.GetBool(k)
	}
	if v := coreconfig.Datadog.GetInt("apm_config.max_catalog_entries"); v > 0 {
		c.MaxCatalogEntries = v
	}
	if k := "apm_config.profiling_dd_url"; coreconfig.Datadog.IsSet(k) {
		c.ProfilingProxy.DDURL = coreconfig.Datadog.GetString(k)
	}
	if k := "apm_config.profiling_additional_endpoints"; coreconfig.Datadog.IsSet(k) {
		c.ProfilingProxy.AdditionalEndpoints = coreconfig.Datadog.GetStringMapStringSlice(k)
	}
	if k := "apm_config.debugger_dd_url"; coreconfig.Datadog.IsSet(k) {
		c.DebuggerProxy.DDURL = coreconfig.Datadog.GetString(k)
	}
	if k := "apm_config.debugger_api_key"; coreconfig.Datadog.IsSet(k) {
		c.DebuggerProxy.APIKey = coreconfig.Datadog.GetString(k)
	}
	if k := "apm_config.debugger_additional_endpoints"; coreconfig.Datadog.IsSet(k) {
		c.DebuggerProxy.AdditionalEndpoints = coreconfig.Datadog.GetStringMapStringSlice(k)
	}
	if k := "apm_config.symdb_dd_url"; coreconfig.Datadog.IsSet(k) {
		c.SymDBProxy.DDURL = coreconfig.Datadog.GetString(k)
	}
	if k := "apm_config.symdb_api_key"; coreconfig.Datadog.IsSet(k) {
		c.SymDBProxy.APIKey = coreconfig.Datadog.GetString(k)
	}
	if k := "apm_config.symdb_additional_endpoints"; coreconfig.Datadog.IsSet(k) {
		c.SymDBProxy.AdditionalEndpoints = coreconfig.Datadog.GetStringMapStringSlice(k)
	}
	if k := "evp_proxy_config.enabled"; coreconfig.Datadog.IsSet(k) {
		c.EVPProxy.Enabled = coreconfig.Datadog.GetBool(k)
	}
	if k := "evp_proxy_config.dd_url"; coreconfig.Datadog.IsSet(k) {
		c.EVPProxy.DDURL = coreconfig.Datadog.GetString(k)
	}
	if k := "evp_proxy_config.api_key"; coreconfig.Datadog.IsSet(k) {
		c.EVPProxy.APIKey = coreconfig.Datadog.GetString(k)
	}
	if k := "evp_proxy_config.app_key"; coreconfig.Datadog.IsSet(k) {
		c.EVPProxy.ApplicationKey = coreconfig.Datadog.GetString(k)
	} else {
		// Default to the agent-wide app_key if set
		c.EVPProxy.ApplicationKey = coreconfig.Datadog.GetString("app_key")
	}
	if k := "evp_proxy_config.additional_endpoints"; coreconfig.Datadog.IsSet(k) {
		c.EVPProxy.AdditionalEndpoints = coreconfig.Datadog.GetStringMapStringSlice(k)
	}
	if k := "evp_proxy_config.max_payload_size"; coreconfig.Datadog.IsSet(k) {
		c.EVPProxy.MaxPayloadSize = coreconfig.Datadog.GetInt64(k)
	}
	c.DebugServerPort = coreconfig.Datadog.GetInt("apm_config.debug.port")
	return nil
}

// loadDeprecatedValues loads a set of deprecated values which are kept for
// backwards compatibility with Agent 5. These should eventually be removed.
// TODO(x): remove them gradually or fully in a future release.
func loadDeprecatedValues(c *config.AgentConfig) error {
	cfg := coreconfig.Datadog
	if cfg.IsSet("apm_config.api_key") {
		c.Endpoints[0].APIKey = coreconfig.SanitizeAPIKey(coreconfig.Datadog.GetString("apm_config.api_key"))
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
	if cfg.IsSet("apm_config.disable_rare_sampler") {
		log.Warn("apm_config.disable_rare_sampler/DD_APM_DISABLE_RARE_SAMPLER is deprecated and the rare sampler is now disabled by default. To enable the rare sampler use apm_config.enable_rare_sampler or DD_APM_ENABLE_RARE_SAMPLER")
	}
	return nil
}

// addReplaceRule adds the specified replace rule to the agent configuration. If the pattern fails
// to compile as valid regexp, it exits the application with status code 1.
func addReplaceRule(c *config.AgentConfig, tag, pattern, repl string) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		osutil.Exitf("error adding replace rule: %s", err)
	}
	c.ReplaceTags = append(c.ReplaceTags, &config.ReplaceRule{
		Name:    tag,
		Pattern: pattern,
		Re:      re,
		Repl:    repl,
	})
}

// compileReplaceRules compiles the regular expressions found in the replace rules.
// If it fails it returns the first error.
func compileReplaceRules(rules []*config.ReplaceRule) error {
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

// splitTag splits a "k:v" formatted string and returns a Tag.
func splitTag(tag string) *config.Tag {
	parts := strings.SplitN(tag, ":", 2)
	kv := &config.Tag{
		K: strings.TrimSpace(parts[0]),
	}
	if len(parts) > 1 {
		if v := strings.TrimSpace(parts[1]); v != "" {
			kv.V = v
		}
	}
	return kv
}

// validate validates if the current configuration is good for the agent to start with.
func validate(c *config.AgentConfig) error {
	if len(c.Endpoints) == 0 || c.Endpoints[0].APIKey == "" {
		return config.ErrMissingAPIKey
	}
	if c.DDAgentBin == "" {
		return errors.New("agent binary path not set")
	}
	if c.Hostname == "" && !coreconfig.Datadog.GetBool("serverless.enabled") {
		// no user-set hostname, try to acquire
		if err := acquireHostname(c); err != nil {
			log.Debugf("Could not get hostname via gRPC: %v. Falling back to other methods.", err)
			if err := acquireHostnameFallback(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// fallbackHostnameFunc specifies the function to use for obtaining the hostname
// when it can not be obtained by any other means. It is replaced in tests.
var fallbackHostnameFunc = os.Hostname

// acquireHostname attempts to acquire a hostname for the trace-agent by connecting to the core agent's
// gRPC endpoints. If it fails, it will return an error.
func acquireHostname(c *config.AgentConfig) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := grpc.GetDDAgentClient(ctx)
	if err != nil {
		return err
	}
	reply, err := client.GetHostname(ctx, &pbgo.HostnameRequest{})
	if err != nil {
		return err
	}
	if c.HasFeature("disable_empty_hostname") && reply.Hostname == "" {
		log.Debugf("Acquired empty hostname from gRPC but it's disallowed.")
		return errors.New("empty hostname disallowed")
	}
	c.Hostname = reply.Hostname
	log.Debugf("Acquired hostname from gRPC: %s", c.Hostname)
	return nil
}

// acquireHostnameFallback attempts to acquire a hostname for this configuration. It
// tries to shell out to the infrastructure agent for this, if DD_AGENT_BIN is
// set, otherwise falling back to os.Hostname.
func acquireHostnameFallback(c *config.AgentConfig) error {
	var out bytes.Buffer
	cmd := exec.Command(c.DDAgentBin, "hostname")
	cmd.Env = append(os.Environ(), cmd.Env...) // needed for Windows
	cmd.Stdout = &out
	err := cmd.Run()
	c.Hostname = strings.TrimSpace(out.String())
	if emptyDisallowed := c.HasFeature("disable_empty_hostname") && c.Hostname == ""; err != nil || emptyDisallowed {
		if emptyDisallowed {
			log.Debugf("Core agent returned empty hostname but is disallowed by disable_empty_hostname feature flag. Falling back to os.Hostname.")
		}
		// There was either an error retrieving the hostname from the core agent, or
		// it was empty and its disallowed by the disable_empty_hostname feature flag.
		host, err2 := fallbackHostnameFunc()
		if err2 != nil {
			return fmt.Errorf("couldn't get hostname from agent (%q), nor from OS (%q). Try specifying it by means of config or the DD_HOSTNAME env var", err, err2)
		}
		if emptyDisallowed && host == "" {
			return errors.New("empty hostname disallowed")
		}
		c.Hostname = host
		log.Debugf("Acquired hostname from OS: %q. Core agent was unreachable at %q: %v.", c.Hostname, c.DDAgentBin, err)
		return nil
	}
	log.Debugf("Acquired hostname from core agent (%s): %q.", c.DDAgentBin, c.Hostname)
	return nil
}

// SetHandler returns handler for runtime configuration changes.
func SetHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			httpError(w, http.StatusMethodNotAllowed, fmt.Errorf("%s method not allowed, only %s", req.Method, http.MethodPost))
			return
		}
		for key, values := range req.URL.Query() {
			if len(values) == 0 {
				continue
			}
			value := html.UnescapeString(values[len(values)-1])
			switch key {
			case "log_level":
				lvl := strings.ToLower(value)
				if lvl == "warning" {
					lvl = "warn"
				}
				if err := coreconfig.ChangeLogLevel(lvl); err != nil {
					httpError(w, http.StatusInternalServerError, err)
					return
				}
				coreconfig.Datadog.Set("log_level", lvl)
				log.Infof("Switched log level to %s", lvl)
			default:
				log.Infof("Unsupported config change requested (key: %q).", key)
			}
		}
	})
}

func httpError(w http.ResponseWriter, status int, err error) {
	http.Error(w, fmt.Sprintf(`{"error": %q}`, err.Error()), status)
}

// setMaxMemCPU sets watchdog's max_memory and max_cpu_percent parameters.
// If the agent is containerized, max_memory and max_cpu_percent are disabled by default.
// Resource limits are better handled by container runtimes and orchestrators.
func setMaxMemCPU(c *config.AgentConfig, isContainerized bool) {
	if coreconfig.Datadog.IsSet("apm_config.max_cpu_percent") {
		c.MaxCPU = coreconfig.Datadog.GetFloat64("apm_config.max_cpu_percent") / 100
	} else if isContainerized {
		log.Debug("Running in a container and apm_config.max_cpu_percent is not set, setting it to 0")
		c.MaxCPU = 0
	}

	if coreconfig.Datadog.IsSet("apm_config.max_memory") {
		c.MaxMemory = coreconfig.Datadog.GetFloat64("apm_config.max_memory")
	} else if isContainerized {
		log.Debug("Running in a container and apm_config.max_memory is not set, setting it to 0")
		c.MaxMemory = 0
	}
}

func isObsPipelineEnabled() (bool, string) {
	if coreconfig.Datadog.GetBool("observability_pipelines_worker.traces.enabled") {
		return true, "observability_pipelines_worker"
	}
	if coreconfig.Datadog.GetBool("vector.traces.enabled") {
		return true, "vector"
	}
	return false, "observability_pipelines_worker"
}
