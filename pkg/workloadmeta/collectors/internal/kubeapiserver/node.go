// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func newNodeReflectorStore(wlmetaStore workloadmeta.Store) *reflectorStore {
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
	node := obj.(*corev1.Node)

	return &workloadmeta.KubernetesNode{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesNode,
			ID:   node.Name,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        node.Name,
			Annotations: node.Annotations,
			Labels:      node.Labels,
		},
	}
}
