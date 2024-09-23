// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newMetadataStore(ctx context.Context, wlmetaStore workloadmeta.Component, config config.Reader, metadataclient metadata.Interface, gvr schema.GroupVersionResource) (*cache.Reflector, *reflectorStore) {
	metadataListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return metadataclient.Resource(gvr).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return metadataclient.Resource(gvr).Watch(ctx, options)
		},
	}

	annotationsExclude := config.GetStringSlice("cluster_agent.kube_metadata_collection.resource_annotations_exclude")
	parser, err := kubernetesresourceparsers.NewMetadataParser(gvr, annotationsExclude)
	if err != nil {
		_ = log.Errorf("unable to parse all resource_annotations_exclude: %v, err:", err)
		parser, _ = kubernetesresourceparsers.NewMetadataParser(gvr, nil)
	}

	metadataStore := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      parser,
		filter:      nil,
	}
	metadataReflector := cache.NewNamedReflector(
		componentName,
		metadataListerWatcher,
		&metav1.PartialObjectMetadata{},
		metadataStore,
		noResync,
	)
	return metadataReflector, metadataStore
}
