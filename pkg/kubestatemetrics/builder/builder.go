// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package builder

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	ksmbuild "k8s.io/kube-state-metrics/v2/pkg/builder"
	ksmtypes "k8s.io/kube-state-metrics/v2/pkg/builder/types"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	metricsstore "k8s.io/kube-state-metrics/v2/pkg/metrics_store"
	"k8s.io/kube-state-metrics/v2/pkg/options"
	"k8s.io/kube-state-metrics/v2/pkg/watch"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Builder struct represents the metric store generator
// Follows the builder pattern: https://en.wikipedia.org/wiki/Builder_pattern
type Builder struct {
	ksmBuilder ksmtypes.BuilderInterface

	customResourceClients map[string]interface{}
	kubeClient            clientset.Interface
	namespaces            options.NamespaceList
	fieldSelectorFilter   string
	ctx                   context.Context
	allowDenyList         generator.FamilyGeneratorFilter
	metrics               *watch.ListWatchMetrics

	resync time.Duration

	collectOnlyUnassignedPods bool
	useWorkloadmetaForPods    bool
	WorkloadmetaReflector     *workloadmetaReflector
	workloadmetaStore         workloadmeta.Component

	callbackEnabledResources map[string]bool // resource types that should have callbacks enabled

	eventCallbacks map[string]map[store.StoreEventType]store.StoreEventCallback
	eventMutex     sync.RWMutex
}

// New returns new Builder instance
func New() *Builder {
	return &Builder{
		ksmBuilder:               ksmbuild.NewBuilder(),
		callbackEnabledResources: make(map[string]bool),
		eventCallbacks:           make(map[string]map[store.StoreEventType]store.StoreEventCallback),
	}
}

// WithNamespaces sets the namespaces property of a Builder.
func (b *Builder) WithNamespaces(nss options.NamespaceList) {
	b.namespaces = nss
	b.ksmBuilder.WithNamespaces(nss)
}

// WithFamilyGeneratorFilter configures the white or blacklisted metric to be
// exposed by the store build by the Builder.
func (b *Builder) WithFamilyGeneratorFilter(l generator.FamilyGeneratorFilter) {
	b.allowDenyList = l
	b.ksmBuilder.WithFamilyGeneratorFilter(l)
}

// WithCallbacksForResources configures which resource types should have event callbacks enabled
func (b *Builder) WithCallbacksForResources(resourceTypes []string) {
	for _, resourceType := range resourceTypes {
		b.callbackEnabledResources[resourceType] = true
	}
}

// RegisterStoreEventCallback registers a callback for a specific resource type and event type
func (b *Builder) RegisterStoreEventCallback(resourceType string, eventType store.StoreEventType, callback store.StoreEventCallback) {
	b.eventMutex.Lock()
	defer b.eventMutex.Unlock()

	if b.eventCallbacks[resourceType] == nil {
		b.eventCallbacks[resourceType] = make(map[store.StoreEventType]store.StoreEventCallback)
	}
	b.eventCallbacks[resourceType][eventType] = callback
}

// NotifyStoreEvent calls the registered callback for a resource type and event type
func (b *Builder) NotifyStoreEvent(eventType store.StoreEventType, resourceType string, obj interface{}) {
	b.eventMutex.RLock()
	resourceCallbacks, resourceExists := b.eventCallbacks[resourceType]
	if !resourceExists {
		b.eventMutex.RUnlock()
		return
	}

	callback, callbackExists := resourceCallbacks[eventType]
	b.eventMutex.RUnlock()

	if callbackExists {
		namespace, name := store.ExtractNamespaceAndName(obj)
		callback(eventType, resourceType, namespace, name, obj)
	}
}

// WithFieldSelectorFilter sets the fieldSelector property of a Builder.
func (b *Builder) WithFieldSelectorFilter(fieldSelectors string) {
	b.fieldSelectorFilter = fieldSelectors
	b.ksmBuilder.WithFieldSelectorFilter(fieldSelectors)
}

// WithKubeClient sets the kubeClient property of a Builder.
func (b *Builder) WithKubeClient(c clientset.Interface) {
	b.kubeClient = c
	b.ksmBuilder.WithKubeClient(c)
}

// WithCustomResourceClients sets the customResourceClients property of a Builder.
func (b *Builder) WithCustomResourceClients(clients map[string]interface{}) {
	b.customResourceClients = clients
	b.ksmBuilder.WithCustomResourceClients(clients)
}

// WithMetrics sets the metrics property of a Builder.
func (b *Builder) WithMetrics(r prometheus.Registerer) {
	b.ksmBuilder.WithMetrics(r)
	b.metrics = watch.NewListWatchMetrics(r)
}

// WithEnabledResources sets the enabledResources property of a Builder.
func (b *Builder) WithEnabledResources(c []string) error {
	return b.ksmBuilder.WithEnabledResources(c)
}

// WithContext sets the ctx property of a Builder.
func (b *Builder) WithContext(ctx context.Context) {
	b.ksmBuilder.WithContext(ctx)
	b.ctx = ctx
}

// WithGenerateStoresFunc configures a constom generate store function
func (b *Builder) WithGenerateStoresFunc(f ksmtypes.BuildStoresFunc) {
	b.ksmBuilder.WithGenerateStoresFunc(f)
}

// WithGenerateCustomResourceStoresFunc configures a constom generate store function
func (b *Builder) WithGenerateCustomResourceStoresFunc(f ksmtypes.BuildCustomResourceStoresFunc) {
	b.ksmBuilder.WithGenerateCustomResourceStoresFunc(f)
}

// WithCustomResourceStoreFactories configures a constom store factory
func (b *Builder) WithCustomResourceStoreFactories(fs ...customresource.RegistryFactory) {
	b.ksmBuilder.WithCustomResourceStoreFactories(fs...)
}

// WithAllowLabels configures which labels can be returned for metrics
func (b *Builder) WithAllowLabels(l map[string][]string) error {
	return b.ksmBuilder.WithAllowLabels(l)
}

// WithAllowAnnotations configures which annotations can be returned for metrics
func (b *Builder) WithAllowAnnotations(l map[string][]string) {
	_ = b.ksmBuilder.WithAllowAnnotations(l)
}

// WithPodCollectionFromWorkloadmeta configures the builder to collect pods from
// workloadmeta instead of the API server. This has no effect if pod collection
// is disabled.
func (b *Builder) WithPodCollectionFromWorkloadmeta(store workloadmeta.Component) {
	b.useWorkloadmetaForPods = true
	b.workloadmetaStore = store
}

// WithUnassignedPodsCollection configures the builder to only collect pods that
// are not assigned to any node. This has no effect if pod collection is
// disabled.
func (b *Builder) WithUnassignedPodsCollection() {
	b.collectOnlyUnassignedPods = true
}

// Build initializes and registers all enabled stores.
// Returns metric writers.
func (b *Builder) Build() metricsstore.MetricsWriterList {
	return b.ksmBuilder.Build()
}

// BuildStores initializes and registers all enabled stores.
// It returns metric cache stores.
func (b *Builder) BuildStores() [][]cache.Store {
	stores := b.ksmBuilder.BuildStores()

	if b.WorkloadmetaReflector != nil {
		// Starting the workloadmeta reflector here allows us to start just one for all stores.
		err := b.WorkloadmetaReflector.start(b.ctx)
		if err != nil {
			log.Errorf("Failed to start the workloadmeta reflector: %s", err)
		}
	}

	return stores
}

// WithResync is used if a resync period is configured
func (b *Builder) WithResync(r time.Duration) {
	b.resync = r
}

// WithUsingAPIServerCache sets the API server cache usage
func (b *Builder) WithUsingAPIServerCache(u bool) {
	log.Debug("Using API server cache")
	b.ksmBuilder.WithUsingAPIServerCache(u)
}

// GenerateStores is used to generate new Metrics Store for Metrics Families
func GenerateStores[T any](
	b *Builder,
	metricFamilies []generator.FamilyGenerator,
	expectedType interface{},
	client T,
	listWatchFunc func(kubeClient T, ns string, fieldSelector string) cache.ListerWatcher,
	useAPIServerCache bool,
) []cache.Store {
	filteredMetricFamilies := generator.FilterFamilyGenerators(b.allowDenyList, metricFamilies)
	composedMetricGenFuncs := generator.ComposeMetricGenFuncs(filteredMetricFamilies)

	isPod := false
	if _, ok := expectedType.(*corev1.Pod); ok {
		isPod = true
	} else if u, ok := expectedType.(*unstructured.Unstructured); ok {
		isPod = u.GetAPIVersion() == "v1" && u.GetKind() == "Pod"
	} else if _, ok := expectedType.(*corev1.ConfigMap); ok {
		configMapStore, err := generateConfigMapStores(b, metricFamilies, useAPIServerCache)
		if err != nil {
			log.Debugf("Defaulting to kube-state-metrics for configmap collection: %v", err)
		} else {
			log.Debug("Using meta.k8s.io API for configmap collection")
			return configMapStore
		}
	}

	if b.namespaces.IsAllNamespaces() {
		store := b.createStoreForType(composedMetricGenFuncs, expectedType)

		if isPod {
			// Pods are handled differently because depending on the configuration
			// they're collected from the API server or the Kubelet.
			handlePodCollection(b, store, client, listWatchFunc, corev1.NamespaceAll, useAPIServerCache)
		} else {
			listWatcher := listWatchFunc(client, corev1.NamespaceAll, b.fieldSelectorFilter)
			b.startReflector(expectedType, store, listWatcher, useAPIServerCache)
		}
		return []cache.Store{store}

	}

	stores := make([]cache.Store, 0, len(b.namespaces))
	for _, ns := range b.namespaces {
		store := b.createStoreForType(composedMetricGenFuncs, expectedType)
		if isPod {
			// Pods are handled differently because depending on the configuration
			// they're collected from the API server or the Kubelet.
			handlePodCollection(b, store, client, listWatchFunc, ns, useAPIServerCache)
		} else {
			listWatcher := listWatchFunc(client, ns, b.fieldSelectorFilter)
			b.startReflector(expectedType, store, listWatcher, useAPIServerCache)
		}
		stores = append(stores, store)
	}

	return stores
}

// GenerateStores is used to generate new Metrics Store for the given metric families
func (b *Builder) GenerateStores(
	metricFamilies []generator.FamilyGenerator,
	expectedType interface{},
	listWatchFunc func(kubeClient clientset.Interface, ns string, fieldSelector string) cache.ListerWatcher,
	useAPIServerCache bool,
) []cache.Store {
	return GenerateStores(b, metricFamilies, expectedType, b.kubeClient, listWatchFunc, useAPIServerCache)
}

func (b *Builder) getCustomResourceClient(resourceName string) interface{} {
	if client, ok := b.customResourceClients[resourceName]; ok {
		return client
	}

	return b.kubeClient
}

// GenerateCustomResourceStoresFunc use to generate new Metrics Store for Metrics Families
func (b *Builder) GenerateCustomResourceStoresFunc(
	resourceName string,
	metricFamilies []generator.FamilyGenerator,
	expectedType interface{},
	listWatchFunc func(kubeClient interface{}, ns string, fieldSelector string) cache.ListerWatcher,
	useAPIServerCache bool,
) []cache.Store {
	return GenerateStores(b, metricFamilies,
		expectedType,
		b.getCustomResourceClient(resourceName),
		listWatchFunc,
		useAPIServerCache,
	)
}

// startReflector creates a Kubernetes client-go reflector with the given
// listWatcher for each given namespace and registers it with the given store.
func (b *Builder) startReflector(
	expectedType interface{},
	store cache.Store,
	listWatcher cache.ListerWatcher,
	useAPIServerCache bool,
) {
	if useAPIServerCache {
		listWatcher = newCacheEnabledListerWatcher(listWatcher)
	}
	reflector := cache.NewReflector(listWatcher, expectedType, store, b.resync*time.Second)
	go reflector.Run(b.ctx.Done())
}

type cacheEnabledListerWatcher struct {
	lw cache.ListerWatcherWithContext
	rv string
}

func newCacheEnabledListerWatcher(lw cache.ListerWatcher) cache.ListerWatcher {
	return &cacheEnabledListerWatcher{
		lw: cache.ToListerWatcherWithContext(lw),
		rv: "0",
	}
}

// List uses `ResourceVersion` and `ResourceVersionMatch=NotOlderThan` to avoid a quorum from ETCD.
// https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions
// The first list will use RV=0, and the subsequent list will use the RV from the previous list.
// The APIServer will return any data more recent than the rv, preferring the latest one.
// The implementation differs from kube-state-metrics that uses rv = 0 for list operations.
// https://github.com/kubernetes/kube-state-metrics/blob/7995d5fd23bcff7ae24ab6849f7c393d262fb025/pkg/watch/watch.go#L77
func (c *cacheEnabledListerWatcher) List(options v1.ListOptions) (runtime.Object, error) {
	options.ResourceVersion = c.rv
	options.ResourceVersionMatch = v1.ResourceVersionMatchNotOlderThan
	res, err := c.lw.ListWithContext(context.TODO(), options)
	if err == nil {
		metadataAccessor, err := meta.ListAccessor(res)
		if err != nil {
			return nil, err
		}
		c.rv = metadataAccessor.GetResourceVersion()
	}

	return res, err
}

// Watch simply delegates to the wrapped ListerWatcherWithContext
func (c *cacheEnabledListerWatcher) Watch(options v1.ListOptions) (apiwatch.Interface, error) {
	return c.lw.WatchWithContext(context.TODO(), options)
}

func handlePodCollection[T any](b *Builder, store cache.Store, client T, listWatchFunc func(kubeClient T, ns string, fieldSelector string) cache.ListerWatcher, namespace string, useAPIServerCache bool) {
	if b.useWorkloadmetaForPods {
		if b.WorkloadmetaReflector == nil {
			wr, err := newWorkloadmetaReflector(b.workloadmetaStore, b.namespaces)
			if err != nil {
				log.Errorf("Failed to create workloadmetaReflector: %s", err)
				return
			}
			b.WorkloadmetaReflector = &wr
		}

		err := b.WorkloadmetaReflector.addStore(store)
		if err != nil {
			log.Errorf("Failed to add store to workloadmetaReflector: %s", err)
			return
		}

		// The workloadmeta reflector will be started when all stores are added.
		return
	}

	fieldSelector := b.fieldSelectorFilter
	if b.collectOnlyUnassignedPods {
		// spec.nodeName is set to empty for unassigned pods. This ignores
		// b.fieldSelectorFilter, but I think it's not used.
		fieldSelector = "spec.nodeName="
	}

	listWatcher := listWatchFunc(client, namespace, fieldSelector)
	b.startReflector(&corev1.Pod{}, store, listWatcher, useAPIServerCache)
}

func generateConfigMapStores(
	b *Builder,
	metricFamilies []generator.FamilyGenerator,
	useAPIServerCache bool,
) ([]cache.Store, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create in-cluster config for metadata client: %w", err)
	}

	metadataClient, err := metadata.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	filteredMetricFamilies := generator.FilterFamilyGenerators(b.allowDenyList, metricFamilies)
	composedMetricGenFuncs := generator.ComposeMetricGenFuncs(filteredMetricFamilies)

	stores := make([]cache.Store, 0)

	if b.namespaces.IsAllNamespaces() {
		log.Infof("Using NamespaceAll for ConfigMap collection.")
		store := store.NewMetricsStore(composedMetricGenFuncs, "configmap")
		listWatcher := createConfigMapListWatch(metadataClient, gvr, v1.NamespaceAll)
		b.startReflector(&corev1.ConfigMap{}, store, listWatcher, useAPIServerCache)
		return []cache.Store{store}, nil
	}

	for _, ns := range b.namespaces {
		store := store.NewMetricsStore(composedMetricGenFuncs, "configmap")
		listWatcher := createConfigMapListWatch(metadataClient, gvr, ns)
		b.startReflector(&corev1.ConfigMap{}, store, listWatcher, useAPIServerCache)
		stores = append(stores, store)
	}

	return stores, nil
}

func createConfigMapListWatch(metadataClient metadata.Interface, gvr schema.GroupVersionResource, namespace string) *cache.ListWatch {
	return &cache.ListWatch{
		ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
			result, err := metadataClient.Resource(gvr).Namespace(namespace).List(context.TODO(), options)
			if err != nil {
				return nil, err
			}

			configMapList := &corev1.ConfigMapList{}
			for _, item := range result.Items {
				configMapList.Items = append(configMapList.Items, corev1.ConfigMap{
					ObjectMeta: v1.ObjectMeta{
						Name:            item.GetName(),
						Namespace:       item.GetNamespace(),
						UID:             item.GetUID(),
						ResourceVersion: item.GetResourceVersion(),
					},
				})
			}

			return configMapList, nil
		},
		WatchFunc: func(options v1.ListOptions) (apiwatch.Interface, error) {
			watcher, err := metadataClient.Resource(gvr).Namespace(namespace).Watch(context.TODO(), options)
			if err != nil {
				return nil, err
			}

			return apiwatch.Filter(watcher, func(event apiwatch.Event) (apiwatch.Event, bool) {
				if event.Object == nil {
					return event, false
				}

				partialObject, ok := event.Object.(*v1.PartialObjectMetadata)
				if !ok {
					return event, false
				}

				configMap := &corev1.ConfigMap{
					ObjectMeta: v1.ObjectMeta{
						Name:            partialObject.GetName(),
						Namespace:       partialObject.GetNamespace(),
						UID:             partialObject.GetUID(),
						ResourceVersion: partialObject.GetResourceVersion(),
					},
				}

				event.Object = configMap
				return event, true
			}), nil
		},
	}
}

func (b *Builder) createStoreForType(composedMetricGenFuncs func(interface{}) []metric.FamilyInterface, expectedType interface{}) cache.Store {
	typeName := reflect.TypeOf(expectedType).String()
	metricsStore := store.NewMetricsStore(composedMetricGenFuncs, typeName)

	// Enable callbacks if this resource type is configured for them
	if b.callbackEnabledResources[typeName] {
		metricsStore.EnableCallbacks(b)
	}

	return metricsStore
}
