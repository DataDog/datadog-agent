// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm/customresources"
	"github.com/DataDog/datadog-agent/pkg/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/exp/maps"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/options"
)

const (
	kubeStateMetricsCheckName = "kubernetes_state_core"
	maximumWaitForAPIServer   = 10 * time.Second

	// createdByKindKey represents the KSM label key created_by_kind
	createdByKindKey = "created_by_kind"
	// createdByNameKey represents the KSM label key created_by_name
	createdByNameKey = "created_by_name"
	// ownerKindKey represents the KSM label key owner_kind
	ownerKindKey = "owner_kind"
	// ownerNameKey represents the KSM label key owner_name
	ownerNameKey = "owner_name"
)

var extendedCollectors = map[string]string{
	"jobs":  "jobs_extended",
	"nodes": "nodes_extended",
	"pods":  "pods_extended",
}

var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

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
	//     labels_to_match:
	//       - deployment
	//     labels_to_get:
	//       - label_addonmanager_kubernetes_io_mode
	LabelJoins map[string]*JoinsConfigWithoutLabelsMapping `yaml:"label_joins"`

	// LabelsAsTags
	// Example:
	// labels_as_tags:
	//   pod:
	//     app: pod_app
	//   node:
	//     app: node_app
	//     team: node_team
	LabelsAsTags map[string]map[string]string `yaml:"labels_as_tags"`

	// AnnotationsAsTags
	// Example:
	// annotations_as_tags:
	//   pod:
	//     app: pod_app
	//   node:
	//     app: node_app
	//     team: node_team
	AnnotationsAsTags map[string]map[string]string `yaml:"annotations_as_tags"`

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

	// DisableGlobalTags disables adding the global host tags defined via tags/DD_TAG in the Agent config, default false.
	DisableGlobalTags bool `yaml:"disable_global_tags"`

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

	// Private field containing the label joins configuration built from `LabelJoins`, `LabelsAsTags` and `AnnotationsAsTags`.
	labelJoins map[string]*joinsConfig
}

// KSMCheck wraps the config and the metric stores needed to run the check
type KSMCheck struct {
	core.CheckBase
	agentConfig          config.Config
	instance             *KSMConfig
	allStores            [][]cache.Store
	telemetry            *telemetryCache
	cancel               context.CancelFunc
	isCLCRunner          bool
	clusterNameTagValue  string
	clusterNameRFC1123   string
	metricNamesMapper    map[string]string
	metricAggregators    map[string]metricAggregator
	metricTransformers   map[string]metricTransformerFunc
	metadataMetricsRegex *regexp.Regexp
}

// JoinsConfig contains the config parameters for label joins
type JoinsConfigWithoutLabelsMapping struct {
	// LabelsToMatch contains the labels that must
	// match the labels of the targeted metric
	LabelsToMatch []string `yaml:"labels_to_match"`

	// LabelsToGet contains the labels we want to get from the targeted metric
	LabelsToGet []string `yaml:"labels_to_get"`

	// GetAllLabels replaces LabelsToGet if enabled
	GetAllLabels bool `yaml:"get_all_labels"`
}

func (jc *JoinsConfigWithoutLabelsMapping) setupGetAllLabels() {
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

var labelRegexp *regexp.Regexp

func init() {
	labelRegexp = regexp.MustCompile(`[\/]|[\.]|[\-]`)
}

func init() {
	core.RegisterCheck(kubeStateMetricsCheckName, KubeStateMetricsFactory)
}

// Configure prepares the configuration of the KSM check instance
func (k *KSMCheck) Configure(integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	k.BuildID(integrationConfigDigest, config, initConfig)
	k.agentConfig = ddconfig.Datadog

	err := k.CommonConfigure(integrationConfigDigest, initConfig, config, source)
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

	k.mergeLabelJoins(defaultLabelJoins())

	k.processLabelJoins()
	k.processLabelsAsTags()

	k.mergeAnnotationsAsTags(defaultAnnotationsAsTags())
	k.processAnnotationsAsTags()

	// Prepare labels mapper
	k.mergeLabelsMapper(defaultLabelsMapper())

	// Retrieve cluster name
	k.getClusterName()

	k.initTags()

	builder := kubestatemetrics.New()

	// Prepare the collectors for the resources specified in the configuration file.
	collectors := k.instance.Collectors

	// Enable the KSM default collectors if the config collectors list is empty.
	if len(collectors) == 0 {
		collectors = options.DefaultResources.AsSlice()
	}

	// Enable exposing resource annotations explicitly for kube_<resource>_annotations metadata metrics.
	// Equivalent to configuring --metric-annotations-allowlist.
	allowedAnnotations := map[string][]string{}
	for _, collector := range collectors {
		// Any annotation can be used for label joins.
		allowedAnnotations[collector] = []string{"*"}
	}

	builder.WithAllowAnnotations(allowedAnnotations)

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

	builder.WithFamilyGeneratorFilter(allowDenyList)

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

	builder.WithVPAClient(c.VPAClient)

	ctx, cancel := context.WithCancel(context.Background())
	k.cancel = cancel
	builder.WithContext(ctx)

	resyncPeriod := k.instance.ResyncPeriod
	if resyncPeriod == 0 {
		resyncPeriod = ddconfig.Datadog.GetInt("kubernetes_informers_resync_period")
	}

	builder.WithResync(time.Duration(resyncPeriod) * time.Second)

	builder.WithGenerateStoresFunc(builder.GenerateStores)

	// configure custom resources required for extended features and
	// compatibility across deprecated/removed versions of APIs
	cr := k.discoverCustomResources(c, collectors)
	builder.WithGenerateCustomResourceStoresFunc(builder.GenerateCustomResourceStoresFunc)
	builder.WithCustomResourceStoreFactories(cr.factories...)
	builder.WithCustomResourceClients(cr.clients)

	// Enable exposing resource labels explicitly for kube_<resource>_labels metadata metrics.
	// Equivalent to configuring --metric-labels-allowlist.
	allowedLabels := map[string][]string{}
	for _, collector := range collectors {
		// Any label can be used for label joins.
		allowedLabels[collector] = []string{"*"}
	}

	if err = builder.WithAllowLabels(allowedLabels); err != nil {
		return err
	}

	if err := builder.WithEnabledResources(cr.collectors); err != nil {
		return err
	}

	// Start the collection process
	k.allStores = builder.BuildStores()

	return nil
}

func (c *KSMConfig) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

type customResources struct {
	collectors []string
	factories  []customresource.RegistryFactory
	clients    map[string]interface{}
}

func (k *KSMCheck) discoverCustomResources(c *apiserver.APIClient, collectors []string) customResources {
	// automatically add extended collectors if their standard ones are
	// enabled
	for _, c := range collectors {
		if extended, ok := extendedCollectors[c]; ok {
			collectors = append(collectors, extended)
		}
	}

	// extended resource collectors always have a factory registered
	factories := []customresource.RegistryFactory{
		customresources.NewExtendedJobFactory(c),
		customresources.NewCustomResourceDefinitionFactory(c),
		customresources.NewAPIServiceFactory(c),
		customresources.NewExtendedNodeFactory(c),
		customresources.NewExtendedPodFactory(c),
	}

	factories = manageResourcesReplacement(c, factories)

	clients := make(map[string]interface{}, len(factories))
	for _, f := range factories {
		client, _ := f.CreateClient(nil)
		clients[f.Name()] = client
	}

	return customResources{
		collectors: collectors,
		clients:    clients,
		factories:  factories,
	}
}

func manageResourcesReplacement(c *apiserver.APIClient, factories []customresource.RegistryFactory) []customresource.RegistryFactory {
	if c.DiscoveryCl == nil {
		log.Warn("Kubernetes discovery client has not been properly initialized")
		return factories
	}

	_, resources, err := c.DiscoveryCl.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			log.Warnf("unable to perform resource discovery: %s", err)
		} else {
			for group, apiGroupErr := range err.(*discovery.ErrGroupDiscoveryFailed).Groups {
				log.Warnf("unable to perform resource discovery for group %s: %s", group, apiGroupErr)
			}
		}
	}

	// backwards/forwards compatibility resource factories are only
	// registered if they're needed, otherwise they'd overwrite the default
	// ones that ship with ksm
	resourceReplacements := map[string]map[string]func(c *apiserver.APIClient) customresource.RegistryFactory{
		// support for older k8s versions where the resources are no
		// longer supported in KSM
		"batch/v1": {
			"CronJob": customresources.NewCronJobV1Beta1Factory,
		},
		"policy/v1": {
			"PodDisruptionBudget": customresources.NewPodDisruptionBudgetV1Beta1Factory,
		},

		// support for newer k8s versions where the newer resources are
		// not yet supported by KSM
		"autoscaling/v2beta2": {
			"HorizontalPodAutoscaler": customresources.NewHorizontalPodAutoscalerV2Factory,
		},
	}

	for gv, resourceReplacement := range resourceReplacements {
		for _, resource := range resources {
			if resource.GroupVersion != gv {
				continue
			}

			for _, apiResource := range resource.APIResources {
				if _, ok := resourceReplacement[apiResource.Kind]; ok {
					delete(resourceReplacement, apiResource.Kind)
				}
			}
		}
	}

	for _, resourceReplacement := range resourceReplacements {
		for _, factory := range resourceReplacement {
			factories = append(factories, factory(c))
		}
	}

	return factories
}

// Run runs the KSM check
func (k *KSMCheck) Run() error {
	// this check uses a "raw" sender, for better performance.  That requires
	// careful consideration of uses of this sender.  In particular, the `tags
	// []string` arguments must not be used after they are passed to the sender
	// methods, as they may be mutated in-place.
	sender, err := k.GetRawSender()
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

	labelJoiner := newLabelJoiner(k.instance.labelJoins)
	for _, stores := range k.allStores {
		for _, store := range stores {
			metrics := store.(*ksmstore.MetricsStore).Push(k.familyFilter, k.metricFilter)
			labelJoiner.insertFamilies(metrics)
		}
	}

	currentTime := time.Now()
	for _, stores := range k.allStores {
		for _, store := range stores {
			metrics := store.(*ksmstore.MetricsStore).Push(ksmstore.GetAllFamilies, ksmstore.GetAllMetrics)
			k.processMetrics(sender, metrics, labelJoiner, currentTime)
			k.processTelemetry(metrics)
		}
	}

	k.sendTelemetry(sender)

	return nil
}

// Cancel is called when the check is unscheduled, it stops the informers used by the metrics store
func (k *KSMCheck) Cancel() {
	log.Infof("Shutting down informers used by the check '%s'", k.ID())
	k.cancel()
}

// processMetrics attaches tags and forwards metrics to the aggregator
func (k *KSMCheck) processMetrics(sender aggregator.Sender, metrics map[string][]ksmstore.DDMetricsFam, labelJoiner *labelJoiner, now time.Time) {
	for _, metricsList := range metrics {
		for _, metricFamily := range metricsList {
			// First check for aggregator, because the check use _labels metrics to aggregate values.
			if aggregator, found := k.metricAggregators[metricFamily.Name]; found {
				for _, m := range metricFamily.ListMetrics {
					aggregator.accumulate(m)
				}
				// Some metrics can be aggregated and consumed as-is or by a transformer.
				// So, let’s continue the processing.
			}
			if transform, found := k.metricTransformers[metricFamily.Name]; found {
				lMapperOverride := labelsMapperOverride(metricFamily.Name)
				for _, m := range metricFamily.ListMetrics {
					hostname, tags := k.hostnameAndTags(m.Labels, labelJoiner, lMapperOverride)
					transform(sender, metricFamily.Name, m, hostname, tags, now)
				}
				continue
			}
			if ddname, found := k.metricNamesMapper[metricFamily.Name]; found {
				lMapperOverride := labelsMapperOverride(metricFamily.Name)
				for _, m := range metricFamily.ListMetrics {
					hostname, tags := k.hostnameAndTags(m.Labels, labelJoiner, lMapperOverride)
					sender.Gauge(ksmMetricPrefix+ddname, m.Val, hostname, tags)
				}
				continue
			}
			if _, found := k.metricAggregators[metricFamily.Name]; found {
				continue
			}
			if k.metadataMetricsRegex.MatchString(metricFamily.Name) {
				// metadata metrics are only used by the check for label joins
				// they shouldn't be forwarded to Datadog
				continue
			}
			// ignore the metric if it doesn't have a transformer
			// or if it isn't mapped to a datadog metric name
			log.Tracef("KSM metric '%s' is unknown for the check, ignoring it", metricFamily.Name)
		}
	}
	for _, aggregator := range k.metricAggregators {
		aggregator.flush(sender, k, labelJoiner)
	}
}

// hostnameAndTags returns the tags and the hostname for a metric based on the metric labels and the check configuration.
//
// This function must always return a "fresh" slice of tags, that will not be accessed after return.
func (k *KSMCheck) hostnameAndTags(labels map[string]string, labelJoiner *labelJoiner, lMapperOverride map[string]string) (string, []string) {
	hostname := ""

	labelsToAdd := labelJoiner.getLabelsToAdd(labels)

	// generate a dedicated tags slice
	tags := make([]string, 0, len(labels)+len(labelsToAdd))

	ownerKind, ownerName := "", ""
	for key, value := range labels {
		switch key {
		case createdByKindKey, ownerKindKey:
			ownerKind = value
		case createdByNameKey, ownerNameKey:
			ownerName = value
		default:
			tag, hostTag := k.buildTag(key, value, lMapperOverride)
			tags = append(tags, tag)
			if hostTag != "" {
				if k.clusterNameRFC1123 != "" {
					hostname = hostTag + "-" + k.clusterNameRFC1123
				} else {
					hostname = hostTag
				}
			}
		}
	}

	// apply label joins
	for _, label := range labelsToAdd {
		switch label.key {
		case createdByKindKey, ownerKindKey:
			ownerKind = label.value
		case createdByNameKey, ownerNameKey:
			ownerName = label.value
		default:
			tag, hostTag := k.buildTag(label.key, label.value, lMapperOverride)
			tags = append(tags, tag)
			if hostTag != "" {
				if k.clusterNameRFC1123 != "" {
					hostname = hostTag + "-" + k.clusterNameRFC1123
				} else {
					hostname = hostTag
				}
			}
		}
	}

	if owners := ownerTags(ownerKind, ownerName); len(owners) != 0 {
		tags = append(tags, owners...)
	}

	return hostname, append(tags, k.instance.Tags...)
}

// familyFilter is a metric families filter for label joins
// It ensures that we only get the configured metric names to
// get labels based on the label joins config
func (k *KSMCheck) familyFilter(f ksmstore.DDMetricsFam) bool {
	_, found := k.instance.labelJoins[f.Name]
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
func (k *KSMCheck) buildTag(key, value string, lMapperOverride map[string]string) (tag, hostname string) {
	if newKey, found := k.instance.LabelsMapper[key]; found {
		key = newKey
	}

	if lMapperOverride != nil {
		if keyOverride, found := lMapperOverride[key]; found {
			key = keyOverride
		}
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
func (k *KSMCheck) mergeLabelJoins(extra map[string]*JoinsConfigWithoutLabelsMapping) {
	for key, value := range extra {
		if _, found := k.instance.LabelJoins[key]; !found {
			k.instance.LabelJoins[key] = value
		}
	}
}

// mergeAnnotationsAsTags adds extra annotations as tags to the configured mapping.
// User-defined annotations as tags are prioritized.
func (k *KSMCheck) mergeAnnotationsAsTags(extra map[string]map[string]string) {
	if k.instance.AnnotationsAsTags == nil {
		k.instance.AnnotationsAsTags = make(map[string]map[string]string)
	}
	for resource, mapping := range extra {
		_, found := k.instance.AnnotationsAsTags[resource]
		if !found {
			k.instance.AnnotationsAsTags[resource] = make(map[string]string)
			k.instance.AnnotationsAsTags[resource] = mapping
			continue
		}
		for key, value := range mapping {
			if _, found := k.instance.AnnotationsAsTags[resource][key]; !found {
				k.instance.AnnotationsAsTags[resource][key] = value
			}
		}
	}
}

func (k *KSMCheck) processLabelJoins() {
	k.instance.labelJoins = make(map[string]*joinsConfig)
	for metric, joinConf := range k.instance.LabelJoins {
		labelsToGet := make(map[string]string)
		for _, label := range joinConf.LabelsToGet {
			labelsToGet[label] = label
		}
		k.instance.labelJoins[metric] = &joinsConfig{
			labelsToMatch: joinConf.LabelsToMatch,
			labelsToGet:   labelsToGet,
			getAllLabels:  joinConf.GetAllLabels,
		}
	}
}

func (k *KSMCheck) processLabelsAsTags() {
	k.processLabelsOrAnnotationsAsTags("label", k.instance.LabelsAsTags)
}

func (k *KSMCheck) processAnnotationsAsTags() {
	k.processLabelsOrAnnotationsAsTags("annotation", k.instance.AnnotationsAsTags)
}

func (k *KSMCheck) processLabelsOrAnnotationsAsTags(what string, configStuffAsTags map[string]map[string]string) {
	for resourceKind, labelsMapper := range configStuffAsTags {
		labels := make(map[string]string)
		for label, tag := range labelsMapper {
			// KSM converts labels to snake case.
			// Ref: https://github.com/kubernetes/kube-state-metrics/blob/v2.2.2/internal/store/utils.go#L133
			label = what + "_" + toSnakeCase(labelRegexp.ReplaceAllString(label, "_"))
			labels[label] = tag
		}

		if joinsCfg, ok := k.instance.labelJoins["kube_"+resourceKind+"_"+what+"s"]; ok {
			maps.Copy(joinsCfg.labelsToGet, labels)
		} else {
			joinsConfig := &joinsConfig{
				labelsToMatch: getLabelToMatchForKind(resourceKind),
				labelsToGet:   labels,
			}
			k.instance.labelJoins["kube_"+resourceKind+"_"+what+"s"] = joinsConfig
		}
	}
}

// getClusterName retrieves the name of the cluster, if found
func (k *KSMCheck) getClusterName() {
	hostname, _ := hostnameUtil.Get(context.TODO())
	if clusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), hostname); clusterName != "" {
		k.clusterNameRFC1123 = clusterName
	}

	if clusterName := clustername.GetClusterNameTagValue(context.TODO(), hostname); clusterName != "" {
		k.clusterNameTagValue = clusterName
	}
}

// initTags avoids keeping a nil Tags field in the check instance
// Sets the kube_cluster_name tag for all metrics.
// Adds the global user-defined tags from the Agent config.
func (k *KSMCheck) initTags() {
	if k.instance.Tags == nil {
		k.instance.Tags = []string{}
	}

	if k.clusterNameTagValue != "" {
		k.instance.Tags = append(k.instance.Tags, "kube_cluster_name:"+k.clusterNameTagValue)
	}

	if !k.instance.DisableGlobalTags {
		k.instance.Tags = append(k.instance.Tags, config.GetConfiguredTags(k.agentConfig, false)...)
	}
}

// processTelemetry accumulates the telemetry metric values, it can be called multiple times
// during a check run then sendTelemetry should be called to forward the calculated values
func (k *KSMCheck) processTelemetry(metrics map[string][]ksmstore.DDMetricsFam) {
	if !k.instance.Telemetry {
		return
	}

	for name, list := range metrics {
		isMetadataMetric := k.metadataMetricsRegex.MatchString(name)
		if !k.isKnownMetric(name) && !isMetadataMetric {
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

// KubeStateMetricsFactory returns a new KSMCheck
func KubeStateMetricsFactory() check.Check {
	return newKSMCheck(
		core.NewCheckBase(kubeStateMetricsCheckName),
		&KSMConfig{
			LabelsMapper: make(map[string]string),
			LabelJoins:   make(map[string]*JoinsConfigWithoutLabelsMapping),
			Namespaces:   []string{},
		})
}

// KubeStateMetricsFactoryWithParam is used only by test/benchmarks/kubernetes_state
func KubeStateMetricsFactoryWithParam(labelsMapper map[string]string, labelJoins map[string]*JoinsConfigWithoutLabelsMapping, allStores [][]cache.Store) *KSMCheck {
	check := newKSMCheck(
		core.NewCheckBase(kubeStateMetricsCheckName),
		&KSMConfig{
			LabelsMapper: labelsMapper,
			LabelJoins:   labelJoins,
			Namespaces:   []string{},
		})
	check.allStores = allStores
	return check
}

func newKSMCheck(base core.CheckBase, instance *KSMConfig) *KSMCheck {
	return &KSMCheck{
		CheckBase:          base,
		instance:           instance,
		telemetry:          newTelemetryCache(),
		isCLCRunner:        config.IsCLCRunner(),
		metricNamesMapper:  defaultMetricNamesMapper(),
		metricAggregators:  defaultMetricAggregators(),
		metricTransformers: defaultMetricTransformers(),

		// metadata metrics are useful for label joins
		// but shouldn't be submitted to Datadog
		metadataMetricsRegex: regexp.MustCompile(".*_(info|labels|status_reason)"),
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
//   - has a datadog metric name
//   - has a metric transformer
//   - has a metric aggregator
func (k *KSMCheck) isKnownMetric(name string) bool {
	if _, found := k.metricNamesMapper[name]; found {
		return true
	}
	if _, found := k.metricTransformers[name]; found {
		return true
	}
	if _, found := k.metricAggregators[name]; found {
		return true
	}
	return false
}

// buildDeniedMetricsSet adds *_created metrics to the default denied metric rules.
// It allows us to get kube_node_created and kube_pod_created and deny
// the rest of *_created metrics without relying on a unmaintainable and unreadable regex.
func buildDeniedMetricsSet(collectors []string) options.MetricSet {
	deniedMetrics := options.MetricSet{
		".*_generation":                                    {},
		".*_metadata_resource_version":                     {},
		"kube_pod_owner":                                   {},
		"kube_pod_restart_policy":                          {},
		"kube_pod_completion_time":                         {},
		"kube_pod_status_scheduled_time":                   {},
		"kube_cronjob_status_active":                       {},
		"kube_node_status_phase":                           {},
		"kube_cronjob_spec_starting_deadline_seconds":      {},
		"kube_job_spec_active_dealine_seconds":             {},
		"kube_job_spec_completions":                        {},
		"kube_job_spec_parallelism":                        {},
		"kube_job_status_active":                           {},
		"kube_job_status_.*_time":                          {},
		"kube_service_spec_external_ip":                    {},
		"kube_service_status_load_balancer_ingress":        {},
		"kube_statefulset_status_current_revision":         {},
		"kube_statefulset_status_update_revision":          {},
		"kube_pod_container_status_last_terminated_reason": {},
		"kube_lease_renew_time":                            {},
	}
	for _, resource := range collectors {
		// resource format: pods, nodes, jobs, deployments...
		if resource == "pods" || resource == "nodes" {
			continue
		}
		deniedMetrics["kube_"+strings.TrimRight(resource, "s")+"_created"] = struct{}{}
	}

	return deniedMetrics
}

// ownerTags returns kube_<kind> tags based on given kind and name.
// If the owner is a replicaset, it tries to get the kube_deployment tag in addition to kube_replica_set.
// If the owner is a job, it tries to get the kube_cronjob tag in addition to kube_job.
func ownerTags(kind, name string) []string {
	if kind == "" || name == "" {
		return nil
	}

	tagKey, found := kubernetes.KindToTagName[kind]
	if !found {
		log.Debugf("Unknown owner kind %q", kind)
		return nil
	}

	tagFormat := "%s:%s"
	tags := []string{fmt.Sprintf(tagFormat, tagKey, name)}
	switch kind {
	case kubernetes.JobKind:
		if cronjob, _ := kubernetes.ParseCronJobForJob(name); cronjob != "" {
			return append(tags, fmt.Sprintf(tagFormat, kubernetes.CronJobTagName, cronjob))
		}
	case kubernetes.ReplicaSetKind:
		if deployment := kubernetes.ParseDeploymentForReplicaSet(name); deployment != "" {
			return append(tags, fmt.Sprintf(tagFormat, kubernetes.DeploymentTagName, deployment))
		}
	}

	return tags
}

// labelsMapperOverride allows overriding the default label mapping for
// a given metric depending on the metric family.
// Current use-cases:
//   - `phase` tag should be mapped to `pod_phase` on pod metrics only.
//   - Ingress metrics have generic tag names (host/path/service_name/service_port).
//     It's important to have them in a dedicated mapper override for ingresses.
func labelsMapperOverride(metricName string) map[string]string {
	if strings.HasPrefix(metricName, "kube_pod") {
		return map[string]string{"phase": "pod_phase"}
	}

	if strings.HasPrefix(metricName, "kube_ingress") {
		return map[string]string{
			"host":         "kube_ingress_host",
			"path":         "kube_ingress_path",
			"service_name": "kube_service",
			"service_port": "kube_service_port",
		}
	}

	if strings.HasPrefix(metricName, "kube_service") {
		return map[string]string{
			"service": "kube_service",
		}
	}
	return nil
}

func toSnakeCase(s string) string {
	snake := matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}
