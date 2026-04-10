package main

import (
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

type validationState string

const (
	validationStateFail    validationState = "fail"
	validationStateOK      validationState = "ok"
	validationStateMissing validationState = "missing"
)

type gpuConfigValidationResult struct {
	Config          gpuspec.GPUConfig        `json:"config"`
	DeviceCount     int                      `json:"device_count"`
	DetailedResult  gpuspec.ValidationResult `json:"detailed_result"`
	State           validationState          `json:"state"`
}

func (r *gpuConfigValidationResult) hasFailures() bool {
	return r.DeviceCount > 0 && r.DetailedResult.HasFailures()
}

type orgValidationResults struct {
	Results            []gpuConfigValidationResult `json:"results"`
	MetricsCount       int                         `json:"metrics_count"`
	ArchitecturesCount int                         `json:"architectures_count"`
}

func determineResultState(result gpuConfigValidationResult) validationState {
	if result.hasFailures() {
		return validationStateFail
	}
	return validationStateOK
}
