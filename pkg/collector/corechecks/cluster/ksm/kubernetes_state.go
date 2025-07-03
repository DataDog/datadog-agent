// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package ksm implements the Kubernetes State Core cluster check.
package ksm

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/customresourcestate"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/tagger/kubetags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm/customresources"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	hostnameUtil "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	// CheckName is the name of the check
	CheckName               = "kubernetes_state_core"
	maximumWaitForAPIServer = 10 * time.Second

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
	"jobs":  "batch/v1, Resource=jobs_extended",
	"nodes": "core/v1, Resource=nodes_extended",
	"pods":  "core/v1, Resource=pods_extended",
}

// collectorNameReplacement contains a mapping of collector names as they would appear in the KSM config to what
// their new collector name would be. For backwards compatibility.
var collectorNameReplacement = map[string]string{
	"apiservices":               "apiregistration.k8s.io/v1, Resource=apiservices",
	"customresourcedefinitions": "apiextensions.k8s.io/v1, Resource=customresourcedefinitions",
	// verticalpodautoscalers were removed from the built-in KSM metrics in KSM 2.9, and the changes made to
	// the KSM builder in KSM 2.9 result in the detected custom resource store name being different.
	"verticalpodautoscalers": "autoscaling.k8s.io/v1, Resource=verticalpodautoscalers",
}

var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

type podCollectionMode string

const (
	// defaultPodCollection is the default mode where pods are collected from
	// the API server.
	defaultPodCollection podCollectionMode = "default"

	// nodeKubeletPodCollection is the mode where pods are collected from the
	// kubelet.
	//
	// This is meant to be enabled when the check is running on the node agent.
	// This is useful in clusters with a large number of pods where emitting pod
	// metrics from a single instance might be too much and cause performance
	// issues.
	//
	// One thing to note is that when the node agent collects metrics from the
	// kubelet and the cluster agent or cluster check runner collects metrics
	// for other resources, label joins are not supported for pod metrics if the
	// join source is not a pod.
	nodeKubeletPodCollection podCollectionMode = "node_kubelet"

	// clusterUnassignedPodCollection is the mode where pods are collected from
	// the API server but only unassigned pods.
	//
	// This is meant to be enabled when the check is running on the cluster
	// agent or the cluster check runner and "nodeKubeletPodCollection" is
	// enabled on the node agents, because unassigned pods cannot be collected
	// from node agents.
	clusterUnassignedPodCollection podCollectionMode = "cluster_unassigned"
)

// KSMConfig contains the check config parameters
type KSMConfig struct {
	// Collectors defines the resource type collectors.
	// Example: Enable pods and nodes collectors.
	// collectors:
	//   - nodes
	//   - pods
	Collectors []string `yaml:"collectors"`

	// CustomResourceStateMetrics defines the custom resource states metrics
	// https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/extend/customresourcestate-metrics.md
	// Example: Enable custom resource state metrics for CRD mycrd.
	// custom_resource:
	//    spec:
	//      resources:
	//      - groupVersionKind:
	//          group: "datadoghq.com"
	//          kind: "DatadogAgent"
	//          version: "v2alpha1"
	//        metrics:
	//          - name: "custom_metric"
	//            help: "custom_metric"
	//            each:
	//              type: Gauge
	//              gauge:
	//                path: [status, agent, available]
	CustomResource customresourcestate.Metrics `yaml:"custom_resource"`

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
	// It is also enriched in `initTags` with `kube_cluster_name` and global tags.
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

	// UseAPIServerCache enables the use of the API server cache for the check
	UseAPIServerCache bool `yaml:"use_apiserver_cache"`

	// PodCollectionMode defines how pods are collected.
	// Accepted values are: "default", "node_kubelet", and "cluster_unassigned".
	PodCollectionMode podCollectionMode `yaml:"pod_collection_mode"`
}

// KSMCheck wraps the config and the metric stores needed to run the check
type KSMCheck struct {
	core.CheckBase
	agentConfig          model.Config
	instance             *KSMConfig
	allStores            [][]cache.Store
	telemetry            *telemetryCache
	cancel               context.CancelFunc
	isCLCRunner          bool
	isRunningOnNodeAgent bool
	clusterIDTagValue    string
	clusterNameTagValue  string
	clusterNameRFC1123   string
	metricNamesMapper    map[string]string
	metricAggregators    map[string]metricAggregator
	metricTransformers   map[string]metricTransformerFunc
	metadataMetricsRegex *regexp.Regexp
	initRetry            retry.Retrier
}

// JoinsConfigWithoutLabelsMapping contains the config parameters for label joins
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

	if slices.Contains(jc.LabelsToGet, "*") {
		jc.GetAllLabels = true
		return
	}
}

var labelRegexp *regexp.Regexp

func init() {
	labelRegexp = regexp.MustCompile(`[\/]|[\.]|[\-]`)
}

// Configure prepares the configuration of the KSM check instance
func (k *KSMCheck) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, config, initConfig integration.Data, source string) error {
	k.BuildID(integrationConfigDigest, config, initConfig)
	k.agentConfig = pkgconfigsetup.Datadog()

	err := k.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	err = k.instance.parse(config)
	if err != nil {
		return err
	}

	maps.Copy(k.metricNamesMapper, customresources.GetCustomMetricNamesMapper(k.instance.CustomResource.Spec.Resources))

	// Retrieve cluster name
	k.getClusterName()

	// Retrieve the ClusterID from the cluster-agent
	k.getClusterID()

	// Initialize global tags and check tags
	k.initTags()

	// Prepare label joins
	for _, joinConf := range k.instance.LabelJoins {
		joinConf.setupGetAllLabels()
	}

	k.mergeLabelJoins(defaultLabelJoins())

	setupLabelsAndAnnotationsAsTagsFunc := func() {
		metadataAsTags := configUtils.GetMetadataAsTags(pkgconfigsetup.Datadog())

		k.processLabelJoins()
		k.instance.LabelsAsTags = mergeLabelsOrAnnotationAsTags(metadataAsTags.GetResourcesLabelsAsTags(), k.instance.LabelsAsTags, true)
		k.processLabelsAsTags()

		// We need to merge the user-defined annotations as tags with the default annotations first
		mergedAnnotationsAsTags := mergeLabelsOrAnnotationAsTags(k.instance.AnnotationsAsTags, defaultAnnotationsAsTags(), false)
		k.instance.AnnotationsAsTags = mergeLabelsOrAnnotationAsTags(metadataAsTags.GetResourcesAnnotationsAsTags(), mergedAnnotationsAsTags, true)
		k.processAnnotationsAsTags()
	}

	// Prepare labels mapper
	k.mergeLabelsMapper(defaultLabelsMapper())

	// Retry configuration steps related to API Server in check executions if necessary
	// TODO: extract init configuration attempt function into a struct method
	err = k.initRetry.SetupRetrier(&retry.Config{
		Name: fmt.Sprintf("%s_%s", CheckName, "configuration"),
		AttemptMethod: func() error {
			builder := kubestatemetrics.New()
			builder.WithUsingAPIServerCache(k.instance.UseAPIServerCache)

			k.configurePodCollection(builder, k.instance.Collectors)

			var collectors []string
			var apiServerClient *apiserver.APIClient
			var resources []*v1.APIResourceList

			switch k.instance.PodCollectionMode {
			case nodeKubeletPodCollection:
				// In this case we don't need to set up anything related to the API
				// server.
				collectors = []string{"pods"}
				setupLabelsAndAnnotationsAsTagsFunc()
			case defaultPodCollection, clusterUnassignedPodCollection:
				// We can try to get the API Client directly because this code will be retried if it fails
				apiServerClient, err = apiserver.GetAPIClient()
				if err != nil {
					return err
				}

				err = apiserver.InitializeGlobalResourceTypeCache(apiServerClient.Cl.Discovery())
				if err != nil {
					return err
				}

				setupLabelsAndAnnotationsAsTagsFunc()

				// Discover resources that are currently available
				resources, err = discoverResources(apiServerClient.Cl.Discovery())
				if err != nil {
					return err
				}

				// Prepare the collectors for the resources specified in the configuration file.
				collectors, err = filterUnknownCollectors(k.instance.Collectors, resources)
				if err != nil {
					return err
				}

				// Enable the KSM default collectors if the config collectors list is empty.
				if len(collectors) == 0 {
					collectors = options.DefaultResources.AsSlice()
				}

				builder.WithKubeClient(apiServerClient.InformerCl)
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

			builder.WithFamilyGeneratorFilter(allowDenyList)

			ctx, cancel := context.WithCancel(context.Background())
			k.cancel = cancel
			builder.WithContext(ctx)

			resyncPeriod := k.instance.ResyncPeriod
			if resyncPeriod == 0 {
				resyncPeriod = pkgconfigsetup.Datadog().GetInt("kubernetes_informers_resync_period")
			}

			builder.WithResync(time.Duration(resyncPeriod) * time.Second)

			builder.WithGenerateStoresFunc(builder.GenerateStores)

			// configure custom resources required for extended features and
			// compatibility across deprecated/removed versions of APIs
			cr := k.discoverCustomResources(apiServerClient, collectors, resources)
			builder.WithGenerateCustomResourceStoresFunc(builder.GenerateCustomResourceStoresFunc)
			builder.WithCustomResourceStoreFactories(cr.factories...)
			builder.WithCustomResourceClients(cr.clients)

			// Enable exposing resource annotations explicitly for kube_<resource>_annotations metadata metrics.
			// Equivalent to configuring --metric-annotations-allowlist.
			allowedAnnotations := map[string][]string{}
			for _, collector := range collectors {
				// Any annotation can be used for label joins.
				allowedAnnotations[collector] = []string{"*"}
			}

			builder.WithAllowAnnotations(allowedAnnotations)

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
		},
		Strategy:          retry.Backoff,
		InitialRetryDelay: k.Interval(),
		MaxRetryDelay:     10 * k.Interval(),
	})
	if err != nil {
		return err
	}

	return nil
}

func discoverResources(client discovery.DiscoveryInterface) ([]*v1.APIResourceList, error) {
	_, resources, err := client.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return nil, fmt.Errorf("unable to perform resource discovery: %s", err)
		}

		for group, apiGroupErr := range err.(*discovery.ErrGroupDiscoveryFailed).Groups {
			log.Warnf("unable to perform resource discovery for group %s: %s", group, apiGroupErr)
		}
	}
	return resources, nil
}

func filterUnknownCollectors(collectors []string, resources []*v1.APIResourceList) ([]string, error) {
	resourcesSet := make(map[string]struct{}, len(collectors))
	for _, resourceList := range resources {
		for _, resource := range resourceList.APIResources {
			resourcesSet[resource.Name] = struct{}{}
		}
	}

	filteredCollectors := make([]string, 0, len(collectors))
	for i := range collectors {
		if _, ok := resourcesSet[collectors[i]]; ok {
			if _, okRepl := collectorNameReplacement[collectors[i]]; okRepl {
				filteredCollectors = append(filteredCollectors, collectorNameReplacement[collectors[i]])
			} else {
				filteredCollectors = append(filteredCollectors, collectors[i])
			}
		} else {
			log.Warnf("resource %v is unknown and will not be collected", collectors[i])
		}
	}
	return filteredCollectors, nil
}

func (c *KSMConfig) parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

type customResources struct {
	collectors []string
	factories  []customresource.RegistryFactory
	clients    map[string]interface{}
}

func (k *KSMCheck) discoverCustomResources(c *apiserver.APIClient, collectors []string, resources []*v1.APIResourceList) customResources {
	// automatically add extended collectors if their standard ones are
	// enabled
	for _, c := range collectors {
		if extended, ok := extendedCollectors[c]; ok {
			collectors = append(collectors, extended)
		}
	}

	if k.instance.PodCollectionMode == nodeKubeletPodCollection {
		return customResources{
			collectors: collectors,
			factories: []customresource.RegistryFactory{
				customresources.NewExtendedPodFactoryForKubelet(),
			},
		}
	}

	// extended resource collectors always have a factory registered
	factories := []customresource.RegistryFactory{
		customresources.NewExtendedJobFactory(c),
		customresources.NewCustomResourceDefinitionFactory(c),
		customresources.NewAPIServiceFactory(c),
		customresources.NewExtendedNodeFactory(c),
		customresources.NewExtendedPodFactory(c),
		customresources.NewVerticalPodAutoscalerFactory(c),
	}

	factories = manageResourcesReplacement(c, factories, resources)

	clients := make(map[string]interface{}, len(factories))
	for _, f := range factories {
		client, _ := f.CreateClient(nil)
		clients[f.Name()] = client
	}

	customResourceFactories := customresources.GetCustomResourceFactories(k.instance.CustomResource, c)
	customResourceClients, customResourceCollectors := customresources.GetCustomResourceClientsAndCollectors(k.instance.CustomResource.Spec.Resources, c)

	collectors = lo.Uniq(append(collectors, customResourceCollectors...))
	maps.Copy(clients, customResourceClients)
	factories = append(factories, customResourceFactories...)

	return customResources{
		collectors: collectors,
		clients:    clients,
		factories:  factories,
	}
}

func manageResourcesReplacement(c *apiserver.APIClient, factories []customresource.RegistryFactory, resources []*v1.APIResourceList) []customresource.RegistryFactory {
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
		"autoscaling/v2": {
			"HorizontalPodAutoscaler": customresources.NewHorizontalPodAutoscalerV2Beta2Factory,
		},
	}

	for gv, resourceReplacement := range resourceReplacements {
		for _, resource := range resources {
			if resource.GroupVersion != gv {
				continue
			}

			for _, apiResource := range resource.APIResources {
				delete(resourceReplacement, apiResource.Kind)
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
	if err := k.initRetry.TriggerRetry(); err != nil {
		return err.LastTryError
	}

	// this check uses a "raw" sender, for better performance.  That requires
	// careful consideration of uses of this sender.  In particular, the `tags
	// []string` arguments must not be used after they are passed to the sender
	// methods, as they may be mutated in-place.
	sender, err := k.GetRawSender()
	if err != nil {
		return err
	}

	// Normally the sender is kept for the lifetime of the check.
	// But as `SetCheckCustomTags` is cheap and `k.instance.Tags` is immutable
	// It's fast and safe to set it after we get the sender.
	sender.SetCheckCustomTags(k.instance.Tags)

	// Do not fallback to the Agent hostname if the hostname corresponding to the KSM metric is unknown
	// Note that by design, some metrics cannot have hostnames (e.g kubernetes_state.pod.unschedulable)
	sender.DisableDefaultHostname(true)

	// If KSM is running in the node agent, and it's configured to collect only
	// pods and from the node agent, we don't need to run leader election,
	// because each node agent is responsible for collecting its own pods.
	podsFromKubeletInNodeAgent := k.isRunningOnNodeAgent && k.instance.PodCollectionMode == nodeKubeletPodCollection

	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// we also do a safety check for dedicated runners to avoid trying the leader election
	if (!k.isCLCRunner || !k.instance.LeaderSkip) && !podsFromKubeletInNodeAgent {
		// Only run if Leader Election is enabled.
		if !pkgconfigsetup.Datadog().GetBool("leader_election") {
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
	if k.cancel != nil {
		k.cancel()
	}
}

// processMetrics attaches tags and forwards metrics to the aggregator
func (k *KSMCheck) processMetrics(sender sender.Sender, metrics map[string][]ksmstore.DDMetricsFam, labelJoiner *labelJoiner, now time.Time) {
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
					hostname, tagList := k.hostnameAndTags(m.Labels, labelJoiner, lMapperOverride)
					transform(sender, metricFamily.Name, m, hostname, tagList, now)
				}
				continue
			}
			metricPrefix := ksmMetricPrefix
			if ddname, found := k.metricNamesMapper[metricFamily.Name]; found && strings.HasPrefix(ddname, "customresource.") {
				metricPrefix = metricPrefix[:len(metricPrefix)-1] + "_"
			}
			if ddname, found := k.metricNamesMapper[metricFamily.Name]; found {
				lMapperOverride := labelsMapperOverride(metricFamily.Name)
				for _, m := range metricFamily.ListMetrics {
					hostname, tagList := k.hostnameAndTags(m.Labels, labelJoiner, lMapperOverride)
					sender.Gauge(metricPrefix+ddname, m.Val, hostname, tagList)
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
	tagList := make([]string, 0, len(labels)+len(labelsToAdd))

	ownerKind, ownerName := "", ""
	for key, value := range labels {
		switch key {
		case createdByKindKey, ownerKindKey:
			ownerKind = value
		case createdByNameKey, ownerNameKey:
			ownerName = value
		default:
			tag, hostTag := k.buildTag(key, value, lMapperOverride)
			tagList = append(tagList, tag)
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
			tagList = append(tagList, tag)
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
		tagList = append(tagList, owners...)
	}

	return hostname, tagList
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

func (k *KSMCheck) getClusterID() {
	clusterID, err := clustername.GetClusterID()
	if err != nil {
		log.Warnf("Error retrieving the cluster ID: %s", err)
		return
	}
	k.clusterIDTagValue = clusterID
}

// initTags avoids keeping a nil Tags field in the check instance
// Sets the kube_cluster_name tag for all metrics.
// Sets the orch_cluster_id tag for all metrics.
// Adds the global user-defined tags from the Agent config.
func (k *KSMCheck) initTags() {
	if k.clusterNameTagValue != "" {
		k.instance.Tags = append(k.instance.Tags, tags.KubeClusterName+":"+k.clusterNameTagValue)
	}

	if k.clusterIDTagValue != "" {
		k.instance.Tags = append(k.instance.Tags, tags.OrchClusterID+":"+k.clusterIDTagValue)
	}

	if !k.instance.DisableGlobalTags {
		k.instance.Tags = append(k.instance.Tags, configUtils.GetConfiguredTags(k.agentConfig, false)...)
	}
}

func (k *KSMCheck) configurePodCollection(builder *kubestatemetrics.Builder, collectors []string) {
	switch k.instance.PodCollectionMode {
	case "":
		k.instance.PodCollectionMode = defaultPodCollection
	case defaultPodCollection:
		// No need to do anything
	case nodeKubeletPodCollection:
		if k.isRunningOnNodeAgent {
			// If the check is running in a node agent, we can collect pods from
			// the kubelet but only if it's the only collector enabled. When
			// there are more collectors enabled, we need leader election and
			// pods would only be collected from one of the agents.
			if len(collectors) == 1 && collectors[0] == "pods" {
				builder.WithPodCollectionFromKubelet()
			} else {
				log.Warnf("pod collection from the Kubelet is enabled but it's only supported when the only collector enabled is pods, " +
					"so the check will collect pods from the API server instead of the Kubelet")
				k.instance.PodCollectionMode = defaultPodCollection
			}
		} else {
			log.Warnf("pod collection from the Kubelet is enabled but KSM is running in the cluster agent or cluster check runner, " +
				"so the check will collect pods from the API server instead of the Kubelet")
			k.instance.PodCollectionMode = defaultPodCollection
		}
	case clusterUnassignedPodCollection:
		if k.isRunningOnNodeAgent {
			log.Warnf("collection of unassigned pods is enabled but KSM is running in a node agent, so the option will be ignored")
			k.instance.PodCollectionMode = defaultPodCollection
		} else {
			builder.WithUnassignedPodsCollection()
		}
	default:
		log.Warnf("invalid pod collection mode %q, falling back to the default mode", k.instance.PodCollectionMode)
		k.instance.PodCollectionMode = defaultPodCollection
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
func (k *KSMCheck) sendTelemetry(s sender.Sender) {
	if !k.instance.Telemetry {
		return
	}

	// reset the cache for the next check run
	defer k.telemetry.reset()

	s.Gauge(ksmMetricPrefix+"telemetry.metrics.count.total", float64(k.telemetry.getTotal()), "", nil)
	s.Gauge(ksmMetricPrefix+"telemetry.unknown_metrics.count", float64(k.telemetry.getUnknown()), "", nil) // useful to track metrics that aren't mapped to DD metrics
	for resource, count := range k.telemetry.getResourcesCount() {
		s.Gauge(ksmMetricPrefix+"telemetry.metrics.count", float64(count), "", []string{"resource_name:" + resource})
	}
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return newKSMCheck(
		core.NewCheckBase(CheckName),
		&KSMConfig{
			LabelsMapper: make(map[string]string),
			LabelJoins:   make(map[string]*JoinsConfigWithoutLabelsMapping),
			Namespaces:   []string{},
		})
}

// KubeStateMetricsFactoryWithParam is used only by test/benchmarks/kubernetes_state
func KubeStateMetricsFactoryWithParam(labelsMapper map[string]string, labelJoins map[string]*JoinsConfigWithoutLabelsMapping, allStores [][]cache.Store) *KSMCheck {
	check := newKSMCheck(
		core.NewCheckBase(CheckName),
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
		CheckBase:            base,
		instance:             instance,
		telemetry:            newTelemetryCache(),
		isCLCRunner:          pkgconfigsetup.IsCLCRunner(pkgconfigsetup.Datadog()),
		isRunningOnNodeAgent: flavor.GetFlavor() != flavor.ClusterAgent && !pkgconfigsetup.IsCLCRunner(pkgconfigsetup.Datadog()),
		metricNamesMapper:    defaultMetricNamesMapper(),
		metricAggregators:    defaultMetricAggregators(),
		metricTransformers:   defaultMetricTransformers(),

		// metadata metrics are useful for label joins
		// but shouldn't be submitted to Datadog
		metadataMetricsRegex: regexp.MustCompile(".*_(info|labels|status_reason)"),
	}
}

// mergeLabelsOrAnnotationAsTags adds extra labels or annotations to the instance mapping
func mergeLabelsOrAnnotationAsTags(extra, instanceMap map[string]map[string]string, shouldTransformResource bool) map[string]map[string]string {
	if instanceMap == nil {
		instanceMap = make(map[string]map[string]string)
	}
	// In the case of a misconfiguration issue, the value could be explicitly set to nil
	for resource, mapping := range instanceMap {
		if mapping == nil {
			delete(instanceMap, resource)
		}
	}

	for resource, mapping := range extra {
		var singularName = resource
		var err error
		if shouldTransformResource {
			// modify the resource name to the singular form of the resource
			singularName, err = toSingularResourceName(resource)
			if err != nil {
				log.Errorf("failed to get singular resource name for %q: %v", resource, err)
				continue
			}
		}

		_, found := instanceMap[singularName]
		if !found {
			instanceMap[singularName] = make(map[string]string)
			instanceMap[singularName] = mapping
			continue
		}
		for key, value := range mapping {
			if _, found := instanceMap[singularName][key]; !found {
				instanceMap[singularName][key] = value
			}
		}
	}

	return instanceMap
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

	tagKey, err := kubetags.GetTagForKubernetesKind(kind)
	if err != nil {
		return nil
	}

	tagFormat := "%s:%s"
	tagList := []string{fmt.Sprintf(tagFormat, tagKey, name)}
	switch kind {
	case kubernetes.JobKind:
		if cronjob, _ := kubernetes.ParseCronJobForJob(name); cronjob != "" {
			return append(tagList, fmt.Sprintf(tagFormat, tags.KubeCronjob, cronjob))
		}
	case kubernetes.ReplicaSetKind:
		if deployment := kubernetes.ParseDeploymentForReplicaSet(name); deployment != "" {
			return append(tagList, fmt.Sprintf(tagFormat, tags.KubeDeployment, deployment))
		}
	}

	return tagList
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

func toSingularResourceName(resourceGroup string) (string, error) {
	// Expected input in the form of: resourceTypePlural.apiGroup
	resourceType, group, _ := strings.Cut(resourceGroup, ".")
	kind, err := apiserver.GetResourceKind(resourceType, group)
	return strings.ToLower(kind), err
}
