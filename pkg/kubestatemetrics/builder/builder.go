// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package builder

import (
	"context"
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"

	"github.com/prometheus/client_golang/prometheus"
	vpaclientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	ksmbuild "k8s.io/kube-state-metrics/pkg/builder"
	ksmtypes "k8s.io/kube-state-metrics/pkg/builder/types"
	"k8s.io/kube-state-metrics/pkg/metric_generator"
	"k8s.io/kube-state-metrics/pkg/options"
	"k8s.io/kube-state-metrics/pkg/watch"
)

// Builder struct represents the metric store generator
// Follows the builder pattern: https://en.wikipedia.org/wiki/Builder_pattern
type Builder struct {
	ksmBuilder ksmtypes.BuilderInterface

	kubeClient    clientset.Interface
	vpaClient     vpaclientset.Interface
	namespaces    options.NamespaceList
	ctx           context.Context
	allowDenyList ksmtypes.AllowDenyLister
	metrics       *watch.ListWatchMetrics
	shard         int32
	totalShards   int

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

// WithAllowDenyList configures the white or blacklisted metric to be exposed
// by the store build by the Builder.
func (b *Builder) WithAllowDenyList(l ksmtypes.AllowDenyLister) {
	b.allowDenyList = l
	b.ksmBuilder.WithAllowDenyList(l)
}

// WithSharding sets the shard and totalShards property of a Builder.
func (b *Builder) WithSharding(shard int32, totalShards int) {
	b.shard = shard
	b.totalShards = totalShards
	b.ksmBuilder.WithSharding(shard, totalShards)
}

// WithKubeClient sets the kubeClient property of a Builder.
func (b *Builder) WithKubeClient(c clientset.Interface) {
	b.kubeClient = c
	b.ksmBuilder.WithKubeClient(c)
}

// WithVPAClient sets the vpaClient property of a Builder so that the verticalpodautoscaler collector can query VPA objects.
func (b *Builder) WithVPAClient(c vpaclientset.Interface) {
	b.vpaClient = c
	b.ksmBuilder.WithVPAClient(c)
}

// WithMetrics sets the metrics property of a Builder.
func (b *Builder) WithMetrics(r *prometheus.Registry) {
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

// DefaultGenerateStoreFunc returns default buildStore function
func (b *Builder) DefaultGenerateStoreFunc() ksmtypes.BuildStoreFunc {
	return b.GenerateStore
}

// WithCustomGenerateStoreFunc configures a constom generate store function
func (b *Builder) WithGenerateStoreFunc(f ksmtypes.BuildStoreFunc) {
	b.ksmBuilder.WithGenerateStoreFunc(f)
}

// Build initializes and registers all enabled stores.
func (b *Builder) Build() []cache.Store {
	return b.ksmBuilder.Build()
}

// WithResync is used if a resync period is configured
func (b *Builder) WithResync(r time.Duration) {
	b.resync = r
}

// GenerateStore use to generate new Metrics Store for Metrics Families
func (b *Builder) GenerateStore(metricFamilies []generator.FamilyGenerator,
	expectedType interface{},
	listWatchFunc func(kubeClient clientset.Interface, ns string) cache.ListerWatcher,
) cache.Store {
	filteredMetricFamilies := generator.FilterMetricFamilies(b.allowDenyList, metricFamilies)
	composedMetricGenFuncs := generator.ComposeMetricGenFuncs(filteredMetricFamilies)
	store := store.NewMetricsStore(
		composedMetricGenFuncs,
		// Used later on to identify the Type of resource.
		reflect.TypeOf(expectedType).String(),
	)
	b.reflectorPerNamespace(expectedType, store, listWatchFunc)
	return store
}

// reflectorPerNamespace creates a Kubernetes client-go reflector with the given
// listWatchFunc for each given namespace and registers it with the given store.
func (b *Builder) reflectorPerNamespace(
	expectedType interface{},
	store cache.Store,
	listWatchFunc func(kubeClient clientset.Interface, ns string) cache.ListerWatcher,
) {
	for _, ns := range b.namespaces {
		lw := listWatchFunc(b.kubeClient, ns) //instrumentedListWatch := watch.NewInstrumentedListerWatcher(lw, g.metrics, reflect.TypeOf(expectedType).String())
		reflector := cache.NewReflector(lw, expectedType, store, b.resync*time.Second)
		go reflector.Run(b.ctx.Done())
	}
}
