// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"context"
	"sync/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	csiAPMEnabledAnnotation = "csi.datadoghq.com/apm-enabled"

	// CSIDriver is a cluster-scoped resource in the storage.k8s.io API group.
	// We look it up via workloadmeta's generic KubernetesMetadata store.
	csiDriverGVRGroup    = "storage.k8s.io"
	csiDriverGVRResource = "csidrivers"

	csiWatcherSubscriberName = "admission-csi-driver-watcher"
)

// CSIDriverWatcher exposes the cached state of the Datadog CSI driver to the
// admission controller hot path.
//
// Implementations must be safe to call from multiple goroutines without
// additional synchronisation: the admission webhook calls these methods once
// per pod mutation, so the read path must be lock-free.
type CSIDriverWatcher interface {
	// IsRegistered returns true when the Datadog CSI driver object exists in
	// the cluster, regardless of whether APM SSI support is advertised.
	IsRegistered() bool
	// IsAPMEnabled returns true when the Datadog CSI driver is registered
	// in the cluster and has explicitly advertised support for APM SSI
	// volumes via the csi.datadoghq.com/apm-enabled="true" annotation.
	IsAPMEnabled() bool
}

// csiDriverState is the snapshot stored atomically inside csiDriverWatcher.
// It is intentionally a value type so that an atomic.Pointer swap publishes
// a consistent view to readers.
type csiDriverState struct {
	registered bool
	apmEnabled bool
}

// csiDriverWatcher subscribes to workloadmeta events for the Datadog CSI
// driver and keeps an in-memory snapshot updated asynchronously, so that
// callers on the admission hot path can read the current state with a
// single atomic load.
type csiDriverWatcher struct {
	state atomic.Pointer[csiDriverState]
}

// NewCSIDriverWatcher starts a workloadmeta subscription that tracks the
// Datadog CSI driver and returns a watcher exposing the cached state.
//
// The subscription is bound to ctx: when ctx is cancelled, the underlying
// goroutine drains its event channel and exits. The watcher remains usable
// after the goroutine has stopped (subsequent IsAPMEnabled calls keep
// returning the last observed state).
//
// A nil wmeta is treated as "no information available" — IsAPMEnabled will
// always return false in that case, which falls back to the init-container
// provider.
func NewCSIDriverWatcher(ctx context.Context, wmeta workloadmeta.Component) CSIDriverWatcher {
	w := &csiDriverWatcher{}
	w.state.Store(&csiDriverState{})

	if wmeta == nil {
		log.Warnf("library injection CSI driver watcher: workloadmeta is nil, CSI auto-detection will be disabled")
		return w
	}

	go w.run(ctx, wmeta)
	return w
}

// IsRegistered implements CSIDriverWatcher.
func (w *csiDriverWatcher) IsRegistered() bool {
	return w.state.Load().registered
}

// IsAPMEnabled implements CSIDriverWatcher.
func (w *csiDriverWatcher) IsAPMEnabled() bool {
	s := w.state.Load()
	return s.registered && s.apmEnabled
}

// run subscribes to workloadmeta events for the Datadog CSI driver and
// updates the watcher's state on every received bundle until ctx is done.
func (w *csiDriverWatcher) run(ctx context.Context, wmeta workloadmeta.Component) {
	filter := workloadmeta.NewFilterBuilder().
		AddKindWithEntityFilter(workloadmeta.KindKubernetesMetadata, isDatadogCSIDriverEntity).
		Build()

	ch := wmeta.Subscribe(csiWatcherSubscriberName, workloadmeta.NormalPriority, filter)
	defer wmeta.Unsubscribe(ch)

	for {
		select {
		case <-ctx.Done():
			log.Debugf("library injection CSI driver watcher: stopping (%v)", ctx.Err())
			return
		case bundle, ok := <-ch:
			if !ok {
				log.Debugf("library injection CSI driver watcher: subscription channel closed")
				return
			}
			bundle.Acknowledge()
			for _, event := range bundle.Events {
				w.apply(event)
			}
		}
	}
}

// apply updates the watcher's cached state based on a single workloadmeta
// event for the Datadog CSI driver.
func (w *csiDriverWatcher) apply(event workloadmeta.Event) {
	driver, ok := event.Entity.(*workloadmeta.KubernetesMetadata)
	if !ok {
		return
	}

	switch event.Type {
	case workloadmeta.EventTypeSet:
		apmEnabled := driver.Annotations[csiAPMEnabledAnnotation] == "true"
		w.state.Store(&csiDriverState{registered: true, apmEnabled: apmEnabled})
		if apmEnabled {
			log.Debugf("library injection CSI driver watcher: Datadog CSI driver %q is registered with APM enabled", csiDriverName)
		} else {
			log.Debugf("library injection CSI driver watcher: Datadog CSI driver %q is registered but %s != true, CSI auto-selection will fall back to init container",
				csiDriverName, csiAPMEnabledAnnotation)
		}
	case workloadmeta.EventTypeUnset:
		w.state.Store(&csiDriverState{})
		log.Debugf("library injection CSI driver watcher: Datadog CSI driver %q is no longer registered, CSI auto-selection will fall back to init container", csiDriverName)
	}
}

// isDatadogCSIDriverEntity is the entity filter used by the workloadmeta
// subscription: it keeps only the KubernetesMetadata entry for the Datadog
// CSIDriver object, ignoring every other GVR collected by the generic
// metadata informer.
func isDatadogCSIDriverEntity(entity workloadmeta.Entity) bool {
	md, ok := entity.(*workloadmeta.KubernetesMetadata)
	if !ok || md.GVR == nil {
		return false
	}
	return md.GVR.Group == csiDriverGVRGroup &&
		md.GVR.Resource == csiDriverGVRResource &&
		md.Name == csiDriverName
}
