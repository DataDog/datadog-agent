// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package crstore stores Single Step Instrumentation configuration sourced from
// DatadogInstrumentation custom resources.
package crstore

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// WorkloadKey identifies the workload a DatadogInstrumentation CR applies to.
type WorkloadKey struct {
	Kind      string
	Namespace string
	Name      string
}

// APMEntry is the per-workload APM configuration extracted from a
// DatadogInstrumentation custom resource.
type APMEntry struct {
	CR             types.NamespacedName
	Enabled        bool
	TracerVersions map[string]string
	TracerConfigs  []corev1.EnvVar
}

// Store holds APM configuration indexed by workload and CR.
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
// pointed at a different workload, the old workload entry is removed.
func (s *Store) UpsertAPM(workload WorkloadKey, entry APMEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if prev, ok := s.workloadByCR[entry.CR]; ok && prev != workload {
		delete(s.apmByWorkload, prev)
	}
	s.apmByWorkload[workload] = copyAPMEntry(entry)
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

	if existing, ok := s.apmByWorkload[workload]; ok && existing.CR == cr {
		delete(s.apmByWorkload, workload)
	}
}

// GetAPM returns the APM entry for a workload, if any.
func (s *Store) GetAPM(workload WorkloadKey) (APMEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.apmByWorkload[workload]
	if !ok {
		return APMEntry{}, false
	}
	return copyAPMEntry(entry), true
}

func copyAPMEntry(entry APMEntry) APMEntry {
	copied := entry
	if entry.TracerVersions != nil {
		copied.TracerVersions = make(map[string]string, len(entry.TracerVersions))
		for lang, version := range entry.TracerVersions {
			copied.TracerVersions[lang] = version
		}
	}
	if entry.TracerConfigs != nil {
		copied.TracerConfigs = append([]corev1.EnvVar(nil), entry.TracerConfigs...)
	}
	return copied
}
