// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package openmetrics implements the generic OpenMetrics core check.
package openmetrics

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
)

const (
	// CheckName is the name of the check as registered in the core loader.
	CheckName = "openmetrics"

	defaultMetricLimit = 2000
	defaultTimeout     = 10 * time.Second

	latestMode openmetricsMode = "latest"
	legacyMode openmetricsMode = "legacy"
)

type openmetricsMode string

var errUnsupportedCoreConfig = errors.New("openmetrics core check does not support this configuration")

type instanceConfig struct {
	PrometheusURL       string        `yaml:"prometheus_url,omitempty"`
	OpenMetricsEndpoint string        `yaml:"openmetrics_endpoint,omitempty"`
	Namespace           string        `yaml:"namespace,omitempty"`
	Metrics             []interface{} `yaml:"metrics,omitempty"`
	ExtraMetrics        []interface{} `yaml:"extra_metrics,omitempty"`

	PromPrefix      string            `yaml:"prometheus_metrics_prefix,omitempty"`
	RawPrefix       string            `yaml:"raw_metric_prefix,omitempty"`
	LabelsMapper    map[string]string `yaml:"labels_mapper,omitempty"`
	RenameLabels    map[string]string `yaml:"rename_labels,omitempty"`
	ExcludeLabels   []string          `yaml:"exclude_labels,omitempty"`
	IncludeLabels   []string          `yaml:"include_labels,omitempty"`
	LabelToHostname string            `yaml:"label_to_hostname,omitempty"`
	HostnameLabel   string            `yaml:"hostname_label,omitempty"`
	HostnameFormat  string            `yaml:"hostname_format,omitempty"`

	HealthCheck            *bool                  `yaml:"health_service_check,omitempty"`
	EnableHealthCheck      *bool                  `yaml:"enable_health_service_check,omitempty"`
	LabelJoins             map[string]interface{} `yaml:"label_joins,omitempty"`
	ShareLabels            map[string]interface{} `yaml:"share_labels,omitempty"`
	CacheSharedLabels      *bool                  `yaml:"cache_shared_labels,omitempty"`
	TargetInfo             bool                   `yaml:"target_info,omitempty"`
	LabelToHostnameSuffix  string                 `yaml:"label_to_hostname_suffix,omitempty"`
	IgnoreMetrics          []string               `yaml:"ignore_metrics,omitempty"`
	ExcludeMetrics         []string               `yaml:"exclude_metrics,omitempty"`
	IgnoreMetricsByLabels  map[string]interface{} `yaml:"ignore_metrics_by_labels,omitempty"`
	ExcludeMetricsByLabels map[string]interface{} `yaml:"exclude_metrics_by_labels,omitempty"`
	IgnoreTags             []string               `yaml:"ignore_tags,omitempty"`
	RawLineFilters         []string               `yaml:"raw_line_filters,omitempty"`
	TagByEndpoint          *bool                  `yaml:"tag_by_endpoint,omitempty"`

	TypeOverride                  map[string]string `yaml:"type_overrides,omitempty"`
	SendHistogramBuckets          *bool             `yaml:"send_histograms_buckets,omitempty"`
	CollectHistogramBuckets       *bool             `yaml:"collect_histogram_buckets,omitempty"`
	NonCumulativeBuckets          *bool             `yaml:"non_cumulative_buckets,omitempty"`
	NonCumulativeHistogram        *bool             `yaml:"non_cumulative_histogram_buckets,omitempty"`
	DistributionBuckets           bool              `yaml:"send_distribution_buckets,omitempty"`
	HistogramBucketsDistribution  *bool             `yaml:"histogram_buckets_as_distributions,omitempty"`
	CollectCountersDistribution   *bool             `yaml:"collect_counters_with_distributions,omitempty"`
	MonotonicCounter              *bool             `yaml:"send_monotonic_counter,omitempty"`
	MonotonicWithGauge            bool              `yaml:"send_monotonic_with_gauge,omitempty"`
	DistributionCountsAsMonotonic bool              `yaml:"send_distribution_counts_as_monotonic,omitempty"`
	DistributionSumsAsMonotonic   bool              `yaml:"send_distribution_sums_as_monotonic,omitempty"`
	CacheMetricWildcards          *bool             `yaml:"cache_metric_wildcards,omitempty"`
	UseProcessStartTime           *bool             `yaml:"use_process_start_time,omitempty"`

	Headers                    map[string]string      `yaml:"headers,omitempty"`
	ExtraHeaders               map[string]string      `yaml:"extra_headers,omitempty"`
	Username                   string                 `yaml:"username,omitempty"`
	Password                   string                 `yaml:"password,omitempty"`
	BearerTokenAuth            interface{}            `yaml:"bearer_token_auth,omitempty"`
	BearerTokenPath            string                 `yaml:"bearer_token_path,omitempty"`
	BearerTokenRefreshInterval int                    `yaml:"bearer_token_refresh_interval,omitempty"`
	Proxy                      map[string]interface{} `yaml:"proxy,omitempty"`
	SkipProxy                  bool                   `yaml:"skip_proxy,omitempty"`
	NoProxy                    interface{}            `yaml:"no_proxy,omitempty"`
	AllowRedirects             *bool                  `yaml:"allow_redirects,omitempty"`
	Timeout                    float64                `yaml:"timeout,omitempty"`
	PrometheusTimeout          float64                `yaml:"prometheus_timeout,omitempty"`
	TLSVerify                  *bool                  `yaml:"tls_verify,omitempty"`
	TLSCert                    string                 `yaml:"tls_cert,omitempty"`
	TLSPrivateKey              string                 `yaml:"tls_private_key,omitempty"`
	TLSCACert                  string                 `yaml:"tls_ca_cert,omitempty"`
	TLSPrivateKeyPassword      string                 `yaml:"tls_private_key_password,omitempty"`
	TLSValidateHostname        *bool                  `yaml:"tls_validate_hostname,omitempty"`
	TLSUseHostHeader           bool                   `yaml:"tls_use_host_header,omitempty"`
	TLSProtocolsAllowed        []string               `yaml:"tls_protocols_allowed,omitempty"`
	TLSCiphers                 interface{}            `yaml:"tls_ciphers,omitempty"`
	SSLVerify                  *bool                  `yaml:"ssl_verify,omitempty"`
	SSLCert                    string                 `yaml:"ssl_cert,omitempty"`
	SSLPrivateKey              string                 `yaml:"ssl_private_key,omitempty"`
	SSLCACert                  interface{}            `yaml:"ssl_ca_cert,omitempty"`

	Tags                   []string               `yaml:"tags,omitempty"`
	Service                string                 `yaml:"service,omitempty"`
	MaxReturnedMetrics     int                    `yaml:"max_returned_metrics,omitempty"`
	UseLatestSpec          *bool                  `yaml:"use_latest_spec,omitempty"`
	IgnoreConnectionErrors *bool                  `yaml:"ignore_connection_errors,omitempty"`
	Telemetry              *bool                  `yaml:"telemetry,omitempty"`
	PersistConnections     *bool                  `yaml:"persist_connections,omitempty"`
	LogRequests            *bool                  `yaml:"log_requests,omitempty"`
	MetadataMetricName     string                 `yaml:"metadata_metric_name,omitempty"`
	MetadataLabelMap       map[string]string      `yaml:"metadata_label_map,omitempty"`
	AuthToken              map[string]interface{} `yaml:"auth_token,omitempty"`
}

type scraperConfig struct {
	mode         openmetricsMode
	endpoint     string
	namespace    string
	namespaceSet bool

	rawMetricPrefix string
	metrics         []interface{}
	extraMetrics    []interface{}
	excludeMetrics  []string

	includeLabels []string
	excludeLabels []string
	renameLabels  map[string]string

	excludeMetricsByLabels map[string]interface{}
	shareLabels            map[string]types.ShareLabelsConfig
	targetInfo             bool
	cacheSharedLabels      bool

	rawLineFilters []string
	tags           []string
	ignoreTags     []string
	tagByEndpoint  bool

	hostnameLabel  string
	hostnameFormat string

	enableHealthServiceCheck bool
	healthServiceCheckName   string
	ignoreConnectionErrors   bool
	telemetry                bool
	persistConnections       bool

	timeout            time.Duration
	maxReturnedMetrics int

	useLatestSpec bool

	collectHistogramBuckets           bool
	nonCumulativeHistogramBuckets     bool
	histogramBucketsAsDistributions   bool
	collectCountersWithDistributions  bool
	cacheMetricWildcards              bool
	useProcessStartTime               bool
	sendMonotonicCounter              bool
	sendMonotonicWithGauge            bool
	sendDistributionCountsAsMonotonic bool
	sendDistributionSumsAsMonotonic   bool
	sendHistogramBuckets              bool
	sendDistributionBuckets           bool

	headers                    map[string]string
	username                   string
	password                   string
	basicAuthConfigured        bool
	legacyAuthEncoding         bool
	bearerTokenAuth            bool
	bearerTokenPath            string
	bearerToken                string
	bearerTokenRefreshInterval time.Duration
	tlsVerify                  bool
	tlsCert                    string
	tlsPrivateKey              string
	tlsCACert                  string
	tlsPrivateKeyPassword      string
	tlsValidateHostname        bool
	tlsUseHostHeader           bool
	tlsProtocolsAllowed        []string
	tlsCiphers                 []string
	skipProxy                  bool
	proxy                      map[string]string
	noProxy                    []string
	allowRedirect              bool
	authToken                  *authTokenConfig
	checkID                    string
}

func parseConfig(raw []byte) (*scraperConfig, error) {
	return parseConfigWithInit(raw, nil)
}

func parseConfigWithInit(raw []byte, initRaw []byte) (*scraperConfig, error) {
	rawConfig, err := decodeRawConfig(raw)
	if err != nil {
		return nil, err
	}
	initConfig, err := decodeRawConfig(initRaw)
	if err != nil {
		return nil, err
	}
	effectiveRawConfig := mergeInitConfig(initConfig, rawConfig)
	latestConfig := hasRawKey(rawConfig, "openmetrics_endpoint")
	if err := validateRawConfig(effectiveRawConfig, latestConfig); err != nil {
		return nil, err
	}

	effectiveRaw, err := yaml.Marshal(effectiveRawConfig)
	if err != nil {
		return nil, err
	}

	var instance instanceConfig
	if err := yaml.Unmarshal(effectiveRaw, &instance); err != nil {
		return nil, err
	}

	shareLabels, err := parseShareLabels(instance.ShareLabels)
	if err != nil {
		return nil, err
	}
	shareLabels = mergeShareLabels(shareLabels, labelJoinsToShareLabels(instance.LabelJoins))
	authToken, err := parseAuthToken(instance.AuthToken)
	if err != nil {
		return nil, err
	}
	proxy, noProxy := parseProxyConfig(instance.Proxy)
	tlsCiphers, err := parseTLSCiphers(instance.TLSCiphers)
	if err != nil {
		return nil, err
	}

	mode := latestMode
	endpoint := instance.OpenMetricsEndpoint
	if !latestConfig {
		mode = legacyMode
		endpoint = instance.PrometheusURL
	}
	bearerTokenAuth, err := parseBearerTokenAuth(instance.BearerTokenAuth, endpoint)
	if err != nil {
		return nil, err
	}
	bearerTokenRefreshInterval := 60
	if hasRawKey(effectiveRawConfig, "bearer_token_refresh_interval") {
		bearerTokenRefreshInterval = instance.BearerTokenRefreshInterval
	}

	cfg := &scraperConfig{
		mode:                              mode,
		endpoint:                          endpoint,
		namespace:                         instance.Namespace,
		namespaceSet:                      hasRawKey(rawConfig, "namespace"),
		rawMetricPrefix:                   instance.RawPrefix,
		metrics:                           instance.Metrics,
		extraMetrics:                      instance.ExtraMetrics,
		excludeMetrics:                    instance.ExcludeMetrics,
		includeLabels:                     instance.IncludeLabels,
		excludeLabels:                     instance.ExcludeLabels,
		renameLabels:                      copyStringMap(instance.RenameLabels),
		excludeMetricsByLabels:            copyInterfaceMap(instance.ExcludeMetricsByLabels),
		shareLabels:                       shareLabels,
		targetInfo:                        instance.TargetInfo,
		cacheSharedLabels:                 boolDefault(instance.CacheSharedLabels, instance.CacheSharedLabels == nil, true),
		rawLineFilters:                    instance.RawLineFilters,
		tags:                              append([]string(nil), instance.Tags...),
		ignoreTags:                        append([]string(nil), instance.IgnoreTags...),
		tagByEndpoint:                     boolDefaultPtr(instance.TagByEndpoint, true),
		hostnameLabel:                     instance.HostnameLabel,
		hostnameFormat:                    instance.HostnameFormat,
		enableHealthServiceCheck:          boolDefaultPtr(instance.EnableHealthCheck, true),
		healthServiceCheckName:            "openmetrics.health",
		ignoreConnectionErrors:            boolDefaultPtr(instance.IgnoreConnectionErrors, false),
		telemetry:                         boolDefaultPtr(instance.Telemetry, false),
		persistConnections:                boolDefaultPtr(instance.PersistConnections, false) || instance.TLSUseHostHeader,
		timeout:                           defaultTimeout,
		maxReturnedMetrics:                instance.MaxReturnedMetrics,
		useLatestSpec:                     boolDefaultPtr(instance.UseLatestSpec, false),
		collectHistogramBuckets:           boolDefaultPtr(instance.CollectHistogramBuckets, true),
		nonCumulativeHistogramBuckets:     boolDefaultPtr(instance.NonCumulativeHistogram, false),
		histogramBucketsAsDistributions:   boolDefaultPtr(instance.HistogramBucketsDistribution, false),
		collectCountersWithDistributions:  boolDefaultPtr(instance.CollectCountersDistribution, false),
		cacheMetricWildcards:              boolDefaultPtr(instance.CacheMetricWildcards, true),
		useProcessStartTime:               boolDefaultPtr(instance.UseProcessStartTime, false),
		sendMonotonicCounter:              boolDefaultPtr(instance.MonotonicCounter, true),
		sendMonotonicWithGauge:            instance.MonotonicWithGauge,
		sendDistributionCountsAsMonotonic: instance.DistributionCountsAsMonotonic,
		sendDistributionSumsAsMonotonic:   instance.DistributionSumsAsMonotonic,
		sendHistogramBuckets:              boolDefaultPtr(instance.SendHistogramBuckets, true),
		sendDistributionBuckets:           instance.DistributionBuckets,
		headers:                           mergeHeaders(instance.Headers, instance.ExtraHeaders),
		username:                          instance.Username,
		password:                          instance.Password,
		basicAuthConfigured:               hasRawKey(effectiveRawConfig, "username") && hasRawKey(effectiveRawConfig, "password"),
		legacyAuthEncoding:                rawBoolDefault(effectiveRawConfig, "use_legacy_auth_encoding", true),
		bearerTokenAuth:                   bearerTokenAuth,
		bearerTokenPath:                   instance.BearerTokenPath,
		bearerTokenRefreshInterval:        time.Duration(bearerTokenRefreshInterval) * time.Second,
		tlsVerify:                         boolDefaultPtr(instance.TLSVerify, true),
		tlsCert:                           instance.TLSCert,
		tlsPrivateKey:                     instance.TLSPrivateKey,
		tlsCACert:                         instance.TLSCACert,
		tlsPrivateKeyPassword:             instance.TLSPrivateKeyPassword,
		tlsValidateHostname:               boolDefaultPtr(instance.TLSValidateHostname, true),
		tlsUseHostHeader:                  instance.TLSUseHostHeader,
		tlsProtocolsAllowed:               append([]string(nil), instance.TLSProtocolsAllowed...),
		tlsCiphers:                        tlsCiphers,
		skipProxy:                         instance.SkipProxy,
		proxy:                             proxy,
		noProxy:                           noProxy,
		allowRedirect:                     boolDefaultPtr(instance.AllowRedirects, true),
		authToken:                         authToken,
	}
	if !hasRawKey(effectiveRawConfig, "skip_proxy") && hasRawKey(effectiveRawConfig, "no_proxy") {
		cfg.skipProxy = rawAffirmative(instance.NoProxy)
	}

	if instance.Timeout > 0 {
		cfg.timeout = time.Duration(instance.Timeout * float64(time.Second))
	} else if instance.PrometheusTimeout > 0 {
		cfg.timeout = time.Duration(instance.PrometheusTimeout * float64(time.Second))
	}
	if cfg.maxReturnedMetrics == 0 {
		cfg.maxReturnedMetrics = defaultMetricLimit
	}

	if mode == latestMode {
		if err := cfg.validateLatest(); err != nil {
			return nil, err
		}
		cfg.histogramBucketsAsDistributions = cfg.histogramBucketsAsDistributions || cfg.collectCountersWithDistributions
		cfg.collectHistogramBuckets = cfg.collectHistogramBuckets || cfg.histogramBucketsAsDistributions
		cfg.nonCumulativeHistogramBuckets = cfg.nonCumulativeHistogramBuckets || cfg.histogramBucketsAsDistributions
		return cfg, nil
	}

	if err := cfg.applyLegacyCompatibility(&instance); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *scraperConfig) validateLatest() error {
	if c.endpoint == "" {
		return errors.New("the setting `openmetrics_endpoint` is required")
	}
	if c.namespace == "" && !c.namespaceSet {
		c.namespace = "openmetrics"
	}
	return validateCommon(c)
}

func (c *scraperConfig) applyLegacyCompatibility(instance *instanceConfig) error {
	if c.endpoint == "" {
		return errors.New("unable to find prometheus URL in config file")
	}
	if c.namespace == "" && !c.namespaceSet {
		c.namespace = "openmetrics"
	}

	c.rawMetricPrefix = instance.PromPrefix
	c.renameLabels = copyStringMap(instance.LabelsMapper)
	if c.renameLabels == nil {
		c.renameLabels = map[string]string{}
	}
	c.renameLabels["le"] = "upper_bound"
	c.hostnameLabel = instance.LabelToHostname
	if instance.LabelToHostnameSuffix != "" {
		c.hostnameFormat = "<HOSTNAME>" + instance.LabelToHostnameSuffix
	}
	c.enableHealthServiceCheck = boolDefaultPtr(instance.HealthCheck, true)
	c.healthServiceCheckName = "prometheus.health"
	c.excludeMetrics = globListToRegex(instance.IgnoreMetrics)
	c.excludeMetricsByLabels = legacyExcludeByLabels(instance.IgnoreMetricsByLabels)
	c.shareLabels = labelJoinsToShareLabels(instance.LabelJoins)
	if instance.CacheSharedLabels == nil && len(instance.LabelJoins) > 0 {
		c.cacheSharedLabels = false
	}
	c.collectHistogramBuckets = boolDefaultPtr(instance.SendHistogramBuckets, true)
	c.nonCumulativeHistogramBuckets = boolDefaultPtr(instance.NonCumulativeBuckets, false)
	c.histogramBucketsAsDistributions = instance.DistributionBuckets
	c.collectCountersWithDistributions = false
	if c.histogramBucketsAsDistributions {
		c.nonCumulativeHistogramBuckets = true
	}
	c.cacheMetricWildcards = true
	c.useLatestSpec = false
	c.tagByEndpoint = false
	c.headers = mergeHeaders(instance.Headers, instance.ExtraHeaders)
	c.metrics = legacyMetrics(instance.Metrics, instance.TypeOverride)
	if instance.MetadataMetricName != "" && len(instance.MetadataLabelMap) > 0 {
		c.metrics = append(c.metrics, map[string]interface{}{
			instance.MetadataMetricName: map[string]interface{}{
				"name":      instance.MetadataMetricName,
				"type":      "legacy_metadata",
				"label_map": copyStringMap(instance.MetadataLabelMap),
			},
		})
	}
	c.extraMetrics = nil
	bearerTokenAuth, err := parseBearerTokenAuth(instance.BearerTokenAuth, c.endpoint)
	if err != nil {
		return err
	}
	c.bearerTokenAuth = bearerTokenAuth
	c.bearerTokenPath = instance.BearerTokenPath
	c.tlsUseHostHeader = instance.TLSUseHostHeader
	c.tlsProtocolsAllowed = append([]string(nil), instance.TLSProtocolsAllowed...)
	c.tlsCiphers, _ = parseTLSCiphers(instance.TLSCiphers)
	if instance.SSLVerify != nil {
		c.tlsVerify = *instance.SSLVerify
	}
	if instance.SSLCert != "" {
		c.tlsCert = instance.SSLCert
	}
	if instance.SSLPrivateKey != "" {
		c.tlsPrivateKey = instance.SSLPrivateKey
	}
	switch ca := instance.SSLCACert.(type) {
	case string:
		c.tlsCACert = ca
	case bool:
		if !ca {
			c.tlsVerify = false
		}
	}

	return validateCommon(c)
}

func validateCommon(c *scraperConfig) error {
	if c.namespace != "" && !isString(c.namespace) {
		return errors.New("setting `namespace` must be a string")
	}
	if c.rawMetricPrefix != "" && !isString(c.rawMetricPrefix) {
		return errors.New("setting `raw_metric_prefix` must be a string")
	}
	if c.hostnameLabel != "" && !isString(c.hostnameLabel) {
		return errors.New("setting `hostname_label` must be a string")
	}
	if c.hostnameFormat != "" {
		if !strings.Contains(c.hostnameFormat, "<HOSTNAME>") && c.hostnameLabel != "" {
			return errors.New("setting `hostname_format` does not contain the placeholder `<HOSTNAME>`")
		}
	}
	if _, err := compileRegexList(c.rawLineFilters); err != nil {
		return err
	}
	if _, err := compileRegexList(c.excludeMetrics); err != nil {
		return err
	}
	if _, err := compileRegexList(c.ignoreTags); err != nil {
		return err
	}
	return nil
}

func decodeRawConfig(raw []byte) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	var decoded interface{}
	if err := yaml.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	config, ok := normalizeMap(decoded)
	if !ok {
		return nil, errors.New("openmetrics instance configuration must be a mapping")
	}
	return config, nil
}

func hasRawKey(config map[string]interface{}, key string) bool {
	_, ok := config[key]
	return ok
}

func mergeInitConfig(initConfig map[string]interface{}, instanceConfig map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{"timeout", "skip_proxy", "log_requests", "tls_ignore_warning"} {
		if value, ok := initConfig[key]; ok {
			out[key] = value
		}
	}
	if value, ok := initConfig["proxy"]; ok {
		out["proxy"] = value
	}
	for key, value := range instanceConfig {
		out[key] = value
	}
	return out
}

func validateRawConfig(config map[string]interface{}, latestConfig bool) error {
	if err := validateUnsupportedRawOptions(config); err != nil {
		return err
	}
	if latestConfig {
		endpoint, ok := config["openmetrics_endpoint"].(string)
		if !ok {
			return errors.New("the setting `openmetrics_endpoint` must be a string")
		}
		if endpoint == "" {
			return errors.New("the setting `openmetrics_endpoint` is required")
		}
	}

	for _, setting := range []string{"namespace", "raw_metric_prefix", "hostname_label", "hostname_format"} {
		if value, ok := config[setting]; ok {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("setting `%s` must be a string", setting)
			}
		}
	}
	for _, setting := range []string{"exclude_labels", "include_labels", "exclude_metrics", "tags", "raw_line_filters"} {
		if err := validateRawStringArray(config, setting); err != nil {
			return err
		}
	}
	if err := validateRawStringMap(config, "rename_labels", "label"); err != nil {
		return err
	}
	if err := validateRawExcludeMetricsByLabels(config); err != nil {
		return err
	}
	for _, setting := range []string{"metrics", "extra_metrics"} {
		if err := validateRawMetrics(config, setting); err != nil {
			return err
		}
	}
	if err := validateRawShareLabels(config); err != nil {
		return err
	}
	if err := validateRawProxy(config); err != nil {
		return err
	}
	if err := validateRawTLSOptions(config); err != nil {
		return err
	}
	if value, ok := config["target_info"]; ok {
		if _, ok := value.(bool); !ok {
			return errors.New("setting `target_info` must be a boolean")
		}
	}
	if rawAuthToken, ok := config["auth_token"]; ok {
		authTokenConfig, ok := normalizeMap(rawAuthToken)
		if !ok {
			return errors.New("the `auth_token` field must be a mapping")
		}
		if _, err := parseAuthToken(authTokenConfig); err != nil {
			return err
		}
	}
	return nil
}

func validateRawProxy(config map[string]interface{}) error {
	rawProxy, ok := config["proxy"]
	if !ok {
		return nil
	}
	proxyConfig, ok := normalizeMap(rawProxy)
	if !ok {
		return errors.New("setting `proxy` must be a mapping")
	}
	for _, key := range []string{"http", "https", "url"} {
		if value, ok := proxyConfig[key]; ok {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("setting `proxy.%s` must be a string", key)
			}
		}
	}
	if noProxy, ok := proxyConfig["no_proxy"]; ok {
		if _, err := parseNoProxy(noProxy); err != nil {
			return err
		}
	}
	return nil
}

func validateRawTLSOptions(config map[string]interface{}) error {
	for _, setting := range []string{"tls_use_host_header", "tls_validate_hostname"} {
		if value, ok := config[setting]; ok {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("setting `%s` must be a boolean", setting)
			}
		}
	}
	for _, setting := range []string{"tls_protocols_allowed"} {
		if err := validateRawStringArray(config, setting); err != nil {
			return err
		}
	}
	if value, ok := config["tls_ciphers"]; ok {
		ciphers, err := parseTLSCiphers(value)
		if err != nil {
			return err
		}
		if _, err := tlsCipherSuites(ciphers); err != nil {
			return unsupportedCoreConfig(err.Error())
		}
	}
	return nil
}

func validateUnsupportedRawOptions(config map[string]interface{}) error {
	for _, setting := range []string{"openmetrics_endpoint", "prometheus_url"} {
		if endpoint, ok := config[setting].(string); ok && strings.HasPrefix(strings.ToLower(endpoint), "unix://") {
			return unsupportedCoreConfig("unix socket endpoint")
		}
	}

	if rawAuthType, ok := config["auth_type"]; ok {
		authType, ok := rawAuthType.(string)
		if !ok {
			return errors.New("setting `auth_type` must be a string")
		}
		if authType != "" && !strings.EqualFold(authType, "basic") {
			return unsupportedCoreConfig(fmt.Sprintf("auth_type `%s`", authType))
		}
	}

	for _, setting := range []string{
		"metric_patterns", "connect_timeout", "read_timeout", "request_size",
		"tls_protocols_allowed",
		"aws_host", "aws_region", "aws_service", "ntlm_domain",
		"kerberos", "kerberos_auth", "kerberos_cache", "kerberos_delegate", "kerberos_force_initiate",
		"kerberos_hostname", "kerberos_keytab", "kerberos_principal",
		"tls_intermediate_ca_certs",
	} {
		if _, ok := config[setting]; ok {
			return unsupportedCoreConfig(setting)
		}
	}
	if rawAffirmative(config["log_requests"]) {
		return unsupportedCoreConfig("log_requests")
	}
	if rawAffirmative(config["tls_ignore_warning"]) {
		return unsupportedCoreConfig("tls_ignore_warning")
	}
	if rawAffirmative(config["disable_generic_tags"]) {
		return unsupportedCoreConfig("disable_generic_tags")
	}
	if value, ok := config["enable_legacy_tags_normalization"]; ok && !rawAffirmative(value) {
		return unsupportedCoreConfig("enable_legacy_tags_normalization")
	}
	if value, ok := config["min_collection_interval"]; ok {
		if _, ok := value.(int); !ok {
			return unsupportedCoreConfig("fractional min_collection_interval")
		}
	}
	if authTokenOAuthHasOptions(config["auth_token"]) {
		return unsupportedCoreConfig("auth_token oauth options")
	}
	return nil
}

func authTokenOAuthHasOptions(raw interface{}) bool {
	authToken, ok := normalizeMap(raw)
	if !ok {
		return false
	}
	reader, ok := normalizeMap(authToken["reader"])
	if !ok || reader["type"] != "oauth" {
		return false
	}
	options, ok := normalizeMap(reader["options"])
	return ok && len(options) > 0
}

func rawBoolDefault(config map[string]interface{}, key string, defaultValue bool) bool {
	value, ok := config[key]
	if !ok {
		return defaultValue
	}
	return rawAffirmative(value)
}

func unsupportedCoreConfig(setting string) error {
	return fmt.Errorf("%w: %s", errUnsupportedCoreConfig, setting)
}

func validateRawStringArray(config map[string]interface{}, setting string) error {
	rawValue, ok := config[setting]
	if !ok {
		return nil
	}
	values, ok := rawValue.([]interface{})
	if !ok {
		return fmt.Errorf("setting `%s` must be an array", setting)
	}
	for i, value := range values {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("entry #%d of setting `%s` must be a string", i+1, setting)
		}
	}
	return nil
}

func validateRawStringMap(config map[string]interface{}, setting string, keyName string) error {
	rawValue, ok := config[setting]
	if !ok {
		return nil
	}
	values, ok := normalizeMap(rawValue)
	if !ok {
		return fmt.Errorf("setting `%s` must be a mapping", setting)
	}
	for key, value := range values {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("value for %s `%s` of setting `%s` must be a string", keyName, key, setting)
		}
	}
	return nil
}

func validateRawExcludeMetricsByLabels(config map[string]interface{}) error {
	rawValue, ok := config["exclude_metrics_by_labels"]
	if !ok {
		return nil
	}
	values, ok := normalizeMap(rawValue)
	if !ok {
		return errors.New("setting `exclude_metrics_by_labels` must be a mapping")
	}
	for label, rawValues := range values {
		if value, ok := rawValues.(bool); ok {
			if value {
				continue
			}
			return fmt.Errorf("label `%s` of setting `exclude_metrics_by_labels` must be an array or set to `true`", label)
		}
		labelValues, ok := rawValues.([]interface{})
		if !ok {
			return fmt.Errorf("label `%s` of setting `exclude_metrics_by_labels` must be an array or set to `true`", label)
		}
		for i, labelValue := range labelValues {
			if _, ok := labelValue.(string); !ok {
				return fmt.Errorf("value #%d for label `%s` of setting `exclude_metrics_by_labels` must be a string", i+1, label)
			}
		}
	}
	return nil
}

func validateRawMetrics(config map[string]interface{}, setting string) error {
	rawValue, ok := config[setting]
	if !ok {
		return nil
	}
	values, ok := rawValue.([]interface{})
	if !ok {
		return fmt.Errorf("setting `%s` must be an array", setting)
	}
	for i, value := range values {
		if _, ok := value.(string); ok {
			continue
		}
		metricMap, ok := normalizeMap(value)
		if !ok {
			return fmt.Errorf("entry #%d of setting `%s` must be a string or a mapping", i+1, setting)
		}
		for metricName, rawMetricConfig := range metricMap {
			if _, ok := rawMetricConfig.(string); ok {
				continue
			}
			if _, ok := normalizeMap(rawMetricConfig); !ok {
				return fmt.Errorf("value of entry `%s` of setting `%s` must be a string or a mapping", metricName, setting)
			}
		}
	}
	return nil
}

func validateRawShareLabels(config map[string]interface{}) error {
	rawValue, ok := config["share_labels"]
	if !ok {
		return nil
	}
	values, ok := normalizeMap(rawValue)
	if !ok {
		return errors.New("setting `share_labels` must be a mapping")
	}
	for metricName, rawMetricConfig := range values {
		if value, ok := rawMetricConfig.(bool); ok {
			if value {
				continue
			}
			return fmt.Errorf("metric `%s` of setting `share_labels` must be a mapping or set to `true`", metricName)
		}
		metricConfig, ok := normalizeMap(rawMetricConfig)
		if !ok {
			return fmt.Errorf("metric `%s` of setting `share_labels` must be a mapping or set to `true`", metricName)
		}
		for _, option := range []string{"labels", "match"} {
			if err := validateRawShareLabelStringArray(metricConfig, metricName, option); err != nil {
				return err
			}
		}
		if err := validateRawShareLabelValues(metricConfig, metricName); err != nil {
			return err
		}
	}
	return nil
}

func validateRawShareLabelStringArray(config map[string]interface{}, metricName string, option string) error {
	rawValue, ok := config[option]
	if !ok {
		return nil
	}
	values, ok := rawValue.([]interface{})
	if !ok {
		return fmt.Errorf("option `%s` for metric `%s` of setting `share_labels` must be an array", option, metricName)
	}
	for i, value := range values {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("entry #%d of option `%s` for metric `%s` of setting `share_labels` must be a string", i+1, option, metricName)
		}
	}
	return nil
}

func validateRawShareLabelValues(config map[string]interface{}, metricName string) error {
	rawValue, ok := config["values"]
	if !ok {
		return nil
	}
	values, ok := rawValue.([]interface{})
	if !ok {
		return fmt.Errorf("option `values` for metric `%s` of setting `share_labels` must be an array", metricName)
	}
	for i, value := range values {
		switch typedValue := value.(type) {
		case int:
			continue
		case string:
			if _, err := strconv.Atoi(typedValue); err == nil {
				continue
			}
		}
		return fmt.Errorf("entry #%d of option `values` for metric `%s` of setting `share_labels` must represent an integer", i+1, metricName)
	}
	return nil
}

func isString(_ string) bool {
	return true
}

func boolDefaultPtr(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func boolDefault(value *bool, useDefault bool, defaultValue bool) bool {
	if useDefault {
		return defaultValue
	}
	return *value
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyInterfaceMap(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeHeaders(headers, extra map[string]string) map[string]string {
	out := copyStringMap(headers)
	if out == nil {
		out = map[string]string{}
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func compileRegexList(patterns []string) (*regexp.Regexp, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	return regexp.Compile(strings.Join(patterns, "|"))
}

func globListToRegex(globs []string) []string {
	out := make([]string, 0, len(globs))
	for _, glob := range globs {
		out = append(out, globToRegex(glob))
	}
	return out
}

func globToRegex(glob string) string {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}

func legacyExcludeByLabels(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for label, rawValues := range in {
		values := interfaceSliceToStrings(rawValues)
		if stringSliceContains(values, "*") {
			out[label] = true
		} else {
			out[label] = values
		}
	}
	return out
}

func parseProxyConfig(rawProxy map[string]interface{}) (map[string]string, []string) {
	if len(rawProxy) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, key := range []string{"http", "https", "url"} {
		if value, ok := rawProxy[key].(string); ok && value != "" {
			out[key] = value
		}
	}
	noProxy, _ := parseNoProxy(rawProxy["no_proxy"])
	if len(out) == 0 {
		out = nil
	}
	return out, noProxy
}

func parseNoProxy(raw interface{}) ([]string, error) {
	switch value := raw.(type) {
	case nil:
		return nil, nil
	case string:
		return splitNoProxy(value), nil
	case []interface{}:
		out := make([]string, 0, len(value))
		for i, entry := range value {
			stringEntry, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("entry #%d of setting `proxy.no_proxy` must be a string", i+1)
			}
			out = append(out, stringEntry)
		}
		return out, nil
	case []string:
		return append([]string(nil), value...), nil
	default:
		return nil, errors.New("setting `proxy.no_proxy` must be a string or an array")
	}
}

func splitNoProxy(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func parseTLSCiphers(raw interface{}) ([]string, error) {
	switch value := raw.(type) {
	case nil:
		return nil, nil
	case string:
		return splitTLSCiphers(value), nil
	case []interface{}:
		out := make([]string, 0, len(value))
		for i, entry := range value {
			stringEntry, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("entry #%d of setting `tls_ciphers` must be a string", i+1)
			}
			out = append(out, stringEntry)
		}
		return out, nil
	case []string:
		return append([]string(nil), value...), nil
	default:
		return nil, errors.New("setting `tls_ciphers` must be a string or an array")
	}
}

func splitTLSCiphers(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ':'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func rawAffirmative(value interface{}) bool {
	switch typed := value.(type) {
	case string:
		switch strings.ToLower(typed) {
		case "yes", "true", "1", "y", "on":
			return true
		default:
			return false
		}
	case nil:
		return false
	case bool:
		return typed
	case int:
		return typed != 0
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	default:
		return true
	}
}

func parseBearerTokenAuth(raw interface{}, endpoint string) (bool, error) {
	switch value := raw.(type) {
	case nil:
		return false, nil
	case bool:
		return value, nil
	case string:
		switch strings.ToLower(value) {
		case "tls_only":
			return strings.HasPrefix(strings.ToLower(endpoint), "https://"), nil
		case "true", "yes", "1", "y", "on":
			return true, nil
		case "false", "no", "0", "n", "off", "":
			return false, nil
		default:
			return false, fmt.Errorf("setting `bearer_token_auth` must be a boolean or `tls_only`, got `%s`", value)
		}
	default:
		return false, errors.New("setting `bearer_token_auth` must be a boolean or `tls_only`")
	}
}

func parseShareLabels(in map[string]interface{}) (map[string]types.ShareLabelsConfig, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make(map[string]types.ShareLabelsConfig, len(in))
	for metric, rawConfig := range in {
		if enabled, ok := rawConfig.(bool); ok {
			if enabled {
				out[metric] = types.ShareLabelsConfig{}
				continue
			}
			return nil, errors.New("setting `share_labels` entries must be mappings or set to `true`")
		}

		config, ok := normalizeMap(rawConfig)
		if !ok {
			return nil, fmt.Errorf("metric `%s` of setting `share_labels` must be a mapping or set to `true`", metric)
		}
		labels, err := parseStringList(config["labels"], fmt.Sprintf("option `labels` for metric `%s` of setting `share_labels`", metric), false)
		if err != nil {
			return nil, err
		}
		match, err := parseStringList(config["match"], fmt.Sprintf("option `match` for metric `%s` of setting `share_labels`", metric), false)
		if err != nil {
			return nil, err
		}
		values, err := parseStringList(config["values"], fmt.Sprintf("option `values` for metric `%s` of setting `share_labels`", metric), true)
		if err != nil {
			return nil, err
		}
		out[metric] = types.ShareLabelsConfig{
			Labels: labels,
			Match:  match,
			Values: values,
		}
	}
	return out, nil
}

func mergeShareLabels(base, extra map[string]types.ShareLabelsConfig) map[string]types.ShareLabelsConfig {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = map[string]types.ShareLabelsConfig{}
	}
	for metric, config := range extra {
		base[metric] = config
	}
	return base
}

func labelJoinsToShareLabels(in map[string]interface{}) map[string]types.ShareLabelsConfig {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]types.ShareLabelsConfig, len(in))
	for metric, rawConfig := range in {
		config, ok := normalizeMap(rawConfig)
		if !ok {
			continue
		}
		out[metric] = types.ShareLabelsConfig{
			Match:  labelJoinMatchLabels(config),
			Labels: labelJoinLabelsToGet(config["labels_to_get"]),
			Values: []string{"1"},
		}
	}
	return out
}

func labelJoinMatchLabels(config map[string]interface{}) []string {
	rawLabels, ok := config["labels_to_match"]
	if !ok {
		rawLabels = config["label_to_match"]
	}
	labels := interfaceToStrings(rawLabels)
	if stringSliceContains(labels, "*") {
		return nil
	}
	return labels
}

func labelJoinLabelsToGet(raw interface{}) []string {
	labels := interfaceToStrings(raw)
	if stringSliceContains(labels, "*") {
		return nil
	}
	return labels
}

func legacyMetrics(metrics []interface{}, overrides map[string]string) []interface{} {
	if len(metrics) == 0 && len(overrides) == 0 {
		return metrics
	}

	remainingOverrides := copyStringMap(overrides)
	out := make([]interface{}, 0, len(metrics)+len(overrides))
	for _, metric := range metrics {
		switch value := metric.(type) {
		case string:
			data := map[string]interface{}{"name": value, "type": "legacy"}
			if override, ok := remainingOverrides[value]; ok {
				data["legacy_type_override"] = override
				delete(remainingOverrides, value)
			}
			out = append(out, map[string]interface{}{legacyMetricKey(value): data})
		case map[interface{}]interface{}:
			for rawName, rawNewName := range value {
				name, ok := rawName.(string)
				if !ok {
					continue
				}
				newName, ok := rawNewName.(string)
				if !ok {
					continue
				}
				data := map[string]interface{}{"name": newName, "type": "legacy"}
				if override, ok := remainingOverrides[name]; ok {
					data["legacy_type_override"] = override
					delete(remainingOverrides, name)
				}
				out = append(out, map[string]interface{}{legacyMetricKey(name): data})
			}
		case map[string]string:
			for name, newName := range value {
				data := map[string]interface{}{"name": newName, "type": "legacy"}
				if override, ok := remainingOverrides[name]; ok {
					data["legacy_type_override"] = override
					delete(remainingOverrides, name)
				}
				out = append(out, map[string]interface{}{legacyMetricKey(name): data})
			}
		default:
			out = append(out, metric)
		}
	}
	for name, metricType := range remainingOverrides {
		out = append(out, map[string]interface{}{legacyMetricKey(name): map[string]interface{}{"type": "legacy", "legacy_type_override": metricType}})
	}
	return out
}

func legacyMetricKey(metric string) string {
	if hasGlob(metric) {
		return globToRegex(metric)
	}
	return metric
}

func hasGlob(metric string) bool {
	return strings.ContainsAny(metric, "*?")
}

func interfaceSliceToStrings(value interface{}) []string {
	values := interfaceToStrings(value)
	if len(values) == 0 {
		return nil
	}
	return values
}

func interfaceToStrings(value interface{}) []string {
	switch values := value.(type) {
	case string:
		return []string{values}
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if str, ok := value.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func parseStringList(value interface{}, setting string, allowNumbers bool) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	values, ok := value.([]interface{})
	if !ok {
		if strings.HasPrefix(setting, "option") {
			return nil, fmt.Errorf("%s must be an array", strings.ToUpper(setting[:1])+setting[1:])
		}
		return nil, fmt.Errorf("setting `%s` must be an array", setting)
	}

	out := make([]string, 0, len(values))
	for i, value := range values {
		switch typed := value.(type) {
		case string:
			out = append(out, typed)
		case int:
			if allowNumbers {
				out = append(out, strconv.Itoa(typed))
				continue
			}
			return nil, fmt.Errorf("entry #%d of %s must be a string", i+1, setting)
		default:
			if allowNumbers {
				return nil, fmt.Errorf("entry #%d of %s must represent an integer", i+1, setting)
			}
			return nil, fmt.Errorf("entry #%d of %s must be a string", i+1, setting)
		}
	}
	return out, nil
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
