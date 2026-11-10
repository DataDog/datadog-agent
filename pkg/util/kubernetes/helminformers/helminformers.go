// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package helminformers shares the informers on Helm release Secrets and
// ConfigMaps between the checks of a process, so their payloads are cached once.
package helminformers

import (
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// LabelSelector selects the objects Helm uses to store releases.
	LabelSelector = "owner=helm"
	// ResyncPeriod is the resync period of the shared informers.
	ResyncPeriod = 5 * time.Minute
)

// Kind is the type of object Helm stores releases in.
type Kind string

const (
	// Secrets is Helm's default storage driver.
	Secrets Kind = "secret"
	// ConfigMaps is Helm's configmap storage driver.
	ConfigMaps Kind = "configmap"
)

type key struct {
	client kubernetes.Interface
	kind   Kind
}

type entry struct {
	informer cache.SharedIndexInformer
	stopCh   chan struct{}
	refs     int
}

var (
	mu      sync.Mutex
	entries = map[key]*entry{}
)

// Acquire returns the shared, already running informer for kind, and a release
// function to call once its handlers are removed; the last release stops it.
func Acquire(client kubernetes.Interface, kind Kind) (cache.SharedIndexInformer, func()) {
	mu.Lock()
	defer mu.Unlock()

	k := key{client: client, kind: kind}
	e, found := entries[k]
	if !found {
		e = newEntry(client, kind)
		entries[k] = e
	}
	e.refs++

	var once sync.Once
	return e.informer, func() {
		once.Do(func() {
			mu.Lock()
			defer mu.Unlock()

			e.refs--
			if e.refs == 0 {
				close(e.stopCh)
				delete(entries, k)
			}
		})
	}
}

// Owns reports whether informer is a shared informer of the registry, which
// runs it: its holders must not run it themselves.
func Owns(informer cache.SharedInformer) bool {
	mu.Lock()
	defer mu.Unlock()
	for _, e := range entries {
		if cache.SharedInformer(e.informer) == informer {
			return true
		}
	}
	return false
}

func newEntry(client kubernetes.Interface, kind Kind) *entry {
	factory := informers.NewSharedInformerFactoryWithOptions(client, ResyncPeriod,
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = LabelSelector
		}))

	var informer cache.SharedIndexInformer
	switch kind {
	case Secrets:
		informer = factory.Core().V1().Secrets().Informer()
	default:
		informer = factory.Core().V1().ConfigMaps().Informer()
	}

	if err := informer.SetTransform(trim); err != nil {
		log.Debugf("Unable to set transform on the Helm release %s informer: %v", kind, err)
	}

	e := &entry{informer: informer, stopCh: make(chan struct{})}
	go informer.Run(e.stopCh)
	return e
}

// trim drops fields that no consumer needs before objects are stored in the
// cache. The last-applied-configuration annotation, when present, holds a full
// copy of the object data.
func trim(obj interface{}) (interface{}, error) {
	switch o := obj.(type) {
	case *corev1.ConfigMap:
		o.ManagedFields = nil
		redact.RemoveSensitiveAnnotationsAndLabels(o.Annotations, o.Labels)
	case *corev1.Secret:
		o.ManagedFields = nil
		redact.RemoveSensitiveAnnotationsAndLabels(o.Annotations, o.Labels)
	}
	return obj, nil
}
