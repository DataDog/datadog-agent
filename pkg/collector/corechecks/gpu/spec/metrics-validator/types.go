package main

import (
	"strings"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

type validationState string

const (
	validationStateFail    validationState = "fail"
	validationStateOK      validationState = "ok"
	validationStateUnknown validationState = "unknown"
	validationStateMissing validationState = "missing"
)

type gpuConfig struct {
	Architecture string             `json:"architecture"`
	DeviceMode   gpuspec.DeviceMode `json:"device_mode"`
	IsKnown      bool               `json:"is_known"`
}

func (c gpuConfig) tagFilter() string {
	parts := []string{"gpu_architecture:" + c.Architecture}
	switch c.DeviceMode {
	case gpuspec.DeviceModeMIG:
		parts = append(parts, "gpu_slicing_mode:mig")
	case gpuspec.DeviceModeVGPU:
		parts = append(parts, "gpu_virtualization_mode:vgpu")
	default:
		parts = append(parts, "gpu_virtualization_mode:passthrough")
	}
	return strings.Join(parts, ",")
}

func (c gpuConfig) filterExpression() string {
	parts := []string{"gpu_architecture:" + c.Architecture}
	switch c.DeviceMode {
	case gpuspec.DeviceModeMIG:
		parts = append(parts, "gpu_slicing_mode:mig")
	case gpuspec.DeviceModeVGPU:
		parts = append(parts, "gpu_virtualization_mode:vgpu")
	default:
		parts = append(parts, "gpu_virtualization_mode:passthrough")
	}
	return strings.Join(parts, " AND ")
}

func (c gpuConfig) equals(other gpuConfig) bool {
	return c.Architecture == other.Architecture && c.DeviceMode == other.DeviceMode
}

type gpuConfigValidationResult struct {
	Config          gpuConfig                `json:"config"`
	DeviceCount     int                      `json:"device_count"`
	DetailedResult  gpuspec.ValidationResult `json:"detailed_result"`
	PresentMetrics  int                      `json:"present_metrics"`
	ExpectedMetrics int                      `json:"expected_metrics"`
	MissingMetrics  int                      `json:"missing_metrics"`
	UnknownMetrics  int                      `json:"unknown_metrics"`
	TagFailures     int                      `json:"tag_failures"`
	State           validationState          `json:"state"`
}

func (r *gpuConfigValidationResult) hasFailures() bool {
	return r.DeviceCount > 0 && (r.MissingMetrics+r.UnknownMetrics+r.TagFailures > 0)
}

type orgValidationResults struct {
	Results            []gpuConfigValidationResult `json:"results"`
	MetricsCount       int                         `json:"metrics_count"`
	ArchitecturesCount int                         `json:"architectures_count"`
	FailingCount       int                         `json:"failing_count"`
}

func determineResultState(result gpuConfigValidationResult) validationState {
	if !result.Config.IsKnown {
		return validationStateUnknown
	}
	if result.hasFailures() {
		return validationStateFail
	}
	return validationStateOK
}
