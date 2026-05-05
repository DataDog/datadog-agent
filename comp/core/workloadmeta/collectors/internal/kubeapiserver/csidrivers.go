// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"context"

	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core/config"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newCSIDriverStore(ctx context.Context, wlm workloadmeta.Component, cfg config.Reader, client kubernetes.Interface) (*cache.Reflector, *reflectorStore) {
	log.Info("Starting workloadmeta informer for storage.k8s.io/v1.CSIDrivers")

	csiDriverListerWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.StorageV1().CSIDrivers().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.StorageV1().CSIDrivers().Watch(ctx, options)
		},
	}

	csiDriverStore := newCSIDriverReflectorStore(wlm, cfg)
	csiDriverReflector := cache.NewNamedReflector(
		componentName,
		csiDriverListerWatcher,
		&storagev1.CSIDriver{},
		csiDriverStore,
		noResync,
	)
	return csiDriverReflector, csiDriverStore
}

func newCSIDriverReflectorStore(wlmetaStore workloadmeta.Component, _ config.Reader) *reflectorStore {
	return &reflectorStore{
		wlmetaStore: wlmetaStore,
		seen:        make(map[string]workloadmeta.EntityID),
		parser:      kubernetesresourceparsers.NewCSIDriverParser(),
	}
}
