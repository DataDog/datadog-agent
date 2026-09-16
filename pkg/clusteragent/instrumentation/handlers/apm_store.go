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
	targets    map[ssi.DDICRTarget]ssi.DDIAPMConfig
	targetByCR map[types.NamespacedName]ssi.DDICRTarget
}

// NewAPMTargetStore returns an empty APMTargetStore.
func NewAPMTargetStore() *APMTargetStore {
	return &APMTargetStore{
		targets:    make(map[ssi.DDICRTarget]ssi.DDIAPMConfig),
		targetByCR: make(map[types.NamespacedName]ssi.DDICRTarget),
	}
}

// UpsertTarget stores the DDI target for a workload.
func (s *APMTargetStore) UpsertTarget(workload ssi.DDICRTarget, config ssi.DDIAPMConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deleteByCRLocked(config.CR)
	s.targets[workload] = config
	s.targetByCR[config.CR] = workload
}

// DeleteByCR removes the entry sourced from the given CR name, if present.
func (s *APMTargetStore) DeleteByCR(cr types.NamespacedName) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deleteByCRLocked(cr)
}

func (s *APMTargetStore) deleteByCRLocked(cr types.NamespacedName) {
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
func (s *APMTargetStore) GetTarget(workload ssi.DDICRTarget) (ssi.DDIAPMConfig, bool) {
	if s == nil {
		return ssi.DDIAPMConfig{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	target, ok := s.targets[workload]
	if !ok {
		return ssi.DDIAPMConfig{}, false
	}
	return target, true
}
