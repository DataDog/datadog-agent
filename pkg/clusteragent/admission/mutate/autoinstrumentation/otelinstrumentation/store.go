// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package otelinstrumentation makes a workload configured for the community
// OpenTelemetry Operator keep working when that operator is replaced by the Datadog
// Cluster Agent.
//
// Store caches the Operator's Instrumentation custom resources, and Resolver answers
// what a pod's instrumentation.opentelemetry.io/inject-<lang> annotations ask for,
// reproducing upstream's semantics. Both read in-memory caches only and never fail
// admission. Translate then turns a resolved custom resource into per-language Datadog
// configuration for swap mode, where the Datadog tracing library is injected in place of
// the community SDK and configured with DD_* variables. Passthrough mode, which keeps
// the community SDK images, is not implemented.
//
// The Store and the Resolver are built by the cluster-agent start command when
// apm_config.instrumentation.otel_instrumentation_crd_mode names an enabled mode, and
// the Resolver is handed to the target mutator. A nil Resolver there means the CRD is not watched at
// all, which the admission path treats the same way as an absent CRD.
package otelinstrumentation

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v7"
	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InstrumentationGVR is the GVR of the community OpenTelemetry Operator's Instrumentation
// CRD. The resource is namespace-scoped.
var InstrumentationGVR = otelv1alpha1.GroupVersion.WithResource("instrumentations")

const (
	informerResyncPeriod = 5 * time.Minute

	// Same backoff as the DatadogInstrumentation controller's CRD check. A zero max
	// elapsed time means the check only ever gives up when the context is cancelled.
	crdCheckInitialInterval = 5 * time.Second
	crdCheckMaxInterval     = 5 * time.Minute
	crdCheckMultiplier      = 2.0
	crdCheckMaxElapsedTime  = 0
)

// Store keeps the cluster's Instrumentation custom resources in memory so that pod
// admission can resolve one without an API call.
//
// The CRD belongs to the community OpenTelemetry Operator and is therefore optional:
// a Store whose CRD never shows up stays inert and every lookup misses. Since a lookup
// sits on the admission hot path it never reports an error either — a missing CRD, a
// cache that has not synced yet and an absent custom resource are all an ordinary miss,
// and the caller keeps evaluating the other SSI mechanisms.
type Store struct {
	client dynamic.Interface

	mu     sync.RWMutex
	synced bool
	crs    map[string]*otelv1alpha1.Instrumentation
}

// NewStore returns a Store that is inert until Run starts it.
func NewStore(client dynamic.Interface) *Store {
	return &Store{
		client: client,
		crs:    make(map[string]*otelv1alpha1.Instrumentation),
	}
}

// Get returns the Instrumentation custom resource in the given namespace, reading only
// the local cache. The returned custom resource is shared with the cache and must not be
// modified.
func (s *Store) Get(namespace, name string) (*otelv1alpha1.Instrumentation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.synced {
		return nil, false
	}
	cr, ok := s.crs[storeKey(namespace, name)]
	return cr, ok
}

// ListNamespace returns every Instrumentation custom resource in the given namespace,
// sorted by name, reading only the local cache. The second return value reports whether
// the Store is serving at all, which distinguishes an inert or unsynced Store from a
// namespace that genuinely holds no custom resource — the caller needs that difference
// because an empty namespace is what makes an inject-<lang> value of "true"
// unresolvable. The returned custom resources are shared with the cache and must not be
// modified.
func (s *Store) ListNamespace(namespace string) ([]*otelv1alpha1.Instrumentation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.synced {
		return nil, false
	}

	crs := make([]*otelv1alpha1.Instrumentation, 0, len(s.crs))
	for _, cr := range s.crs {
		if cr.Namespace == namespace {
			crs = append(crs, cr)
		}
	}
	sort.Slice(crs, func(i, j int) bool { return crs[i].Name < crs[j].Name })
	return crs, true
}

// HasSynced reports whether the Store has completed its initial cache sync. Until it
// has, every lookup misses.
func (s *Store) HasSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.synced
}

// Run starts tracking Instrumentation custom resources and blocks until ctx is done.
// It waits for the CRD to exist first, so that a cluster without the community
// OpenTelemetry Operator never gets an informer whose reflector would keep retrying a
// resource the API server does not serve.
func (s *Store) Run(ctx context.Context) {
	if err := s.waitForCRD(ctx); err != nil {
		log.Infof("OpenTelemetry Instrumentation store will not start: %v", err)
		return
	}

	factory := dynamicinformer.NewDynamicSharedInformerFactory(s.client, informerResyncPeriod)
	informer := factory.ForResource(InstrumentationGVR).Informer()
	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    s.upsert,
		UpdateFunc: func(_, newObj interface{}) { s.upsert(newObj) },
		DeleteFunc: s.remove,
	}); err != nil {
		log.Errorf("Cannot add event handler to OpenTelemetry Instrumentation informer: %v", err)
		return
	}

	log.Infof("Starting OpenTelemetry Instrumentation store (waiting for cache sync)")
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Errorf("Failed to wait for OpenTelemetry Instrumentation cache to sync")
		return
	}
	s.setSynced()

	log.Infof("Started OpenTelemetry Instrumentation store (cache sync finished)")
	<-ctx.Done()
	log.Infof("Stopping OpenTelemetry Instrumentation store")
}

func (s *Store) waitForCRD(ctx context.Context) error {
	exp := &backoff.ExponentialBackOff{
		InitialInterval:     crdCheckInitialInterval,
		RandomizationFactor: 0,
		Multiplier:          crdCheckMultiplier,
		MaxInterval:         crdCheckMaxInterval,
	}
	exp.Reset()

	attempt := 0
	_, err := backoff.Retry(ctx, func() (any, error) {
		_, err := s.client.Resource(InstrumentationGVR).List(ctx, metav1.ListOptions{Limit: 1})
		if err == nil {
			return nil, nil
		}
		if apierrors.IsUnauthorized(err) || apierrors.IsForbidden(err) {
			log.Errorf("Instrumentation CRD check failed: not retryable: %s", err)
			return nil, backoff.Permanent(err)
		}
		attempt++
		if apierrors.IsNotFound(err) {
			log.Debugf("Instrumentation CRD missing (attempt=%d): will retry", attempt)
		} else {
			log.Debugf("Instrumentation CRD check failed transiently (attempt=%d): %v: will retry", attempt, err)
		}
		return nil, err
	}, backoff.WithBackOff(exp), backoff.WithMaxElapsedTime(crdCheckMaxElapsedTime))
	return err
}

func (s *Store) upsert(obj interface{}) {
	cr, err := InstrumentationFromObject(obj)
	if err != nil {
		log.Warnf("Couldn't convert OpenTelemetry Instrumentation object: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.crs[storeKey(cr.Namespace, cr.Name)] = cr
}

func (s *Store) remove(obj interface{}) {
	cr, err := InstrumentationFromObject(obj)
	if err != nil {
		log.Warnf("Couldn't convert deleted OpenTelemetry Instrumentation object: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.crs, storeKey(cr.Namespace, cr.Name))
}

func (s *Store) setSynced() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.synced = true
}

func storeKey(namespace, name string) string {
	return namespace + "/" + name
}
