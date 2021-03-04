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
	openmetricsDefaultMetrics = []string{"*"}
)

// PrometheusCheck represents the openmetrics check instances and the corresponding autodiscovery rules
type PrometheusCheck struct {
	Instances []*OpenmetricsInstance `mapstructure:"configurations" yaml:"configurations,omitempty" json:"configurations"`
	AD        *ADConfig              `mapstructure:"autodiscovery" yaml:"autodiscovery,omitempty" json:"autodiscovery"`
}

// OpenmetricsInstance contains the openmetrics check instance fields
type OpenmetricsInstance struct {
	URL                           string                      `mapstructure:"prometheus_url" yaml:"prometheus_url,omitempty" json:"prometheus_url,omitempty"`
	Namespace                     string                      `mapstructure:"namespace" yaml:"namespace,omitempty" json:"namespace"`
	Metrics                       []string                    `mapstructure:"metrics" yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Prefix                        string                      `mapstructure:"prometheus_metrics_prefix" yaml:"prometheus_metrics_prefix,omitempty" json:"prometheus_metrics_prefix,omitempty"`
	HealthCheck                   bool                        `mapstructure:"health_service_check" yaml:"health_service_check,omitempty" json:"health_service_check,omitempty"`
	LabelToHostname               bool                        `mapstructure:"label_to_hostname" yaml:"label_to_hostname,omitempty" json:"label_to_hostname,omitempty"`
	LabelJoins                    map[string]LabelJoinsConfig `mapstructure:"label_joins" yaml:"label_joins,omitempty" json:"label_joins,omitempty"`
	LabelsMapper                  map[string]string           `mapstructure:"labels_mapper" yaml:"labels_mapper,omitempty" json:"labels_mapper,omitempty"`
	TypeOverride                  map[string]string           `mapstructure:"type_overrides" yaml:"type_overrides,omitempty" json:"type_overrides,omitempty"`
	HistogramBuckets              bool                        `mapstructure:"send_histograms_buckets" yaml:"send_histograms_buckets,omitempty" json:"send_histograms_buckets,omitempty"`
	DistributionBuckets           bool                        `mapstructure:"send_distribution_buckets" yaml:"send_distribution_buckets,omitempty" json:"send_distribution_buckets,omitempty"`
	MonotonicCounter              bool                        `mapstructure:"send_monotonic_counter" yaml:"send_monotonic_counter,omitempty" json:"send_monotonic_counter,omitempty"`
	DistributionCountsAsMonotonic bool                        `mapstructure:"send_distribution_counts_as_monotonic" yaml:"send_distribution_counts_as_monotonic,omitempty" json:"send_distribution_counts_as_monotonic,omitempty"`
	DistributionSumsAsMonotonic   bool                        `mapstructure:"send_distribution_sums_as_monotonic" yaml:"send_distribution_sums_as_monotonic,omitempty" json:"send_distribution_sums_as_monotonic,omitempty"`
	ExcludeLabels                 []string                    `mapstructure:"exclude_labels" yaml:"exclude_labels,omitempty" json:"exclude_labels,omitempty"`
	BearerTokenAuth               bool                        `mapstructure:"bearer_token_auth" yaml:"bearer_token_auth,omitempty" json:"bearer_token_auth,omitempty"`
	BearerTokenPath               string                      `mapstructure:"bearer_token_path" yaml:"bearer_token_path,omitempty" json:"bearer_token_path,omitempty"`
	IgnoreMetrics                 []string                    `mapstructure:"ignore_metrics" yaml:"ignore_metrics,omitempty" json:"ignore_metrics,omitempty"`
	Proxy                         map[string]string           `mapstructure:"proxy" yaml:"proxy,omitempty" json:"proxy,omitempty"`
	SkipProxy                     bool                        `mapstructure:"skip_proxy" yaml:"skip_proxy,omitempty" json:"skip_proxy,omitempty"`
	Username                      string                      `mapstructure:"username" yaml:"username,omitempty" json:"username,omitempty"`
	Password                      string                      `mapstructure:"password" yaml:"password,omitempty" json:"password,omitempty"`
	TLSVerify                     bool                        `mapstructure:"tls_verify" yaml:"tls_verify,omitempty" json:"tls_verify,omitempty"`
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
}

// LabelJoinsConfig contains the label join configuration fields
type LabelJoinsConfig struct {
	LabelsToMatch []string `mapstructure:"labels_to_match" yaml:"labels_to_match,omitempty" json:"labels_to_match"`
	LabelsToGet   []string `mapstructure:"labels_to_get" yaml:"labels_to_get,omitempty" json:"labels_to_get"`
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
func (pc *PrometheusCheck) Init() error {
	pc.initInstances()
	return pc.initAD()
}

// initInstances defaults the Instances field in PrometheusCheck
func (pc *PrometheusCheck) initInstances() {
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

// IsExcluded returns whether is the annotations match an AD exclusion rule
func (pc *PrometheusCheck) IsExcluded(annotations map[string]string, namespacedName string) bool {
	for k, v := range pc.AD.KubeAnnotations.Excl {
		if annotations[k] == v {
			log.Debugf("'%s' matched the exclusion annotation '%s=%s' ignoring it", namespacedName, k, v)
			return true
		}
	}
	return false
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
			Metrics:   openmetricsDefaultMetrics,
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
