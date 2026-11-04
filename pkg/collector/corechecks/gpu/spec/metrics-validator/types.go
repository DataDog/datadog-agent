// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

type validationState string

const (
	validationStateError   validationState = "error"
	validationStateFail    validationState = "fail"
	validationStateOK      validationState = "ok"
	validationStateMissing validationState = "missing"
)

type gpuConfigValidationResult struct {
	Config          gpuspec.GPUConfig        `json:"config"`
	DeviceCount     int                      `json:"device_count"`
	DetailedResult  gpuspec.ValidationResult `json:"detailed_result"`
	RetrievalErrors []string                 `json:"retrieval_errors,omitempty"`
	State           validationState          `json:"state"`
}

func (r *gpuConfigValidationResult) hasFailures() bool {
	return r.DeviceCount > 0 && r.DetailedResult.HasFailures()
}

func (r *gpuConfigValidationResult) hasRetrievalErrors() bool {
	return len(r.RetrievalErrors) > 0
}

type orgValidationResults struct {
	Results            []gpuConfigValidationResult `json:"results"`
	MetricsCount       int                         `json:"metrics_count"`
	ArchitecturesCount int                         `json:"architectures_count"`
}

func determineResultState(result gpuConfigValidationResult) validationState {
	if result.hasRetrievalErrors() {
		return validationStateError
	}
	if result.hasFailures() {
		return validationStateFail
	}
	return validationStateOK
}
