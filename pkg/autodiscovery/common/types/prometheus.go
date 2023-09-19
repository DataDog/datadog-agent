// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Default openmetrics check configuration values
	openmetricsURLPrefix   = "http://%%host%%:"
	openmetricsDefaultPort = "%%port%%"
	openmetricsDefaultPath = "/metrics"
	openmetricsDefaultNS   = ""

	// PrometheusScrapeAnnotation standard Prometheus scrape annotation key
	PrometheusScrapeAnnotation = "prometheus.io/scrape"
	// PrometheusPathAnnotation standard Prometheus path annotation key
	PrometheusPathAnnotation = "prometheus.io/path"
	// PrometheusPortAnnotation standard Prometheus port annotation key
	PrometheusPortAnnotation = "prometheus.io/port"
)

var (
	// PrometheusStandardAnnotations contains the standard Prometheus AD annotations
	PrometheusStandardAnnotations = []string{
		PrometheusScrapeAnnotation,
		PrometheusPathAnnotation,
		PrometheusPortAnnotation,
	}
	// OpenmetricsDefaultMetricsV1 containers the wildcard pattern to match all metrics
	OpenmetricsDefaultMetricsV1 = []interface{}{"*"}
	// OpenmetricsDefaultMetricsV2 containers the match-all regular expression to match all metrics
	OpenmetricsDefaultMetricsV2 = []interface{}{".*"}
)

// PrometheusCheck represents the openmetrics check instances and the corresponding autodiscovery rules
type PrometheusCheck struct {
	Instances []*OpenmetricsInstance `mapstructure:"configurations" yaml:"configurations,omitempty" json:"configurations"`
	AD        *ADConfig              `mapstructure:"autodiscovery" yaml:"autodiscovery,omitempty" json:"autodiscovery"`
}

// OpenmetricsInstance contains the openmetrics check instance fields
type OpenmetricsInstance struct {
	PrometheusURL                 string                      `mapstructure:"prometheus_url" yaml:"prometheus_url,omitempty" json:"prometheus_url,omitempty"`
	Namespace                     string                      `mapstructure:"namespace" yaml:"namespace,omitempty" json:"namespace"`
	Metrics                       []interface{}               `mapstructure:"metrics" yaml:"metrics,omitempty" json:"metrics,omitempty"`
	PromPrefix                    string                      `mapstructure:"prometheus_metrics_prefix" yaml:"prometheus_metrics_prefix,omitempty" json:"prometheus_metrics_prefix,omitempty"`
	HealthCheck                   *bool                       `mapstructure:"health_service_check" yaml:"health_service_check,omitempty" json:"health_service_check,omitempty"`
	LabelToHostname               string                      `mapstructure:"label_to_hostname" yaml:"label_to_hostname,omitempty" json:"label_to_hostname,omitempty"`
	LabelJoins                    map[string]LabelJoinsConfig `mapstructure:"label_joins" yaml:"label_joins,omitempty" json:"label_joins,omitempty"`
	LabelsMapper                  map[string]string           `mapstructure:"labels_mapper" yaml:"labels_mapper,omitempty" json:"labels_mapper,omitempty"`
	TypeOverride                  map[string]string           `mapstructure:"type_overrides" yaml:"type_overrides,omitempty" json:"type_overrides,omitempty"`
	SendHistogramBuckets          *bool                       `mapstructure:"send_histograms_buckets" yaml:"send_histograms_buckets,omitempty" json:"send_histograms_buckets,omitempty"`
	DistributionBuckets           bool                        `mapstructure:"send_distribution_buckets" yaml:"send_distribution_buckets,omitempty" json:"send_distribution_buckets,omitempty"`
	MonotonicCounter              *bool                       `mapstructure:"send_monotonic_counter" yaml:"send_monotonic_counter,omitempty" json:"send_monotonic_counter,omitempty"`
	MonotonicWithGauge            bool                        `mapstructure:"send_monotonic_with_gauge" yaml:"send_monotonic_with_gauge,omitempty" json:"send_monotonic_with_gauge,omitempty"`
	DistributionCountsAsMonotonic bool                        `mapstructure:"send_distribution_counts_as_monotonic" yaml:"send_distribution_counts_as_monotonic,omitempty" json:"send_distribution_counts_as_monotonic,omitempty"`
	DistributionSumsAsMonotonic   bool                        `mapstructure:"send_distribution_sums_as_monotonic" yaml:"send_distribution_sums_as_monotonic,omitempty" json:"send_distribution_sums_as_monotonic,omitempty"`
	ExcludeLabels                 []string                    `mapstructure:"exclude_labels" yaml:"exclude_labels,omitempty" json:"exclude_labels,omitempty"`
	BearerTokenAuth               bool                        `mapstructure:"bearer_token_auth" yaml:"bearer_token_auth,omitempty" json:"bearer_token_auth,omitempty"`
	BearerTokenPath               string                      `mapstructure:"bearer_token_path" yaml:"bearer_token_path,omitempty" json:"bearer_token_path,omitempty"`
	IgnoreMetrics                 []string                    `mapstructure:"ignore_metrics" yaml:"ignore_metrics,omitempty" json:"ignore_metrics,omitempty"`
	IgnoreMetricsByLabels         map[string]interface{}      `mapstructure:"ignore_metrics_by_labels" yaml:"ignore_metrics_by_labels,omitempty" json:"ignore_metrics_by_labels,omitempty"`
	IgnoreTags                    []string                    `mapstructure:"ignore_tags" yaml:"ignore_tags,omitempty" json:"ignore_tags,omitempty"`
	Proxy                         map[string]string           `mapstructure:"proxy" yaml:"proxy,omitempty" json:"proxy,omitempty"`
	SkipProxy                     bool                        `mapstructure:"skip_proxy" yaml:"skip_proxy,omitempty" json:"skip_proxy,omitempty"`
	Username                      string                      `mapstructure:"username" yaml:"username,omitempty" json:"username,omitempty"`
	Password                      string                      `mapstructure:"password" yaml:"password,omitempty" json:"password,omitempty"`
	TLSVerify                     *bool                       `mapstructure:"tls_verify" yaml:"tls_verify,omitempty" json:"tls_verify,omitempty"`
	TLSHostHeader                 bool                        `mapstructure:"tls_use_host_header" yaml:"tls_use_host_header,omitempty" json:"tls_use_host_header,omitempty"`
	TLSIgnoreWarn                 bool                        `mapstructure:"tls_ignore_warning" yaml:"tls_ignore_warning,omitempty" json:"tls_ignore_warning,omitempty"`
	TLSCert                       string                      `mapstructure:"tls_cert" yaml:"tls_cert,omitempty" json:"tls_cert,omitempty"`
	TLSPrivateKey                 string                      `mapstructure:"tls_private_key" yaml:"tls_private_key,omitempty" json:"tls_private_key,omitempty"`
	TLSCACert                     string                      `mapstructure:"tls_ca_cert" yaml:"tls_ca_cert,omitempty" json:"tls_ca_cert,omitempty"`
	Headers                       map[string]string           `mapstructure:"headers" yaml:"headers,omitempty" json:"headers,omitempty"`
	ExtraHeaders                  map[string]string           `mapstructure:"extra_headers" yaml:"extra_headers,omitempty" json:"extra_headers,omitempty"`
	Timeout                       int                         `mapstructure:"timeout" yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Tags                          []string                    `mapstructure:"tags" yaml:"tags,omitempty" json:"tags,omitempty"`
	Service                       string                      `mapstructure:"service" yaml:"service,omitempty" json:"service,omitempty"`
	MinCollectInterval            int                         `mapstructure:"min_collection_interval" yaml:"min_collection_interval,omitempty" json:"min_collection_interval,omitempty"`
	EmptyDefaultHost              bool                        `mapstructure:"empty_default_hostname" yaml:"empty_default_hostname,omitempty" json:"empty_default_hostname,omitempty"`
	MaxReturnedMetrics            int                         `mapstructure:"max_returned_metrics" yaml:"max_returned_metrics,omitempty" json:"max_returned_metrics,omitempty"`
	TagByEndpoint                 *bool                       `mapstructure:"tag_by_endpoint" yaml:"tag_by_endpoint,omitempty" json:"tag_by_endpoint,omitempty"`

	// openmetrics v2 specific fields
	OpenMetricsEndpoint              string                       `mapstructure:"openmetrics_endpoint" yaml:"openmetrics_endpoint,omitempty" json:"openmetrics_endpoint,omitempty"`                // Supersedes `prometheus_url`
	ExcludeMetrics                   []string                     `mapstructure:"exclude_metrics" yaml:"exclude_metrics,omitempty" json:"exclude_metrics,omitempty"`                               // Supersedes `ignore_metrics`
	ExcludeMetricsByLabels           map[string]interface{}       `mapstructure:"exclude_metrics_by_labels" yaml:"exclude_metrics_by_labels,omitempty" json:"exclude_metrics_by_labels,omitempty"` // Supersedes `ignore_metrics_by_labels`
	IncludeLabels                    []string                     `mapstructure:"include_labels" yaml:"include_labels,omitempty" json:"include_labels,omitempty"`
	RawPrefix                        string                       `mapstructure:"raw_metric_prefix" yaml:"raw_metric_prefix,omitempty" json:"raw_metric_prefix,omitempty"`                               // Supersedes `prometheus_metrics_prefix`
	EnableHealthCheck                *bool                        `mapstructure:"enable_health_service_check" yaml:"enable_health_service_check,omitempty" json:"enable_health_service_check,omitempty"` // Supersedes `health_service_check`
	RenameLabels                     map[string]string            `mapstructure:"rename_labels" yaml:"rename_labels,omitempty" json:"rename_labels,omitempty"`                                           // Supersedes `labels_mapper`
	ShareLabels                      map[string]ShareLabelsConfig `mapstructure:"share_labels" yaml:"share_labels,omitempty" json:"share_labels,omitempty"`                                              // Supersedes `label_joins`
	CacheSharedLabels                bool                         `mapstructure:"cache_shared_labels" yaml:"cache_shared_labels,omitempty" json:"cache_shared_labels,omitempty"`
	RawLineFilters                   []string                     `mapstructure:"raw_line_filters" yaml:"raw_line_filters,omitempty" json:"raw_line_filters,omitempty"`
	CollectHistogramBuckets          *bool                        `mapstructure:"collect_histogram_buckets" yaml:"collect_histogram_buckets,omitempty" json:"collect_histogram_buckets,omitempty"` // Supersedes `send_histograms_buckets`
	NonCumulativeHistogramBuckets    *bool                        `mapstructure:"non_cumulative_histogram_buckets" yaml:"non_cumulative_histogram_buckets,omitempty" json:"non_cumulative_histogram_buckets,omitempty"`
	HistogramBucketsAsDistributions  bool                         `mapstructure:"histogram_buckets_as_distributions" yaml:"histogram_buckets_as_distributions,omitempty" json:"histogram_buckets_as_distributions,omitempty"` // Supersedes `send_distribution_buckets`
	CollectCountersWithDistributions bool                         `mapstructure:"collect_counters_with_distributions" yaml:"collect_counters_with_distributions,omitempty" json:"collect_counters_with_distributions,omitempty"`
	UseProcessStartTime              *bool                        `mapstructure:"use_process_start_time" yaml:"use_process_start_time,omitempty" json:"use_process_start_time,omitempty"`
	HostnameLabel                    string                       `mapstructure:"hostname_label" yaml:"hostname_label,omitempty" json:"hostname_label,omitempty"`
	HostnameFormat                   string                       `mapstructure:"hostname_format" yaml:"hostname_format,omitempty" json:"hostname_format,omitempty"`
	CacheMetricWildcards             bool                         `mapstructure:"cache_metric_wildcards" yaml:"cache_metric_wildcards,omitempty" json:"cache_metric_wildcards,omitempty"`
	Telemetry                        *bool                        `mapstructure:"telemetry" yaml:"telemetry,omitempty" json:"telemetry,omitempty"`
	IgnoreConnectionErrors           *bool                        `mapstructure:"ignore_connection_errors" yaml:"ignore_connection_errors,omitempty" json:"ignore_connection_errors,omitempty"`
	RequestSize                      int                          `mapstructure:"request_size" yaml:"request_size,omitempty" json:"request_size,omitempty"`
	LogRequests                      *bool                        `mapstructure:"log_requests" yaml:"log_requests,omitempty" json:"log_requests,omitempty"`
	PersistConnections               *bool                        `mapstructure:"persist_connections" yaml:"persist_connections,omitempty" json:"persist_connections,omitempty"`
	AllowRedirects                   bool                         `mapstructure:"allow_redirects" yaml:"allow_redirects,omitempty" json:"allow_redirects,omitempty"`
	AuthToken                        map[string]interface{}       `mapstructure:"auth_token" yaml:"auth_token,omitempty" json:"auth_token,omitempty"`
}

// LabelJoinsConfig contains the label join configuration fields
type LabelJoinsConfig struct {
	LabelsToMatch []string `mapstructure:"labels_to_match" yaml:"labels_to_match,omitempty" json:"labels_to_match"`
	LabelsToGet   []string `mapstructure:"labels_to_get" yaml:"labels_to_get,omitempty" json:"labels_to_get"`
}

// ShareLabelsConfig contains the share labels fields
type ShareLabelsConfig struct {
	Labels []string `mapstructure:"labels" yaml:"labels,omitempty" json:"labels,omitempty"`
	Match  []string `mapstructure:"match" yaml:"match,omitempty" json:"match,omitempty"`
	Values []string `mapstructure:"values" yaml:"values,omitempty" json:"values,omitempty"`
}

// ADConfig contains the autodiscovery configuration data for a PrometheusCheck
type ADConfig struct {
	KubeAnnotations    *InclExcl      `mapstructure:"kubernetes_annotations,omitempty" yaml:"kubernetes_annotations,omitempty" json:"kubernetes_annotations,omitempty"`
	KubeContainerNames []string       `mapstructure:"kubernetes_container_names,omitempty" yaml:"kubernetes_container_names,omitempty" json:"kubernetes_container_names,omitempty"`
	ContainersRe       *regexp.Regexp `mapstructure:",omitempty" yaml:",omitempty"`
}

// InclExcl contains the include/exclude data structure
type InclExcl struct {
	Incl map[string]string `mapstructure:"include" yaml:"include,omitempty" json:"include,omitempty"`
	Excl map[string]string `mapstructure:"exclude" yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// Init prepares the PrometheusCheck structure and defaults its values
// init must be called only once
func (pc *PrometheusCheck) Init(version int) error {
	pc.initInstances(version)
	return pc.initAD()
}

// initInstances defaults the Instances field in PrometheusCheck
func (pc *PrometheusCheck) initInstances(version int) {
	var openmetricsDefaultMetrics []interface{}
	switch version {
	case 1:
		openmetricsDefaultMetrics = OpenmetricsDefaultMetricsV1
	case 2:
		openmetricsDefaultMetrics = OpenmetricsDefaultMetricsV2
	default:
		log.Errorf("Invalid value for `prometheus_scrape.version`: %d. Can only be 1 or 2.", version)
		return
	}

	if len(pc.Instances) == 0 {
		// Put a default config
		pc.Instances = append(pc.Instances, &OpenmetricsInstance{
			Metrics:   openmetricsDefaultMetrics,
			Namespace: openmetricsDefaultNS,
		})
		return
	}

	for _, instance := range pc.Instances {
		// Default the required config values if not set
		if len(instance.Metrics) == 0 {
			instance.Metrics = openmetricsDefaultMetrics
		}
	}
}

// initAD defaults the AD field in PrometheusCheck
// It also prepares the regex to match the containers by name
func (pc *PrometheusCheck) initAD() error {
	if pc.AD == nil {
		pc.AD = &ADConfig{}
	}

	pc.AD.defaultAD()
	return pc.AD.setContainersRegex()
}

// IsExcluded returns whether the annotations match an AD exclusion rule
func (pc *PrometheusCheck) IsExcluded(annotations map[string]string, namespacedName string) bool {
	for k, v := range pc.AD.KubeAnnotations.Excl {
		if annotations[k] == v {
			log.Debugf("'%s' matched the exclusion annotation '%s=%s' ignoring it", namespacedName, k, v)
			return true
		}
	}
	return false
}

// IsIncluded returns whether the annotations match an AD inclusion rule and is not excluded
func (pc *PrometheusCheck) IsIncluded(annotations map[string]string) bool {
	included := false
	if pc.AD == nil || pc.AD.KubeAnnotations == nil {
		return false
	}

	for k, v := range annotations {
		if pc.AD.KubeAnnotations.Excl[k] == v {
			return false
		}

		if pc.AD.KubeAnnotations.Incl[k] == v {
			included = true
		}
	}

	return included
}

// GetIncludeAnnotations returns the AD include annotations
func (ad *ADConfig) GetIncludeAnnotations() map[string]string {
	annotations := map[string]string{}
	if ad.KubeAnnotations != nil && ad.KubeAnnotations.Incl != nil {
		return ad.KubeAnnotations.Incl
	}
	return annotations
}

// GetExcludeAnnotations returns the AD exclude annotations
func (ad *ADConfig) GetExcludeAnnotations() map[string]string {
	annotations := map[string]string{}
	if ad.KubeAnnotations != nil && ad.KubeAnnotations.Excl != nil {
		return ad.KubeAnnotations.Excl
	}
	return annotations
}

// defaultAD defaults the values of the autodiscovery structure
func (ad *ADConfig) defaultAD() {
	if ad.KubeContainerNames == nil {
		ad.KubeContainerNames = []string{}
	}

	if ad.KubeAnnotations == nil {
		ad.KubeAnnotations = &InclExcl{
			Excl: map[string]string{PrometheusScrapeAnnotation: "false"},
			Incl: map[string]string{PrometheusScrapeAnnotation: "true"},
		}
		return
	}

	if ad.KubeAnnotations.Excl == nil {
		ad.KubeAnnotations.Excl = map[string]string{PrometheusScrapeAnnotation: "false"}
	}

	if ad.KubeAnnotations.Incl == nil {
		ad.KubeAnnotations.Incl = map[string]string{PrometheusScrapeAnnotation: "true"}
	}
}

// setContainersRegex precompiles the regex to match the container names for autodiscovery
// returns an error if the container names cannot be converted to a valid regex
func (ad *ADConfig) setContainersRegex() error {
	ad.ContainersRe = nil
	if len(ad.KubeContainerNames) == 0 {
		return nil
	}

	regexString := strings.Join(ad.KubeContainerNames, "|")
	re, err := regexp.Compile(regexString)
	if err != nil {
		return fmt.Errorf("Invalid container names - regex: '%s': %v", regexString, err)
	}

	ad.ContainersRe = re
	return nil
}

// MatchContainer returns whether a container name matches the 'kubernetes_container_names' configuration
func (ad *ADConfig) MatchContainer(name string) bool {
	if ad.ContainersRe == nil {
		return true
	}
	return ad.ContainersRe.MatchString(name)
}

// DefaultPrometheusCheck has the default openmetrics check values
// To be used when the checks configuration is empty
var DefaultPrometheusCheck = &PrometheusCheck{
	Instances: []*OpenmetricsInstance{
		{
			Metrics:   []interface{}{"PLACEHOLDER"},
			Namespace: openmetricsDefaultNS,
		},
	},
	AD: &ADConfig{
		KubeAnnotations: &InclExcl{
			Excl: map[string]string{PrometheusScrapeAnnotation: "false"},
			Incl: map[string]string{PrometheusScrapeAnnotation: "true"},
		},
		KubeContainerNames: []string{},
	},
}

// BuildURL returns the 'prometheus_url' based on the default values
// and the prometheus path and port annotations
func BuildURL(annotations map[string]string) string {
	port := openmetricsDefaultPort
	if portFromAnnotation, found := annotations[PrometheusPortAnnotation]; found {
		port = portFromAnnotation
	}

	path := openmetricsDefaultPath
	if pathFromAnnotation, found := annotations[PrometheusPathAnnotation]; found {
		path = pathFromAnnotation
	}

	return openmetricsURLPrefix + port + path
}

// PrometheusAnnotations abstracts a map of prometheus annotations
type PrometheusAnnotations map[string]string

// IsMatchingAnnotations returns whether annotations matches the AD include rules for Prometheus
func (a PrometheusAnnotations) IsMatchingAnnotations(svcAnnotations map[string]string) bool {
	for k, v := range a {
		if svcAnnotations[k] == v {
			return true
		}
	}
	return false
}

// AnnotationsDiffer returns whether the Prometheus AD include annotations have changed
func (a PrometheusAnnotations) AnnotationsDiffer(first, second map[string]string) bool {
	for k := range a {
		if first[k] != second[k] {
			return true
		}
	}
	return false
}
