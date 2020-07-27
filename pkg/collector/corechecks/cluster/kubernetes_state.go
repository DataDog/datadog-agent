// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/pkg/options"
)

const (
	// TODO rename correctly once we deprecate the python check
	kubeStateMetricsCheckName = "kubernetes_state-alpha"
	defaultResyncPeriod       = 30
)

// KSMConfig contains the check config parameters
type KSMConfig struct {
	// Collectors defines the resource type collectors.
	// Example: Enable pods and nodes collectors.
	// collectors:
	//   - nodes
	//   - pods
	Collectors []string `yaml:"collectors"`

	// LabelJoins allows adding the tags to join from other KSM metrics.
	// Example: Joining for deployment metrics. Based on:
	// kube_deployment_labels{deployment="kube-dns",label_addonmanager_kubernetes_io_mode="Reconcile"}
	// Use the following config to add the value of label_addonmanager_kubernetes_io_mode as a tag to your KSM
	// deployment metrics.
	// label_joins:
	//   kube_deployment_labels:
	//     label_to_match: deployment
	//     labels_to_get:
	//       - label_addonmanager_kubernetes_io_mode
	LabelJoins map[string]*JoinsConfig `yaml:"label_joins"`

	// LabelsMapper can be used to translate kube-state-metrics labels to other tags.
	// Example: Adding kube_namespace tag instead of namespace.
	// labels_mapper:
	//   namespace: kube_namespace
	LabelsMapper map[string]string `yaml:"labels_mapper"`

	// Namespaces contains the namespaces from which we collect metrics
	// Example: Enable metric collection for objects in prod and kube-system namespaces.
	// namespaces:
	//   - prod
	//   - kube-system
	Namespaces []string `yaml:"namespaces"`

	// ResyncPeriod is the frequency of resync'ing the metrics cache in seconds, default 30.
	ResyncPeriod int `yaml:"resync_period"`
}

// KSMCheck wraps the config and the metric stores needed to run the check
type KSMCheck struct {
	core.CheckBase
	instance *KSMConfig
	store    []cache.Store
}

// JoinsConfig contains the config parameters for label joins
type JoinsConfig struct {
	// LabelsToMatch contains the labels that must
	// match the labels of the targeted metric
	LabelsToMatch []string `yaml:"labels_to_match"`

	// LabelsToGet contains the labels we want to get from the targeted metric
	LabelsToGet []string `yaml:"labels_to_get"`

	// GetAllLabels replaces LabelsToGet if enabled
	GetAllLabels bool `yaml:"get_all_labels"`
}

func (jc *JoinsConfig) setupGetAllLabels() {
	if jc.GetAllLabels {
		return
	}

	for _, l := range jc.LabelsToGet {
		if l == "*" {
			jc.GetAllLabels = true
			return
		}
	}
}

func init() {
	core.RegisterCheck(kubeStateMetricsCheckName, KubeStateMetricsFactory)
}

// Configure prepares the configuration of the KSM check instance
func (k *KSMCheck) Configure(config, initConfig integration.Data, source string) error {
	err := k.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	err = k.instance.parse(config)
	if err != nil {
		return err
	}

	// Prepare label joins
	for _, joinConf := range k.instance.LabelJoins {
		joinConf.setupGetAllLabels()
	}

	k.mergeLabelJoins(defaultLabelJoins)

	// Prepare labels mapper
	k.mergeLabelsMapper(defaultLabelsMapper)

	builder := kubestatemetrics.New()

	// Prepare the collectors for the resources specified in the configuration file.
	collectors := k.instance.Collectors

	// Enable the KSM default collectors if the config collectors list is empty.
	if len(collectors) == 0 {
		collectors = options.DefaultResources.AsSlice()
	}

	if err := builder.WithEnabledResources(collectors); err != nil {
		return err
	}

	// Prepare watched namespaces
	namespaces := k.instance.Namespaces

	// Enable the KSM default namespaces if the config namespaces list is empty.
	if len(namespaces) == 0 {
		namespaces = options.DefaultNamespaces
	}

	builder.WithNamespaces(namespaces)

	allowDenyList, err := allowdenylist.New(options.MetricSet{}, deniedMetrics)
	if err != nil {
		return err
	}

	if err := allowDenyList.Parse(); err != nil {
		return err
	}

	builder.WithAllowDenyList(allowDenyList)

	c, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}

	builder.WithKubeClient(c.Cl)
	builder.WithContext(context.Background())

	resyncPeriod := k.instance.ResyncPeriod
	if resyncPeriod == 0 {
		resyncPeriod = defaultResyncPeriod
	}

	builder.WithResync(time.Duration(resyncPeriod) * time.Second)

	builder.WithGenerateStoreFunc(builder.GenerateStore)

	// Start the collection process
	k.store = builder.Build()

	return nil
}

func (c *KSMConfig) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Run runs the KSM check
func (k *KSMCheck) Run() error {
	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}

	defer sender.Commit()

	metricsToGet := []ksmstore.DDMetricsFam{}
	for _, store := range k.store {
		metrics := store.(*ksmstore.MetricsStore).Push(k.familyFilter, k.metricFilter)
		for _, m := range metrics {
			metricsToGet = append(metricsToGet, m...)
		}
	}

	for _, store := range k.store {
		metrics := store.(*ksmstore.MetricsStore).Push(ksmstore.GetAllFamilies, ksmstore.GetAllMetrics)
		k.processMetrics(sender, metrics, metricsToGet)
	}

	return nil
}

// processMetrics attaches tags and forwards metrics to the aggregator
func (k *KSMCheck) processMetrics(sender aggregator.Sender, metrics map[string][]ksmstore.DDMetricsFam, metricsToGet []ksmstore.DDMetricsFam) {
	for _, metricsList := range metrics {
		for _, metricFamily := range metricsList {
			if metadataMetricsRegex.MatchString(metricFamily.Name) {
				// metadata metrics are only used by the check for label joins
				// they shouldn't be forwarded to Datadog
				continue
			}
			if transform, found := metricTransformers[metricFamily.Name]; found {
				for _, m := range metricFamily.ListMetrics {
					// TODO: implement metric transformer functions
					transform(sender, metricFamily.Name, m, k.joinLabels(m.Labels, metricsToGet))
				}
				continue
			}
			for _, m := range metricFamily.ListMetrics {
				sender.Gauge(formatMetricName(metricFamily.Name), m.Val, "", k.joinLabels(m.Labels, metricsToGet))
			}
		}
	}
}

// joinLabels converts metric labels into datatog tags and applies the label joins config
func (k *KSMCheck) joinLabels(labels map[string]string, metricsToGet []ksmstore.DDMetricsFam) (tags []string) {
	for key, value := range labels {
		tags = append(tags, k.buildTag(key, value))
	}

	// apply label joins
	for _, mFamily := range metricsToGet {
		config, found := k.instance.LabelJoins[mFamily.Name]
		if !found {
			continue
		}
		for _, m := range mFamily.ListMetrics {
			if isMatching(config, labels, m.Labels) {
				tags = append(tags, k.getJoinedTags(config, m.Labels)...)
			}
		}
	}

	return tags
}

// familyFilter is a metric families filter for label joins
// It ensures that we only get the configured metric names to
// get labels based on the label joins config
func (k *KSMCheck) familyFilter(f ksmstore.DDMetricsFam) bool {
	_, found := k.instance.LabelJoins[f.Name]
	return found
}

// metricFilter is a metrics filter for label joins
// It ensures that we only get metadata-only metrics for label joins
// metadata-only metrics that are used for label joins are always equal to 1
// this is required for metrics where all combinations of a state are sent
// but only the active one is set to 1 (others are set to 0)
// example: kube_pod_status_phase
func (k *KSMCheck) metricFilter(m ksmstore.DDMetric) bool {
	return m.Val == float64(1)
}

// isMatching returns whether a targeted metric for label joins is
// matching another metric labels based on the labels to match config
func isMatching(config *JoinsConfig, destlabels, srcLabels map[string]string) bool {
	for _, l := range config.LabelsToMatch {
		firstVal, found := destlabels[l]
		if !found {
			return false
		}
		secondVal, found := srcLabels[l]
		if !found {
			return false
		}
		if firstVal != secondVal {
			return false
		}
	}
	return true
}

// buildTag applies the LabelsMapper config and returns the tag in a key:value string format
func (k *KSMCheck) buildTag(key, value string) string {
	if newKey, found := k.instance.LabelsMapper[key]; found {
		key = newKey
	}
	return fmt.Sprintf("%s:%s", key, value)
}

// getJoinedTags applies the label joins config, it gets labels from a targeted metric labels
func (k *KSMCheck) getJoinedTags(config *JoinsConfig, srcLabels map[string]string) []string {
	tags := []string{}
	if config.GetAllLabels {
		for key, value := range srcLabels {
			tags = append(tags, k.buildTag(key, value))
		}
		return tags
	}
	for _, key := range config.LabelsToGet {
		if value, found := srcLabels[key]; found {
			tags = append(tags, k.buildTag(key, value))
		}
	}
	return tags
}

// mergeLabelsMapper adds extra label mappings to the configured labels mapper
// User-defined mappings are prioritized over additional mappings
func (k *KSMCheck) mergeLabelsMapper(extra map[string]string) {
	for key, value := range extra {
		if _, found := k.instance.LabelsMapper[key]; !found {
			k.instance.LabelsMapper[key] = value
		}
	}
}

// mergeLabelJoins adds extra label joins to the configured label joins
// User-defined label joins are prioritized over additional label joins
func (k *KSMCheck) mergeLabelJoins(extra map[string]*JoinsConfig) {
	for key, value := range extra {
		if _, found := k.instance.LabelJoins[key]; !found {
			k.instance.LabelJoins[key] = value
		}
	}
}

func KubeStateMetricsFactory() check.Check {
	return newKSMCheck(
		core.NewCheckBase(kubeStateMetricsCheckName),
		&KSMConfig{
			LabelsMapper: make(map[string]string),
			LabelJoins:   make(map[string]*JoinsConfig),
			Namespaces:   []string{},
		})
}

func newKSMCheck(base core.CheckBase, instance *KSMConfig) *KSMCheck {
	return &KSMCheck{
		CheckBase: base,
		instance:  instance,
	}
}

// formatMetricName converts the default KSM metric names into Datadog metric names
func formatMetricName(name string) string {
	if ddName, found := metricNamesMapper[name]; found {
		return ksmMetricPrefix + ddName
	}
	log.Tracef("KSM metric '%s' is not found in the metric names mapper", name)
	return ksmMetricPrefix + name
}
