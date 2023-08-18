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

	corecompcfg "github.com/DataDog/datadog-agent/comp/core/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/otlp"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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

// team: agent-apm

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

func setupConfigCommon(deps dependencies, apikey string) (*config.AgentConfig, error) {
	confFilePath := deps.Config.ConfigFileUsed()

	return LoadConfigFile(confFilePath, deps.Config)
}

// LoadConfigFile returns a new configuration based on the given path. The path must not necessarily exist
// and a valid configuration can be returned based on defaults and environment variables. If a
// valid configuration can not be obtained, an error is returned.
func LoadConfigFile(path string, c corecompcfg.Component) (*config.AgentConfig, error) {
	cfg, err := prepareConfig(c)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		cfg.ConfigPath = path
		log.Infof("Loaded configuration: %s", cfg.ConfigPath)
	}

	if err := applyDatadogConfig(cfg, c); err != nil {
		log.Error(err)
	}

	return cfg, validate(cfg, c)
}

func prepareConfig(c corecompcfg.Component) (*config.AgentConfig, error) {
	cfg := config.New()
	cfg.DDAgentBin = defaultDDAgentBin
	cfg.AgentVersion = version.AgentVersion
	cfg.GitCommit = version.Commit

	// the core config can be assumed to already be set-up as it has been
	// injected as a component dependency
	// TODO: do not interface directly with pkg/config anywhere
	coreConfigObject := c.Object()
	if coreConfigObject == nil {
		return nil, fmt.Errorf("No core config found! Bailing out.")
	}

	if !coreConfigObject.GetBool("disable_file_logging") {
		cfg.LogFilePath = DefaultLogFilePath
	}

	orch := fargate.GetOrchestrator() // Needs to be after loading config, because it relies on feature auto-detection
	cfg.FargateOrchestrator = config.FargateOrchestratorName(orch)
	if p := coreconfig.Datadog.GetProxies(); p != nil {
		cfg.Proxy = httputils.GetProxyTransportFunc(p)
	}
	if coreconfig.IsRemoteConfigEnabled(coreConfigObject) && coreConfigObject.GetBool("remote_configuration.apm_sampling.enabled") {
		client, err := remote.NewGRPCClient(
			rcClientName,
			version.AgentVersion,
			[]data.Product{data.ProductAPMSampling, data.ProductAgentConfig},
			rcClientPollInterval,
		)
		if err != nil {
			log.Errorf("Error when subscribing to remote config management %v", err)
		} else {
			cfg.RemoteConfigClient = client
		}
	}
	cfg.ContainerTags = containerTagsFunc
	cfg.ContainerProcRoot = coreConfigObject.GetString("container_proc_root")
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

func applyDatadogConfig(c *config.AgentConfig, core corecompcfg.Component) error {
	if len(c.Endpoints) == 0 {
		c.Endpoints = []*config.Endpoint{{}}
	}
	if core.IsSet("api_key") {
		c.Endpoints[0].APIKey = coreconfig.SanitizeAPIKey(coreconfig.Datadog.GetString("api_key"))
	}
	if core.IsSet("hostname") {
		c.Hostname = core.GetString("hostname")
	}
	if core.IsSet("dogstatsd_port") {
		c.StatsdPort = core.GetInt("dogstatsd_port")
	}

	obsPipelineEnabled, prefix := isObsPipelineEnabled(core)
	if obsPipelineEnabled {
		if host := core.GetString(fmt.Sprintf("%s.traces.url", prefix)); host == "" {
			log.Errorf("%s.traces.enabled but %s.traces.url is empty.", prefix, prefix)
		} else {
			c.Endpoints[0].Host = host
		}
	} else {
		c.Endpoints[0].Host = utils.GetMainEndpoint(coreconfig.Datadog, apiEndpointPrefix, "apm_config.apm_dd_url")
	}
	c.Endpoints = appendEndpoints(c.Endpoints, "apm_config.additional_endpoints")

	if core.IsSet("proxy.no_proxy") {
		proxyList := core.GetStringSlice("proxy.no_proxy")
		noProxy := make(map[string]bool, len(proxyList))
		for _, host := range proxyList {
			// map of hosts that need to be skipped by proxy
			noProxy[host] = true
		}
		for _, e := range c.Endpoints {
			e.NoProxy = noProxy[e.Host]
		}
	}

	if addr := core.GetString("proxy.https"); addr != "" {
		url, err := url.Parse(addr)
		if err == nil {
			c.ProxyURL = url
			c.Proxy = http.ProxyURL(c.ProxyURL)
		} else {
			log.Errorf("Failed to parse proxy URL from proxy.https configuration: %s", err)
		}
	}

	if core.IsSet("skip_ssl_validation") {
		c.SkipSSLValidation = core.GetBool("skip_ssl_validation")
	}
	if core.IsSet("apm_config.enabled") {
		c.Enabled = core.GetBool("apm_config.enabled")
	}
	if coreconfig.Datadog.IsSet("apm_config.log_file") {
		c.LogFilePath = coreconfig.Datadog.GetString("apm_config.log_file")
	}

	if env := configUtils.GetTraceAgentDefaultEnv(coreconfig.Datadog); env != "" {
		c.DefaultEnv = env
	}

	prevEnv := c.DefaultEnv
	c.DefaultEnv = traceutil.NormalizeTag(c.DefaultEnv)
	if c.DefaultEnv != prevEnv {
		log.Debugf("Normalized DefaultEnv from %q to %q", prevEnv, c.DefaultEnv)
	}
	if core.IsSet("apm_config.receiver_port") {
		c.ReceiverPort = core.GetInt("apm_config.receiver_port")
	}
	if core.IsSet("apm_config.receiver_socket") {
		c.ReceiverSocket = core.GetString("apm_config.receiver_socket")
	}
	if core.IsSet("apm_config.connection_limit") {
		c.ConnectionLimit = core.GetInt("apm_config.connection_limit")
	}
	c.PeerServiceAggregation = core.GetBool("apm_config.peer_service_aggregation")
	c.ComputeStatsBySpanKind = core.GetBool("apm_config.compute_stats_by_span_kind")
	if core.IsSet("apm_config.extra_sample_rate") {
		c.ExtraSampleRate = core.GetFloat64("apm_config.extra_sample_rate")
	}
	if core.IsSet("apm_config.max_events_per_second") {
		c.MaxEPS = core.GetFloat64("apm_config.max_events_per_second")
	}
	if core.IsSet("apm_config.max_traces_per_second") {
		c.TargetTPS = core.GetFloat64("apm_config.max_traces_per_second")
	}
	if core.IsSet("apm_config.errors_per_second") {
		c.ErrorTPS = core.GetFloat64("apm_config.errors_per_second")
	}
	if core.IsSet("apm_config.enable_rare_sampler") {
		c.RareSamplerEnabled = core.GetBool("apm_config.enable_rare_sampler")
	}
	if core.IsSet("apm_config.rare_sampler.tps") {
		c.RareSamplerTPS = core.GetInt("apm_config.rare_sampler.tps")
	}
	if core.IsSet("apm_config.rare_sampler.cooldown") {
		c.RareSamplerCooldownPeriod = core.GetDuration("apm_config.rare_sampler.cooldown")
	}
	if core.IsSet("apm_config.rare_sampler.cardinality") {
		c.RareSamplerCardinality = core.GetInt("apm_config.rare_sampler.cardinality")
	}

	if core.IsSet("apm_config.max_remote_traces_per_second") {
		c.MaxRemoteTPS = core.GetFloat64("apm_config.max_remote_traces_per_second")
	}
	if k := "apm_config.features"; core.IsSet(k) {
		feats := core.GetStringSlice(k)
		for _, f := range feats {
			c.Features[f] = struct{}{}
		}
		if c.HasFeature("big_resource") {
			c.MaxResourceLen = 15_000
		}
		log.Infof("Found APM feature flags: %s", feats)
	}

	if k := "apm_config.ignore_resources"; core.IsSet(k) {
		c.Ignore["resource"] = core.GetStringSlice(k)
	}
	if k := "apm_config.max_payload_size"; core.IsSet(k) {
		c.MaxRequestBytes = core.GetInt64(k)
	}
	if core.IsSet("apm_config.trace_buffer") {
		c.TraceBuffer = core.GetInt("apm_config.trace_buffer")
	}
	if core.IsSet("apm_config.decoders") {
		c.Decoders = core.GetInt("apm_config.decoders")
	}
	if core.IsSet("apm_config.max_connections") {
		c.MaxConnections = core.GetInt("apm_config.max_connections")
	} else {
		c.MaxConnections = 1000
	}
	if core.IsSet("apm_config.decoder_timeout") {
		c.DecoderTimeout = core.GetInt("apm_config.decoder_timeout")
	} else {
		c.DecoderTimeout = 1000
	}

	if k := "apm_config.replace_tags"; core.IsSet(k) {
		rt := make([]*config.ReplaceRule, 0)
		if err := coreconfig.Datadog.UnmarshalKey(k, &rt); err != nil {
			log.Errorf("Bad format for %q it should be of the form '[{\"name\": \"tag_name\",\"pattern\":\"pattern\",\"repl\":\"replace_str\"}]', error: %v", "apm_config.replace_tags", err)
		} else {
			err := compileReplaceRules(rt)
			if err != nil {
				return fmt.Errorf("replace_tags: %s", err)
			}
			c.ReplaceTags = rt
		}
	}

	if core.IsSet("bind_host") || core.IsSet("apm_config.apm_non_local_traffic") {
		if core.IsSet("bind_host") {
			host := core.GetString("bind_host")
			c.StatsdHost = host
			c.ReceiverHost = host
		}

		if core.IsSet("apm_config.apm_non_local_traffic") && core.GetBool("apm_config.apm_non_local_traffic") {
			c.ReceiverHost = "0.0.0.0"
		}
	} else if coreconfig.IsContainerized() {
		// Automatically activate non local traffic in containerized environment if no explicit config set
		log.Info("Activating non-local traffic automatically in containerized environment, trace-agent will listen on 0.0.0.0")
		c.ReceiverHost = "0.0.0.0"
	}
	c.StatsdPipeName = core.GetString("dogstatsd_pipe_name")
	c.StatsdSocket = core.GetString("dogstatsd_socket")
	c.WindowsPipeName = core.GetString("apm_config.windows_pipe_name")
	c.PipeBufferSize = core.GetInt("apm_config.windows_pipe_buffer_size")
	c.PipeSecurityDescriptor = core.GetString("apm_config.windows_pipe_security_descriptor")
	c.GUIPort = core.GetString("GUI_port")

	var grpcPort int
	if otlp.IsEnabled(coreconfig.Datadog) {
		grpcPort = core.GetInt(coreconfig.OTLPTracePort)
	}
	c.OTLPReceiver = &config.OTLP{
		BindHost:               c.ReceiverHost,
		GRPCPort:               grpcPort,
		MaxRequestBytes:        c.MaxRequestBytes,
		SpanNameRemappings:     coreconfig.Datadog.GetStringMapString("otlp_config.traces.span_name_remappings"),
		SpanNameAsResourceName: core.GetBool("otlp_config.traces.span_name_as_resource_name"),
		ProbabilisticSampling:  core.GetFloat64("otlp_config.traces.probabilistic_sampler.sampling_percentage"),
	}

	if core.GetBool("apm_config.telemetry.enabled") {
		c.TelemetryConfig.Enabled = true
		c.TelemetryConfig.Endpoints = []*config.Endpoint{{
			Host:   utils.GetMainEndpoint(coreconfig.Datadog, config.TelemetryEndpointPrefix, "apm_config.telemetry.dd_url"),
			APIKey: c.Endpoints[0].APIKey,
		}}
		c.TelemetryConfig.Endpoints = appendEndpoints(c.TelemetryConfig.Endpoints, "apm_config.telemetry.additional_endpoints")
	}
	c.Obfuscation = new(config.ObfuscationConfig)
	if core.IsSet("apm_config.obfuscation") {
		var o config.ObfuscationConfig
		err := coreconfig.Datadog.UnmarshalKey("apm_config.obfuscation", &o)
		if err == nil {
			c.Obfuscation = &o
			if o.RemoveStackTraces {
				if err = addReplaceRule(c, "error.stack", `(?s).*`, "?"); err != nil {
					return err
				}
			}
		}
	}
	{
		// TODO(x): There is an issue with coreconfig.Datadog.IsSet("apm_config.obfuscation"), probably coming from Viper,
		// where it returns false even is "apm_config.obfuscation.credit_cards.enabled" is set via an environment
		// variable, so we need a temporary workaround by specifically setting env. var. accessible fields.
		if core.IsSet("apm_config.obfuscation.credit_cards.enabled") {
			c.Obfuscation.CreditCards.Enabled = core.GetBool("apm_config.obfuscation.credit_cards.enabled")
		}
		if core.IsSet("apm_config.obfuscation.credit_cards.luhn") {
			c.Obfuscation.CreditCards.Luhn = core.GetBool("apm_config.obfuscation.credit_cards.luhn")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.elasticsearch.enabled") {
			c.Obfuscation.ES.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.elasticsearch.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.elasticsearch.keep_values") {
			c.Obfuscation.ES.KeepValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.elasticsearch.keep_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.elasticsearch.obfuscate_sql_values") {
			c.Obfuscation.ES.ObfuscateSQLValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.elasticsearch.obfuscate_sql_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.http.remove_query_string") {
			c.Obfuscation.HTTP.RemoveQueryString = coreconfig.Datadog.GetBool("apm_config.obfuscation.http.remove_query_string")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.http.remove_paths_with_digits") {
			c.Obfuscation.HTTP.RemovePathDigits = coreconfig.Datadog.GetBool("apm_config.obfuscation.http.remove_paths_with_digits")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.memcached.enabled") {
			c.Obfuscation.Memcached.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.memcached.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.mongodb.enabled") {
			c.Obfuscation.Mongo.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.mongodb.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.mongodb.keep_values") {
			c.Obfuscation.Mongo.KeepValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.mongodb.keep_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.mongodb.obfuscate_sql_values") {
			c.Obfuscation.Mongo.ObfuscateSQLValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.mongodb.obfuscate_sql_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.redis.enabled") {
			c.Obfuscation.Redis.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.redis.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.redis.remove_all_args") {
			c.Obfuscation.Redis.RemoveAllArgs = coreconfig.Datadog.GetBool("apm_config.obfuscation.redis.remove_all_args")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.remove_stack_traces") {
			c.Obfuscation.RemoveStackTraces = coreconfig.Datadog.GetBool("apm_config.obfuscation.remove_stack_traces")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.sql_exec_plan.enabled") {
			c.Obfuscation.SQLExecPlan.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.sql_exec_plan.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.sql_exec_plan.keep_values") {
			c.Obfuscation.SQLExecPlan.KeepValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan.keep_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.sql_exec_plan.obfuscate_sql_values") {
			c.Obfuscation.SQLExecPlan.ObfuscateSQLValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan.obfuscate_sql_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.sql_exec_plan_normalize.enabled") {
			c.Obfuscation.SQLExecPlanNormalize.Enabled = coreconfig.Datadog.GetBool("apm_config.obfuscation.sql_exec_plan_normalize.enabled")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.sql_exec_plan_normalize.keep_values") {
			c.Obfuscation.SQLExecPlanNormalize.KeepValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan_normalize.keep_values")
		}
		if coreconfig.Datadog.IsSet("apm_config.obfuscation.sql_exec_plan_normalize.obfuscate_sql_values") {
			c.Obfuscation.SQLExecPlanNormalize.ObfuscateSQLValues = coreconfig.Datadog.GetStringSlice("apm_config.obfuscation.sql_exec_plan_normalize.obfuscate_sql_values")
		}
	}

	if core.IsSet("apm_config.filter_tags.require") {
		tags := core.GetStringSlice("apm_config.filter_tags.require")
		for _, tag := range tags {
			c.RequireTags = append(c.RequireTags, splitTag(tag))
		}
	}
	if core.IsSet("apm_config.filter_tags.reject") {
		tags := core.GetStringSlice("apm_config.filter_tags.reject")
		for _, tag := range tags {
			c.RejectTags = append(c.RejectTags, splitTag(tag))
		}
	}

	// undocumented writers
	for key, cfg := range map[string]*config.WriterConfig{
		"apm_config.trace_writer": c.TraceWriter,
		"apm_config.stats_writer": c.StatsWriter,
	} {
		if err := coreconfig.Datadog.UnmarshalKey(key, cfg); err != nil {
			log.Errorf("Error reading writer config %q: %v", key, err)
		}
	}
	if core.IsSet("apm_config.connection_reset_interval") {
		c.ConnectionResetInterval = getDuration(core.GetInt("apm_config.connection_reset_interval"))
	}
	if core.IsSet("apm_config.max_sender_retries") {
		c.MaxSenderRetries = core.GetInt("apm_config.max_sender_retries")
	} else {
		// Default of 4 was chosen through experimentation, but may not be the optimal value.
		c.MaxSenderRetries = 4
	}
	if core.IsSet("apm_config.sync_flushing") {
		c.SynchronousFlushing = core.GetBool("apm_config.sync_flushing")
	}

	// undocumented deprecated
	if core.IsSet("apm_config.analyzed_rate_by_service") {
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
	if k := "apm_config.analyzed_spans"; core.IsSet(k) {
		for key, rate := range core.GetStringMap("apm_config.analyzed_spans") {
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
	if core.IsSet("apm_config.dd_agent_bin") {
		c.DDAgentBin = core.GetString("apm_config.dd_agent_bin")
	}

	if err := loadDeprecatedValues(c); err != nil {
		return err
	}

	if strings.ToLower(core.GetString("log_level")) == "debug" && !core.IsSet("apm_config.log_throttling") {
		// if we are in "debug mode" and log throttling behavior was not
		// set by the user, disable it
		c.LogThrottling = false
	}
	c.Site = core.GetString("site")
	if c.Site == "" {
		c.Site = coreconfig.DefaultSite
	}
	if k := "use_dogstatsd"; core.IsSet(k) {
		c.StatsdEnabled = core.GetBool(k)
	}
	if v := core.GetInt("apm_config.max_catalog_entries"); v > 0 {
		c.MaxCatalogEntries = v
	}
	if k := "apm_config.profiling_dd_url"; core.IsSet(k) {
		c.ProfilingProxy.DDURL = core.GetString(k)
	}
	if k := "apm_config.profiling_additional_endpoints"; core.IsSet(k) {
		c.ProfilingProxy.AdditionalEndpoints = core.GetStringMapStringSlice(k)
	}
	if k := "apm_config.debugger_dd_url"; core.IsSet(k) {
		c.DebuggerProxy.DDURL = core.GetString(k)
	}
	if k := "apm_config.debugger_api_key"; core.IsSet(k) {
		c.DebuggerProxy.APIKey = core.GetString(k)
	}
	if k := "apm_config.debugger_additional_endpoints"; core.IsSet(k) {
		c.DebuggerProxy.AdditionalEndpoints = core.GetStringMapStringSlice(k)
	}
	if k := "apm_config.symdb_dd_url"; core.IsSet(k) {
		c.SymDBProxy.DDURL = core.GetString(k)
	}
	if k := "apm_config.symdb_api_key"; core.IsSet(k) {
		c.SymDBProxy.APIKey = core.GetString(k)
	}
	if k := "apm_config.symdb_additional_endpoints"; core.IsSet(k) {
		c.SymDBProxy.AdditionalEndpoints = core.GetStringMapStringSlice(k)
	}
	if k := "evp_proxy_config.enabled"; core.IsSet(k) {
		c.EVPProxy.Enabled = core.GetBool(k)
	}
	if k := "evp_proxy_config.dd_url"; core.IsSet(k) {
		c.EVPProxy.DDURL = core.GetString(k)
	}
	if k := "evp_proxy_config.api_key"; core.IsSet(k) {
		c.EVPProxy.APIKey = core.GetString(k)
	}
	if k := "evp_proxy_config.app_key"; core.IsSet(k) {
		c.EVPProxy.ApplicationKey = core.GetString(k)
	} else {
		// Default to the agent-wide app_key if set
		c.EVPProxy.ApplicationKey = core.GetString("app_key")
	}
	if k := "evp_proxy_config.additional_endpoints"; core.IsSet(k) {
		c.EVPProxy.AdditionalEndpoints = core.GetStringMapStringSlice(k)
	}
	if k := "evp_proxy_config.max_payload_size"; core.IsSet(k) {
		c.EVPProxy.MaxPayloadSize = core.GetInt64(k)
	}
	c.DebugServerPort = core.GetInt("apm_config.debug.port")
	return nil
}

// loadDeprecatedValues loads a set of deprecated values which are kept for
// backwards compatibility with Agent 5. These should eventually be removed.
// TODO(x): remove them gradually or fully in a future release.
func loadDeprecatedValues(c *config.AgentConfig) error {
	cfg := coreconfig.Datadog
	if cfg.IsSet("apm_config.api_key") {
		c.Endpoints[0].APIKey = coreconfig.SanitizeAPIKey(cfg.GetString("apm_config.api_key"))
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
func addReplaceRule(c *config.AgentConfig, tag, pattern, repl string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("error adding replace rule: %s", err)
	}
	c.ReplaceTags = append(c.ReplaceTags, &config.ReplaceRule{
		Name:    tag,
		Pattern: pattern,
		Re:      re,
		Repl:    repl,
	})

	return nil
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
func validate(c *config.AgentConfig, core corecompcfg.Component) error {
	if len(c.Endpoints) == 0 || c.Endpoints[0].APIKey == "" {
		return config.ErrMissingAPIKey
	}
	if c.DDAgentBin == "" {
		return errors.New("agent binary path not set")
	}

	if c.Hostname == "" && !core.GetBool("serverless.enabled") {
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
				// Note: This endpoint is used by remote-config to set the log level dynamically
				// Please make sure to reach out to this team before removing it.
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

func isObsPipelineEnabled(core corecompcfg.Component) (bool, string) {
	if core.GetBool("observability_pipelines_worker.traces.enabled") {
		return true, "observability_pipelines_worker"
	}
	if core.GetBool("vector.traces.enabled") {
		return true, "vector"
	}
	return false, "observability_pipelines_worker"
}
