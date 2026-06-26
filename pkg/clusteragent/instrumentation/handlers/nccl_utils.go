// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// NCCLProfilerTarget identifies the workload a DatadogInstrumentation ncclProfiler
// section applies to.
type NCCLProfilerTarget struct {
	Kind      string
	Namespace string
	Name      string
}

// NCCLProfilerConfig is the per-target NCCL profiler configuration extracted from a
// DatadogInstrumentation custom resource.
type NCCLProfilerConfig struct {
	// CR is the source DatadogInstrumentation, used to scope deletes.
	CR types.NamespacedName
	// Enabled toggles injection for the target's pods.
	Enabled bool
	// InjectorImage optionally overrides admission_controller.nccl_profiler.injector_image.
	InjectorImage string
	// Env optionally overrides/adds NCCL env vars on injected pods.
	Env []corev1.EnvVar
}

// NCCLProfilerStore holds NCCL profiler enablement keyed by workload target and source
// CR. The NCCL handler writes it; the ncclprofiler webhook reads it (via Get) to decide
// injection. Mirrors CheckStore (autodiscovery_utils.go); safe for concurrent use.
type NCCLProfilerStore struct {
	mu         sync.RWMutex
	byTarget   map[NCCLProfilerTarget]NCCLProfilerConfig
	targetByCR map[types.NamespacedName]NCCLProfilerTarget
}

// NewNCCLProfilerStore creates a new NCCLProfilerStore.
func NewNCCLProfilerStore() *NCCLProfilerStore {
	return &NCCLProfilerStore{
		byTarget:   make(map[NCCLProfilerTarget]NCCLProfilerConfig),
		targetByCR: make(map[types.NamespacedName]NCCLProfilerTarget),
	}
}

// Upsert stores the NCCL profiler configuration for a workload target.
func (s *NCCLProfilerStore) Upsert(target NCCLProfilerTarget, config NCCLProfilerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byTarget[target] = config
	s.targetByCR[config.CR] = target
}

// Get returns the NCCL profiler configuration for a workload target, if any.
// Called by the ncclprofiler admission webhook to decide injection.
func (s *NCCLProfilerStore) Get(target NCCLProfilerTarget) (NCCLProfilerConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.byTarget[target]
	return c, ok
}

// DeleteByCR removes the entry sourced from the given CR, if present.
func (s *NCCLProfilerStore) DeleteByCR(cr types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target, ok := s.targetByCR[cr]
	if !ok {
		return
	}
	delete(s.targetByCR, cr)
	if existing, ok := s.byTarget[target]; ok && existing.CR == cr {
		delete(s.byTarget, target)
	}
}
