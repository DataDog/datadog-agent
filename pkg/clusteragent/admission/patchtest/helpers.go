// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package patchtest adapts typed fixtures to real patch planners.
package patchtest

import (
	"encoding/json"
	"errors"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
)

// Run adapts existing typed fixtures without weakening their behavior
// assertions. It runs the real planner once and applies its journal to the input.
// This helper must not be used by admission handlers.
func Run(pod *corev1.Pod, ns string, dc dynamic.Interface, planner func(*patch.PodSession, string, dynamic.Interface) (bool, error)) (bool, error) {
	if pod == nil {
		return false, errors.New(metrics.InvalidInput)
	}
	raw, err := json.Marshal(pod)
	if err != nil {
		return false, err
	}
	s, err := patch.NewPodSession(raw)
	if err != nil {
		return false, err
	}
	injected, err := planner(s, ns, dc)
	if err != nil {
		return injected, err
	}
	// A no-op must preserve even the fixture's nil versus empty representation.
	if !s.HasOperations() && s.Err() == nil {
		return injected, nil
	}
	wire, err := s.JSONPatch()
	if err != nil {
		return false, err
	}
	p, err := jsonpatch.DecodePatch(wire)
	if err != nil {
		return false, err
	}
	options := jsonpatch.NewApplyOptions()
	options.SupportNegativeIndices = false
	options.AllowMissingPathOnRemove = false
	options.EnsurePathExistsOnAdd = false
	out, err := p.ApplyWithOptions(raw, options)
	if err != nil {
		return false, err
	}
	var result corev1.Pod
	if err := json.Unmarshal(out, &result); err != nil {
		return false, err
	}
	// An empty initContainers fixture is omitted by the Kubernetes serializer.
	// Preserve that fixture's in-memory empty slice for existing typed assertions.
	if result.Spec.InitContainers == nil && pod.Spec.InitContainers != nil && len(pod.Spec.InitContainers) == 0 {
		result.Spec.InitContainers = []corev1.Container{}
	}
	*pod = result
	return injected, nil
}
