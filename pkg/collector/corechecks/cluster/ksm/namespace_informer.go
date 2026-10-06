// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type namespaceInformer interface {
	Subscribe(kubernetes.Interface, ksmstore.DynamicStore, []string) (cache.ResourceEventHandlerRegistration, error)
	Unsubscribe(cache.ResourceEventHandlerRegistration) error
}

// sharedNamespaceInformer owns one namespace watch for all KSM shards in this process.
type sharedNamespaceInformer struct {
	ctx context.Context

	mu       sync.Mutex
	informer cache.SharedIndexInformer
}

func newSharedNamespaceInformer(ctx context.Context) *sharedNamespaceInformer {
	return &sharedNamespaceInformer{ctx: ctx}
}

func (s *sharedNamespaceInformer) Subscribe(client kubernetes.Interface, stores ksmstore.DynamicStore, namespaces []string) (cache.ResourceEventHandlerRegistration, error) {
	s.mu.Lock()
	informer := s.informer
	start := false
	if informer == nil {
		informer = coreinformers.NewNamespaceInformer(client, 0*time.Second, cache.Indexers{})
		s.informer = informer
		start = true
	}

	registration, err := informer.AddEventHandler(namespaceEventHandler(stores, namespaces))
	if err != nil {
		if start {
			s.informer = nil
		}
		s.mu.Unlock()
		return nil, fmt.Errorf("registering namespace event handler: %w", err)
	}
	s.mu.Unlock()

	if start {
		go informer.RunWithContext(s.ctx)
	}
	return registration, nil
}

func (s *sharedNamespaceInformer) Unsubscribe(registration cache.ResourceEventHandlerRegistration) error {
	if registration == nil {
		return nil
	}

	s.mu.Lock()
	informer := s.informer
	s.mu.Unlock()
	if informer == nil {
		return nil
	}

	return informer.RemoveEventHandler(registration)
}

func namespaceEventHandler(stores ksmstore.DynamicStore, namespaces []string) cache.ResourceEventHandler {
	allNamespaces := len(namespaces) == 1 && namespaces[0] == corev1.NamespaceAll
	enabledNamespaces := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		enabledNamespaces[namespace] = struct{}{}
	}
	shouldHandle := func(namespace string) bool {
		_, enabled := enabledNamespaces[namespace]
		return allNamespaces || enabled
	}

	return cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			namespace, ok := obj.(*corev1.Namespace)
			if !ok {
				log.Errorf("namespace informer received unexpected add object %T", obj)
				return
			}
			if shouldHandle(namespace.Name) {
				stores.Add(namespace.Name)
			}
		},
		DeleteFunc: func(obj interface{}) {
			name, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				log.Errorf("namespace informer could not identify deleted object %T: %s", obj, err)
				return
			}
			if shouldHandle(name) {
				stores.Del(name)
			}
		},
	}
}
