// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package crstore provides an in-memory store of Single Step Instrumentation
// configuration sourced from DatadogInstrumentation custom resources. It is the
// shared state between the DatadogInstrumentation controller (writer) and the
// auto-instrumentation admission webhook (reader). A single Store is
// constructed at cluster-agent startup and passed to both consumers via
// explicit dependency injection.
package crstore

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// WorkloadKey identifies the workload (Deployment, StatefulSet, DaemonSet) a
// DatadogInstrumentation CR applies to.
type WorkloadKey struct {
	Kind      string
	Namespace string
	Name      string
}

// APMEntry is the per-workload APM configuration extracted from a
// DatadogInstrumentation custom resource.
type APMEntry struct {
	// CR is the source CR for this entry, used to clean up by CR identity
	// when the CR is deleted.
	CR             types.NamespacedName
	Enabled        bool
	TracerVersions map[string]string
	TracerConfigs  []corev1.EnvVar
}

// Store holds APM configuration indexed by workload and by CR.
type Store struct {
	mu            sync.RWMutex
	apmByWorkload map[WorkloadKey]APMEntry
	workloadByCR  map[types.NamespacedName]WorkloadKey
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		apmByWorkload: make(map[WorkloadKey]APMEntry),
		workloadByCR:  make(map[types.NamespacedName]WorkloadKey),
	}
}

// UpsertAPM stores the APM configuration for a workload. If the CR previously
// pointed at a different workload, the old entry is removed.
func (s *Store) UpsertAPM(workload WorkloadKey, entry APMEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if prev, ok := s.workloadByCR[entry.CR]; ok && prev != workload {
		delete(s.apmByWorkload, prev)
	}
	s.apmByWorkload[workload] = entry
	s.workloadByCR[entry.CR] = workload
}

// DeleteByCR removes the entry sourced from the given CR, if present.
func (s *Store) DeleteByCR(cr types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()

	workload, ok := s.workloadByCR[cr]
	if !ok {
		return
	}
	delete(s.workloadByCR, cr)

	// Only remove the workload entry if it still belongs to this CR. Another
	// CR may have taken over the workload before the delete is processed.
	if existing, ok := s.apmByWorkload[workload]; ok && existing.CR == cr {
		delete(s.apmByWorkload, workload)
	}
}

// GetAPM returns the APM entry for a workload, if any.
func (s *Store) GetAPM(workload WorkloadKey) (APMEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.apmByWorkload[workload]
	return entry, ok
}
