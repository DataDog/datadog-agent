// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package handlers

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/ssi"
)

// APMTargetStore holds DDI apm targets indexed by the workload they target.
type APMTargetStore struct {
	mu         sync.RWMutex
	targets    map[ssi.WorkloadTarget]ssi.DDITarget
	targetByCR map[types.NamespacedName]ssi.WorkloadTarget
}

// NewAPMTargetStore returns an empty APMTargetStore.
func NewAPMTargetStore() *APMTargetStore {
	return &APMTargetStore{
		targets:    make(map[ssi.WorkloadTarget]ssi.DDITarget),
		targetByCR: make(map[types.NamespacedName]ssi.WorkloadTarget),
	}
}

// UpsertTarget stores the DDI target for a workload.
func (s *APMTargetStore) UpsertTarget(workload ssi.WorkloadTarget, target ssi.DDITarget) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.targets[workload] = target
	s.targetByCR[target.CR] = workload
}

// DeleteByCR removes the entry sourced from the given CR name, if present.
func (s *APMTargetStore) DeleteByCR(cr types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.targetByCR[cr]
	if !ok {
		return
	}
	delete(s.targetByCR, cr)

	if existing, ok := s.targets[target]; ok && existing.CR == cr {
		delete(s.targets, target)
	}
}

// GetTarget returns the DDI target for a workload, if any.
func (s *APMTargetStore) GetTarget(workload ssi.WorkloadTarget) (ssi.DDITarget, bool) {
	if s == nil {
		return ssi.DDITarget{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	target, ok := s.targets[workload]
	if !ok {
		return ssi.DDITarget{}, false
	}
	return target, true
}
