// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newNodeStore(ctx context.Context, wlm workloadmeta.Component, client kubernetes.Interface, metadataclient metadata.Client) (*cache.Reflector, *reflectorStore) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

	nodeListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return metadataclient.Resource(gvr).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return metadataclient.Resource(gvr).Watch(ctx, options)
		},
	}

	nodeStore := newNodeReflectorStore(wlm)
	nodeReflector := cache.NewNamedReflector(
		componentName,
		nodeListerWatcher,
		&v1.PartialObjectMetadata{},
		nodeStore,
		noResync,
	)
	log.Debug("node reflector enabled")
	return nodeReflector, nodeStore
}

func newNodeReflectorStore(wlmetaStore workloadmeta.Component) *reflectorStore {
	store := &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      newNodeParser(),
	}

	return store
}

type nodeParser struct{}

func newNodeParser() objectParser {
	return nodeParser{}
}

func (p nodeParser) Parse(obj interface{}) workloadmeta.Entity {
	nodemeta := obj.(*v1.PartialObjectMetadata)

	log.Infof("Parsing node from node collector: ", *nodemeta)
	return &workloadmeta.KubernetesNode{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNode,
			ID:   nodemeta.Name,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        nodemeta.Name,
			Annotations: nodemeta.Annotations,
			Labels:      nodemeta.Labels,
		},
	}
}
