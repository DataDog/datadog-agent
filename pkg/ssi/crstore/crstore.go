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

// WorkloadTarget identifies the workload target selected by a DatadogInstrumentation CR.
type WorkloadTarget struct {
	Kind      string
	Namespace string
	Name      string
}

// APMConfig is the per-target APM configuration extracted from a
// DatadogInstrumentation custom resource.
type APMConfig struct {
	CR             types.NamespacedName
	Enabled        bool
	TracerVersions map[string]string
	TracerConfigs  []corev1.EnvVar
}

// Store holds APM configuration indexed by workload target and CR.
type Store struct {
	mu          sync.RWMutex
	apmByTarget map[WorkloadTarget]APMConfig
	targetByCR  map[types.NamespacedName]WorkloadTarget
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		apmByTarget: make(map[WorkloadTarget]APMConfig),
		targetByCR:  make(map[types.NamespacedName]WorkloadTarget),
	}
}

// UpsertAPM stores the APM configuration for a workload target.
func (s *Store) UpsertAPM(target WorkloadTarget, config APMConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.apmByTarget[target] = config
	s.targetByCR[config.CR] = target
}

// DeleteByCR removes the entry sourced from the given CR, if present.
func (s *Store) DeleteByCR(cr types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.targetByCR[cr]
	if !ok {
		return
	}
	delete(s.targetByCR, cr)

	if existing, ok := s.apmByTarget[target]; ok && existing.CR == cr {
		delete(s.apmByTarget, target)
	}
}

// GetAPM returns the APM configuration for a workload target, if any.
func (s *Store) GetAPM(target WorkloadTarget) (APMConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config, ok := s.apmByTarget[target]
	if !ok {
		return APMConfig{}, false
	}
	return config, true
}
