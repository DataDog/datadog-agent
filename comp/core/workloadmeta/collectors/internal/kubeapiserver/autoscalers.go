// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// autoscalerResource is an autoscaler resource collected to tag the workloads
// it targets. KEDA is not listed: it's detected through the HPAs it creates.
type autoscalerResource struct {
	groupResource schema.GroupResource
	kind          workloadmeta.AutoscalerKind
}

var autoscalerResources = []autoscalerResource{
	{
		groupResource: schema.GroupResource{
			Group:    "autoscaling",
			Resource: "horizontalpodautoscalers",
		},
		kind: workloadmeta.AutoscalerKindHPA,
	},
	{
		groupResource: schema.GroupResource{
			Group:    "autoscaling.k8s.io",
			Resource: "verticalpodautoscalers",
		},
		kind: workloadmeta.AutoscalerKindVPA,
	},
	{
		groupResource: schema.GroupResource{
			Group:    "datadoghq.com",
			Resource: "watermarkpodautoscalers",
		},
		kind: workloadmeta.AutoscalerKindWPA,
	},
	{
		groupResource: schema.GroupResource{
			Group:    "datadoghq.com",
			Resource: "datadogpodautoscalers",
		},
		kind: workloadmeta.AutoscalerKindDPA,
	},
}

func shouldHaveAutoscalerStores(cfg config.Reader) bool {
	return cfg.GetBool("cluster_agent.autoscaler_tags.enabled")
}

// autoscalerFilter filters out autoscalers that don't target a supported
// workload.
type autoscalerFilter struct{}

func (f *autoscalerFilter) filteredOut(entity workloadmeta.Entity) bool {
	autoscaler := entity.(*workloadmeta.KubernetesAutoscaler)
	return autoscaler.TargetKind == ""
}

// startAutoscalerStores starts a store for each autoscaler kind that can be
// listed. probeClient is used for the startup checks and must have a request
// timeout, because they block the collector start. client is used by the
// stores.
func startAutoscalerStores(ctx context.Context, wlmetaStore workloadmeta.Component, discoveryClient discovery.DiscoveryInterface, probeClient dynamic.Interface, client dynamic.Interface) []syncedStore {
	var stores []syncedStore
	for kind, gvr := range autoscalerGVRsToCollect(ctx, discoveryClient, probeClient) {
		reflector, store, err := newAutoscalerStore(wlmetaStore, client, gvr, kind)
		if err != nil {
			log.Errorf("failed to create autoscaler store for %s: %v", gvr.Resource, err)
			continue
		}
		stores = append(stores, store)
		go reflector.Run(ctx.Done())
	}
	return stores
}

func autoscalerGVRsToCollect(ctx context.Context, discoveryClient discovery.DiscoveryInterface, client dynamic.Interface) map[workloadmeta.AutoscalerKind]schema.GroupVersionResource {
	// Not using getGVRsForRequestedResources because it logs missing resources
	// as errors, and autoscaler CRDs are optional.
	groupResourceToVersion, err := discoverGroupResourceVersions(discoveryClient)
	if err != nil {
		log.Errorf("failed to discover autoscaler resources, autoscaler tags disabled: %v", err)
		return nil
	}

	gvrs := make(map[workloadmeta.AutoscalerKind]schema.GroupVersionResource, len(autoscalerResources))
	for _, resource := range autoscalerResources {
		version, found := groupResourceToVersion[resource.groupResource]
		if !found {
			log.Infof("%s not found in the API server, %s autoscaler tags disabled", resource.groupResource, resource.kind)
			continue
		}

		gvr := resource.groupResource.WithVersion(version)
		// The stores are part of the readiness check, so a kind that can't be
		// listed is skipped instead of blocking readiness
		_, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{Limit: 1})
		if k8serrors.IsForbidden(err) {
			log.Warnf("missing RBAC to list %s, %s autoscaler tags disabled until restart: %v", resource.groupResource, resource.kind, err)
			continue
		}
		if err != nil {
			log.Warnf("failed to list %s, %s autoscaler tags disabled until restart: %v", resource.groupResource, resource.kind, err)
			continue
		}

		gvrs[resource.kind] = gvr
	}

	return gvrs
}

func newAutoscalerStore(wlmetaStore workloadmeta.Component, client dynamic.Interface, gvr schema.GroupVersionResource, kind workloadmeta.AutoscalerKind) (*cache.Reflector, *reflectorStore, error) {
	parser, err := kubernetesresourceparsers.NewAutoscalerParser(kind)
	if err != nil {
		return nil, nil, fmt.Errorf("creating %s parser: %w", gvr.Resource, err)
	}

	listerWatcher := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, options metav1.ListOptions) (runtime.Object, error) {
			obj, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).List(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("listing %s: %w", gvr.Resource, err)
			}
			return obj, nil
		},
		WatchFuncWithContext: func(ctx context.Context, options metav1.ListOptions) (watch.Interface, error) {
			watcher, err := client.Resource(gvr).Namespace(metav1.NamespaceAll).Watch(ctx, options)
			if err != nil {
				return nil, fmt.Errorf("watching %s: %w", gvr.Resource, err)
			}
			return watcher, nil
		},
	}

	store := newAutoscalerReflectorStore(wlmetaStore, parser)
	reflector := cache.NewNamedReflector(
		componentName,
		listerWatcher,
		&unstructured.Unstructured{},
		store,
		noResync,
	)
	return reflector, store, nil
}

func newAutoscalerReflectorStore(wlmetaStore workloadmeta.Component, parser kubernetesresourceparsers.ObjectParser) *reflectorStore {
	return &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
		filter:      &autoscalerFilter{},
	}
}
