// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build openmetrics_differential

package differential

import (
	"fmt"
	"math/rand"
)

// ConfigMutator generates valid-shaped OpenMetrics instance configs to use as
// the second axis of differential testing. The first axis (the payload
// mutator + adversarial catalog + fuzz) explores parser behavior; this axis
// explores transformer / matcher / label-pipeline behavior.
//
// Deterministic given a seed: ConfigMutator(seed).NewConfig(n) is reproducible.
//
// Each knob is a small, named function that applies one modification to a
// baseline config. NewConfig picks a baseline, then applies N random knobs.
// The named-knob design makes triage easier: when something diverges, we know
// which knob produced the diverging config.
type ConfigMutator struct {
	rng         *rand.Rand
	metricNames []string
	labelNames  []string
}

// NewConfigMutator seeds a config mutator using the KSM fixture vocabulary.
func NewConfigMutator(seed int64) *ConfigMutator {
	return NewConfigMutatorForFixture(seed, "ksm/wildcard")
}

// NewConfigMutatorForFixture seeds a config mutator with names that occur in
// the selected fixture. Using names from a different fixture turns many config
// knobs into no-ops and produces misleading attribution.
func NewConfigMutatorForFixture(seed int64, fixtureName string) *ConfigMutator {
	metricNames := kubeStateMetricNames
	labelNames := kubeStateLabelNames
	if fixtureName == "msk_jmx/wildcard" {
		metricNames = mskMetricNames
		labelNames = mskLabelNames
	}
	return &ConfigMutator{
		rng:         rand.New(rand.NewSource(seed)),
		metricNames: metricNames,
		labelNames:  labelNames,
	}
}

// baseConfig is the minimum-viable starting point. Every generated config
// inherits these and may override or extend them.
func baseConfig() map[string]interface{} {
	return map[string]interface{}{
		"namespace": "diff",
		"metrics":   []interface{}{".+"},
	}
}

// configKnob is one named modification. AppliedKnobs slice on NewConfig output
// records which were applied so divergent test logs can attribute the change.
type configKnob struct {
	name  string
	apply func(m *ConfigMutator, cfg map[string]interface{})
}

var allConfigKnobs = []configKnob{
	// -- metric matching shape --
	{"matching/named_list", knobNamedMetricList},
	{"matching/rename_map", knobRenameMap},
	{"matching/mixed_regex_and_rename", knobMixedMatchers},
	{"matching/narrow_regex", knobNarrowRegex},

	// -- exclusion --
	{"exclude/by_name", knobExcludeByName},
	{"exclude/by_label", knobExcludeByLabel},
	{"exclude/ignore_metrics_alias", knobIgnoreMetricsAlias},

	// -- labels / tags --
	{"labels/rename", knobRenameLabels},
	{"labels/exclude", knobExcludeLabels},
	{"labels/include", knobIncludeLabels},
	{"labels/ignore_tags", knobIgnoreTags},
	{"labels/tag_by_endpoint_false", knobTagByEndpointFalse},

	// -- transformer shape --
	{"transformer/type_overrides", knobTypeOverrides},
	{"transformer/raw_metric_prefix", knobRawMetricPrefix},
	{"transformer/send_histograms_buckets_false", knobNoHistogramBuckets},
	{"transformer/non_cumulative_buckets", knobNonCumulativeBuckets},
	{"transformer/send_distribution_buckets", knobDistributionBuckets},
	{"transformer/histogram_as_distributions", knobHistogramAsDistributions},
	{"transformer/send_monotonic_with_gauge", knobMonotonicWithGauge},
	{"transformer/send_monotonic_counter_false", knobNoMonotonicCounter},

	// -- joins --
	{"join/share_labels", knobShareLabels},

	// -- health --
	{"health/disable_service_check", knobDisableHealthCheck},
}

// GeneratedConfig couples the config map with the names of knobs that
// produced it, for triage.
type GeneratedConfig struct {
	Config       map[string]interface{}
	AppliedKnobs []string
}

// NewConfig produces a fresh config: baseline plus `numKnobs` random knobs
// applied (with replacement — same knob may apply twice, which usually means
// the second application overrides the first).
func (c *ConfigMutator) NewConfig(numKnobs int) GeneratedConfig {
	cfg := baseConfig()
	applied := make([]string, 0, numKnobs)
	for i := 0; i < numKnobs; i++ {
		knob := allConfigKnobs[c.rng.Intn(len(allConfigKnobs))]
		knob.apply(c, cfg)
		applied = append(applied, knob.name)
	}
	return GeneratedConfig{Config: cfg, AppliedKnobs: applied}
}

// ---- knob implementations ----------------------------------------------------

// kubeStateMetricNames is a sampling of metric names from the KSM corpus
// fixture; used by knobs that need realistic-looking names so the resulting
// config actually matches something in our payloads.
var kubeStateMetricNames = []string{
	"kube_pod_status_phase",
	"kube_pod_container_status_running",
	"kube_pod_container_status_waiting",
	"kube_pod_container_status_terminated",
	"kube_pod_container_status_ready",
	"kube_deployment_status_replicas",
	"kube_deployment_status_replicas_available",
	"kube_deployment_labels",
	"kube_node_labels",
	"kube_node_info",
	"kube_service_info",
	"kube_daemonset_status_current_number_scheduled",
	"kube_replicaset_status_ready_replicas",
}

var kubeStateLabelNames = []string{
	"namespace", "pod", "container", "node", "deployment",
	"daemonset", "replicaset", "service", "phase", "reason",
}

var mskMetricNames = []string{
	"kafka_server_FetcherStats_FiveMinuteRate",
	"kafka_server_FetcherStats_Count",
	"kafka_server_ReplicaFetcherManager_Value",
	"kafka_log_Log_Value",
	"kafka_cluster_Partition_Value",
	"kafka_server_BrokerTopicMetrics_OneMinuteRate",
	"kafka_network_RequestMetrics_MeanRate",
	"kafka_network_RequestMetrics_StdDev",
}

var mskLabelNames = []string{
	"brokerHost", "brokerPort", "clientId", "name", "partition", "topic",
	"error", "request", "version",
}

func (c *ConfigMutator) pickMetricName() string {
	return c.metricNames[c.rng.Intn(len(c.metricNames))]
}

func (c *ConfigMutator) pickLabelName() string {
	return c.labelNames[c.rng.Intn(len(c.labelNames))]
}

// ---- matching ----

func knobNamedMetricList(c *ConfigMutator, cfg map[string]interface{}) {
	count := 3 + c.rng.Intn(5)
	names := make([]interface{}, 0, count)
	for i := 0; i < count; i++ {
		names = append(names, c.pickMetricName())
	}
	cfg["metrics"] = names
}

func knobRenameMap(c *ConfigMutator, cfg map[string]interface{}) {
	count := 2 + c.rng.Intn(3)
	metrics := []interface{}{}
	for i := 0; i < count; i++ {
		name := c.pickMetricName()
		rename := fmt.Sprintf("renamed.%s", name)
		metrics = append(metrics, map[string]interface{}{name: rename})
	}
	cfg["metrics"] = metrics
}

func knobMixedMatchers(c *ConfigMutator, cfg map[string]interface{}) {
	metrics := []interface{}{
		".+_status_.+", // regex
		c.pickMetricName(),
		map[string]interface{}{c.pickMetricName(): "renamed.metric"},
	}
	cfg["metrics"] = metrics
}

func knobNarrowRegex(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["metrics"] = []interface{}{"kube_pod_.*", "kube_node_.*"}
}

// ---- exclusion ----

func knobExcludeByName(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["exclude_metrics"] = []interface{}{c.pickMetricName(), c.pickMetricName()}
}

func knobExcludeByLabel(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["exclude_metrics_by_labels"] = map[string]interface{}{
		c.pickLabelName(): []interface{}{"kube-system", "default"},
	}
}

func knobIgnoreMetricsAlias(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["ignore_metrics"] = []interface{}{c.pickMetricName()}
}

// ---- labels / tags ----

func knobRenameLabels(c *ConfigMutator, cfg map[string]interface{}) {
	renames := map[string]interface{}{}
	count := 1 + c.rng.Intn(3)
	for i := 0; i < count; i++ {
		src := c.pickLabelName()
		dst := "renamed_" + src
		renames[src] = dst
	}
	cfg["rename_labels"] = renames
}

func knobExcludeLabels(c *ConfigMutator, cfg map[string]interface{}) {
	count := 1 + c.rng.Intn(3)
	excluded := make([]interface{}, 0, count)
	for i := 0; i < count; i++ {
		excluded = append(excluded, c.pickLabelName())
	}
	cfg["exclude_labels"] = excluded
}

func knobIncludeLabels(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["include_labels"] = []interface{}{c.pickLabelName(), c.pickLabelName()}
}

func knobIgnoreTags(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["ignore_tags"] = []interface{}{c.pickLabelName() + ":.*"}
}

func knobTagByEndpointFalse(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["tag_by_endpoint"] = false
}

// ---- transformer ----

var typeOverrideValues = []string{"gauge", "counter", "monotonic_count"}

func knobTypeOverrides(c *ConfigMutator, cfg map[string]interface{}) {
	overrides := map[string]interface{}{}
	count := 1 + c.rng.Intn(3)
	for i := 0; i < count; i++ {
		overrides[c.pickMetricName()] = typeOverrideValues[c.rng.Intn(len(typeOverrideValues))]
	}
	cfg["type_overrides"] = overrides
}

func knobRawMetricPrefix(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["raw_metric_prefix"] = "prom."
}

func knobNoHistogramBuckets(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["send_histograms_buckets"] = false
}

func knobNonCumulativeBuckets(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["non_cumulative_buckets"] = true
}

func knobDistributionBuckets(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["send_distribution_buckets"] = true
}

func knobHistogramAsDistributions(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["histogram_buckets_as_distributions"] = true
}

func knobMonotonicWithGauge(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["send_monotonic_with_gauge"] = true
}

func knobNoMonotonicCounter(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["send_monotonic_counter"] = false
}

// ---- joins ----

func knobShareLabels(c *ConfigMutator, cfg map[string]interface{}) {
	// share_labels is keyed by the source metric. `match` lists join-key
	// labels and `labels` lists labels copied from that source. Keep a valid,
	// output-changing join for each fixture; absent source metrics are ignored.
	cfg["share_labels"] = map[string]interface{}{
		"kube_pod_info": map[string]interface{}{
			"match":  []interface{}{"namespace", "pod"},
			"labels": []interface{}{"node", "pod_ip"},
		},
		"kafka_server_FetcherStats_FiveMinuteRate": map[string]interface{}{
			"match":  []interface{}{"clientId", "name"},
			"labels": []interface{}{"brokerHost", "brokerPort"},
		},
	}
}

// ---- health ----

func knobDisableHealthCheck(c *ConfigMutator, cfg map[string]interface{}) {
	cfg["enable_health_service_check"] = false
}
