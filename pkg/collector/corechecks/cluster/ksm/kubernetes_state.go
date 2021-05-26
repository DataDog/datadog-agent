// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build kubeapiserver

package ksm

import (
	"context"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/options"
)

const (
	kubeStateMetricsCheckName = "kubernetes_state_core"
	maximumWaitForAPIServer   = 10 * time.Second
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

	// Tags contains the list of tags to attach to every metric, event and service check emitted by this integration.
	// Example:
	// tags:
	//   - env:prod
	//   - zone:eu
	Tags []string `yaml:"tags"`

	// Namespaces contains the namespaces from which we collect metrics
	// Example: Enable metric collection for objects in prod and kube-system namespaces.
	// namespaces:
	//   - prod
	//   - kube-system
	Namespaces []string `yaml:"namespaces"`

	// ResyncPeriod is the frequency of resync'ing the metrics cache in seconds, default 5 minutes (kubernetes_informers_resync_period).
	ResyncPeriod int `yaml:"resync_period"`

	// Telemetry enables telemetry check's metrics, default false.
	// Metrics can be found under kubernetes_state.telemetry
	Telemetry bool `yaml:"telemetry"`

	// LeaderSkip forces ignoring the leader election when running the check
	// Can be useful when running the check as cluster check
	LeaderSkip bool `yaml:"skip_leader_election"`
}

// KSMCheck wraps the config and the metric stores needed to run the check
type KSMCheck struct {
	core.CheckBase
	instance    *KSMConfig
	store       []cache.Store
	telemetry   *telemetryCache
	cancel      context.CancelFunc
	isCLCRunner bool
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
	k.BuildID(config, initConfig)

	err := k.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	err = k.CommonConfigure(initConfig, source)
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

	k.initTags()

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

	allowDenyList, err := allowdenylist.New(options.MetricSet{}, buildDeniedMetricsSet(collectors))
	if err != nil {
		return err
	}

	if err := allowDenyList.Parse(); err != nil {
		return err
	}

	builder.WithAllowDenyList(allowDenyList)

	// Due to how init is done, we cannot use GetAPIClient in `Run()` method
	// So we are waiting for a reasonable amount of time here in case.
	// We cannot wait forever as there's no way to be notified of shutdown
	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()
	c, err := apiserver.WaitForAPIClient(apiCtx)
	if err != nil {
		return err
	}

	builder.WithKubeClient(c.Cl)

	ctx, cancel := context.WithCancel(context.Background())
	k.cancel = cancel
	builder.WithContext(ctx)

	resyncPeriod := k.instance.ResyncPeriod
	if resyncPeriod == 0 {
		resyncPeriod = ddconfig.Datadog.GetInt("kubernetes_informers_resync_period")
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

	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// we also do a safety check for dedicated runners to avoid trying the leader election
	if !k.isCLCRunner || !k.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !config.Datadog.GetBool("leader_election") {
			return log.Error("Leader Election not enabled. The cluster-agent will not run the kube-state-metrics core check.")
		}

		leader, errLeader := cluster.RunLeaderElection()
		if errLeader != nil {
			if errLeader == apiserver.ErrNotLeader {
				log.Debugf("Not leader (leader is %q). Skipping the kube-state-metrics core check", leader)
				return nil
			}

			_ = k.Warn("Leader Election error. Not running the kube-state-metrics core check.")
			return err
		}

		log.Tracef("Current leader: %q, running kube-state-metrics core check", leader)
	}

	defer sender.Commit()

	// Do not fallback to the Agent hostname if the hostname corresponding to the KSM metric is unknown
	// Note that by design, some metrics cannot have hostnames (e.g kubernetes_state.pod.unschedulable)
	sender.DisableDefaultHostname(true)

	labelJoiner := newLabelJoiner(k.instance.LabelJoins)
	for _, store := range k.store {
		metrics := store.(*ksmstore.MetricsStore).Push(k.familyFilter, k.metricFilter)
		labelJoiner.insertFamilies(metrics)
	}

	for _, store := range k.store {
		metrics := store.(*ksmstore.MetricsStore).Push(ksmstore.GetAllFamilies, ksmstore.GetAllMetrics)
		k.processMetrics(sender, metrics, labelJoiner)
		k.processTelemetry(metrics)
	}

	k.sendTelemetry(sender)

	return nil
}

// Cancel is called when the check is unscheduled, it stops the informers used by the metrics store
func (k *KSMCheck) Cancel() {
	log.Infof("Shutting down informers used by the check '%s'", k.ID())
	k.cancel()
	k.CommonCancel()
}

// processMetrics attaches tags and forwards metrics to the aggregator
func (k *KSMCheck) processMetrics(sender aggregator.Sender, metrics map[string][]ksmstore.DDMetricsFam, labelJoiner *labelJoiner) {
	for _, metricsList := range metrics {
		for _, metricFamily := range metricsList {
			// First check for aggregator, because the check use _labels metrics to aggregate values.
			if aggregator, found := metricAggregators[metricFamily.Name]; found {
				for _, m := range metricFamily.ListMetrics {
					aggregator.accumulate(m)
				}
				// Some metrics can be aggregated and consumed as-is or by a transformer.
				// So, let’s continue the processing.
			}
			if metadataMetricsRegex.MatchString(metricFamily.Name) {
				// metadata metrics are only used by the check for label joins
				// they shouldn't be forwarded to Datadog
				continue
			}
			if transform, found := metricTransformers[metricFamily.Name]; found {
				for _, m := range metricFamily.ListMetrics {
					hostname, tags := k.hostnameAndTags(m.Labels, labelJoiner)
					transform(sender, metricFamily.Name, m, hostname, tags)
				}
				continue
			}
			if ddname, found := metricNamesMapper[metricFamily.Name]; found {
				for _, m := range metricFamily.ListMetrics {
					hostname, tags := k.hostnameAndTags(m.Labels, labelJoiner)
					sender.Gauge(ksmMetricPrefix+ddname, m.Val, hostname, tags)
				}
				continue
			}
			if _, found := metricAggregators[metricFamily.Name]; found {
				continue
			}
			// ignore the metric if it doesn't have a transformer
			// or if it isn't mapped to a datadog metric name
			log.Tracef("KSM metric '%s' is unknown for the check, ignoring it", metricFamily.Name)
		}
	}
	for _, aggregator := range metricAggregators {
		aggregator.flush(sender, k, labelJoiner)
	}
}

// hostnameAndTags returns the tags and the hostname for a metric based on the metric labels and the check configuration
func (k *KSMCheck) hostnameAndTags(labels map[string]string, labelJoiner *labelJoiner) (string, []string) {
	hostname := ""

	labelsToAdd := labelJoiner.getLabelsToAdd(labels)
	tags := make([]string, 0, len(labels)+len(labelsToAdd))

	for key, value := range labels {
		tag, hostTag := k.buildTag(key, value)
		tags = append(tags, tag)
		if hostTag != "" {
			hostname = hostTag
		}
	}

	// apply label joins
	for _, label := range labelsToAdd {
		tag, hostTag := k.buildTag(label.key, label.value)
		tags = append(tags, tag)
		if hostTag != "" {
			hostname = hostTag
		}
	}

	return hostname, append(tags, k.instance.Tags...)
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

// buildTag applies the LabelsMapper config and returns the tag in a key:value string format
// The second return value is the hostname of the metric if a 'node' or 'host' tag is found, empty string otherwise
func (k *KSMCheck) buildTag(key, value string) (tag, hostname string) {
	if newKey, found := k.instance.LabelsMapper[key]; found {
		key = newKey
	}

	var sb strings.Builder
	sb.Grow(len(key) + 1 + len(value))
	sb.WriteString(key)
	sb.WriteByte(':')
	sb.WriteString(value)
	tag = sb.String()

	if key == "host" || key == "node" {
		hostname = value
	}
	return
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

// initTags avoids keeping a nil Tags field in the check instance
// and sets the kube_cluster_name tag for all metrics
func (k *KSMCheck) initTags() {
	if k.instance.Tags == nil {
		k.instance.Tags = []string{}
	}
	hostname, _ := util.GetHostname()
	if clusterName := clustername.GetClusterName(hostname); clusterName != "" {
		k.instance.Tags = append(k.instance.Tags, "kube_cluster_name:"+clusterName)
	}
}

// processTelemetry accumulates the telemetry metric values, it can be called multiple times
// during a check run then sendTelemetry should be called to forward the calculated values
func (k *KSMCheck) processTelemetry(metrics map[string][]ksmstore.DDMetricsFam) {
	if !k.instance.Telemetry {
		return
	}

	for name, list := range metrics {
		isMetadataMetric := metadataMetricsRegex.MatchString(name)
		if !isKnownMetric(name) && !isMetadataMetric {
			k.telemetry.incUnknown()
			continue
		}
		if isMetadataMetric {
			continue
		}
		count := 0
		for _, family := range list {
			count += len(family.ListMetrics)
		}
		k.telemetry.incTotal(count)
		if resource := resourceNameFromMetric(name); resource != "" {
			k.telemetry.incResource(resourceNameFromMetric(name), count)
		}
	}
}

// sendTelemetry converts the cached telemetry values and forwards them as telemetry metrics
func (k *KSMCheck) sendTelemetry(s aggregator.Sender) {
	if !k.instance.Telemetry {
		return
	}

	// reset the cache for the next check run
	defer k.telemetry.reset()

	s.Gauge(ksmMetricPrefix+"telemetry.metrics.count.total", float64(k.telemetry.getTotal()), "", k.instance.Tags)
	s.Gauge(ksmMetricPrefix+"telemetry.unknown_metrics.count", float64(k.telemetry.getUnknown()), "", k.instance.Tags) // useful to track metrics that aren't mapped to DD metrics
	for resource, count := range k.telemetry.getResourcesCount() {
		s.Gauge(ksmMetricPrefix+"telemetry.metrics.count", float64(count), "", append(k.instance.Tags, "resource_name:"+resource))
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

// KubeStateMetricsFactoryWithParam is used only by test/benchmarks/kubernetes_state
func KubeStateMetricsFactoryWithParam(labelsMapper map[string]string, labelJoins map[string]*JoinsConfig, store []cache.Store) *KSMCheck {
	check := newKSMCheck(
		core.NewCheckBase(kubeStateMetricsCheckName),
		&KSMConfig{
			LabelsMapper: labelsMapper,
			LabelJoins:   labelJoins,
			Namespaces:   []string{},
		})
	check.store = store
	return check
}

func newKSMCheck(base core.CheckBase, instance *KSMConfig) *KSMCheck {
	return &KSMCheck{
		CheckBase:   base,
		instance:    instance,
		telemetry:   newTelemetryCache(),
		isCLCRunner: config.IsCLCRunner(),
	}
}

// resourceNameFromMetric returns the resource name based on the metric name
// It relies on the conventional KSM naming format kube_<resource>_suffix
// returns an empty string otherwise
func resourceNameFromMetric(name string) string {
	parts := strings.SplitN(name, "_", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// isKnownMetric returns whether the KSM metric name is known by the check
// A known metric should satisfy one of the conditions:
//  - has a datadog metric name
//  - has a metric transformer
//  - has a metric aggregator
func isKnownMetric(name string) bool {
	if _, found := metricNamesMapper[name]; found {
		return true
	}
	if _, found := metricTransformers[name]; found {
		return true
	}
	if _, found := metricAggregators[name]; found {
		return true
	}
	return false
}

// buildDeniedMetricsSet adds *_created metrics to the default denied metric rules.
// It allows us to get kube_node_created and kube_pod_created and deny
// the rest of *_created metrics without relying on a unmaintainable and unreadable regex.
func buildDeniedMetricsSet(collectors []string) options.MetricSet {
	deniedMetrics := defaultDeniedMetrics
	for _, resource := range collectors {
		// resource format: pods, nodes, jobs, deployments...
		if resource == "pods" || resource == "nodes" {
			continue
		}
		deniedMetrics["kube_"+strings.TrimRight(resource, "s")+"_created"] = struct{}{}
	}

	return deniedMetrics
}
