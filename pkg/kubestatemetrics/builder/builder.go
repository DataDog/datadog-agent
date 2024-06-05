// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package builder

import (
	"context"
	"reflect"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	vpaclientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	ksmbuild "k8s.io/kube-state-metrics/v2/pkg/builder"
	ksmtypes "k8s.io/kube-state-metrics/v2/pkg/builder/types"
	"k8s.io/kube-state-metrics/v2/pkg/customresource"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
	metricsstore "k8s.io/kube-state-metrics/v2/pkg/metrics_store"
	"k8s.io/kube-state-metrics/v2/pkg/options"
	"k8s.io/kube-state-metrics/v2/pkg/watch"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Builder struct represents the metric store generator
// Follows the builder pattern: https://en.wikipedia.org/wiki/Builder_pattern
type Builder struct {
	ksmBuilder ksmtypes.BuilderInterface

	customResourceClients map[string]interface{}
	kubeClient            clientset.Interface
	vpaClient             vpaclientset.Interface
	namespaces            options.NamespaceList
	fieldSelectorFilter   string
	ctx                   context.Context
	allowDenyList         generator.FamilyGeneratorFilter
	metrics               *watch.ListWatchMetrics

	resync time.Duration
}

// New returns new Builder instance
func New() *Builder {
	return &Builder{
		ksmBuilder: ksmbuild.NewBuilder(),
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

// WithVPAClient sets the vpaClient property of a Builder so that the verticalpodautoscaler collector can query VPA objects.
func (b *Builder) WithVPAClient(c vpaclientset.Interface) {
	b.vpaClient = c
	b.ksmBuilder.WithVPAClient(c)
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
	b.ksmBuilder.WithAllowAnnotations(l)
}

// Build initializes and registers all enabled stores.
// Returns metric writers.
func (b *Builder) Build() metricsstore.MetricsWriterList {
	return b.ksmBuilder.Build()
}

// BuildStores initializes and registers all enabled stores.
// It returns metric cache stores.
func (b *Builder) BuildStores() [][]cache.Store {
	return b.ksmBuilder.BuildStores()
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

	if b.namespaces.IsAllNamespaces() {
		store := store.NewMetricsStore(composedMetricGenFuncs, reflect.TypeOf(expectedType).String())
		listWatcher := listWatchFunc(client, corev1.NamespaceAll, b.fieldSelectorFilter)
		b.startReflector(expectedType, store, listWatcher, useAPIServerCache)
		return []cache.Store{store}

	}

	stores := make([]cache.Store, 0, len(b.namespaces))
	for _, ns := range b.namespaces {
		store := store.NewMetricsStore(composedMetricGenFuncs, reflect.TypeOf(expectedType).String())
		listWatcher := listWatchFunc(client, ns, b.fieldSelectorFilter)
		b.startReflector(expectedType, store, listWatcher, useAPIServerCache)
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
	useApiServerCache bool,
) {
	if useApiServerCache {
		listWatcher = &cacheEnabledListerWatcher{ListerWatcher: listWatcher}
	}
	reflector := cache.NewReflector(listWatcher, expectedType, store, b.resync*time.Second)
	go reflector.Run(b.ctx.Done())
}

type cacheEnabledListerWatcher struct {
	cache.ListerWatcher
}

// List uses a resource version of "0" to list all resources.
// The implementation is required to avoid a quorum from ETCD.
// https://github.com/kubernetes/kubernetes/blob/dae37c21645c0fc04bbb0202ad1eba6ba4438d23/staging/src/k8s.io/apiserver/pkg/storage/cacher/cacher.go#L526-L527
// Refer to https://github.com/kubernetes/kube-state-metrics/blob/7995d5fd23bcff7ae24ab6849f7c393d262fb025/pkg/watch/watch.go#L77
func (c *cacheEnabledListerWatcher) List(options v1.ListOptions) (runtime.Object, error) {
	options.ResourceVersion = "0"
	return c.ListerWatcher.List(options)
}
