package main

import (
	"slices"
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

func (c gpuConfig) key() string {
	return c.Architecture + "|" + string(c.DeviceMode)
}

type gpuConfigValidationResult struct {
	Config          gpuConfig           `json:"config"`
	DeviceCount     int                 `json:"device_count"`
	ExpectedMetrics []string            `json:"expected_metrics"`
	PresentMetrics  []string            `json:"present_metrics"`
	MissingMetrics  []string            `json:"missing_metrics"`
	UnknownMetrics  []string            `json:"unknown_metrics"`
	TagFailures     map[string][]string `json:"tag_failures"`
	State           validationState     `json:"state"`
}

func newGPUConfigValidationResult(config gpuConfig, deviceCount int, expectedMetrics []string) gpuConfigValidationResult {
	return gpuConfigValidationResult{
		Config:          config,
		DeviceCount:     deviceCount,
		ExpectedMetrics: append([]string(nil), expectedMetrics...),
		PresentMetrics:  []string{},
		MissingMetrics:  []string{},
		UnknownMetrics:  []string{},
		TagFailures:     map[string][]string{},
		State:           validationStateUnknown,
	}
}

func (r *gpuConfigValidationResult) hasFailures() bool {
	return r.DeviceCount > 0 && (len(r.MissingMetrics)+len(r.UnknownMetrics)+len(r.TagFailures) > 0)
}

func (r *gpuConfigValidationResult) update(other gpuConfigValidationResult) {
	r.DeviceCount += other.DeviceCount
	if other.State < r.State {
		r.State = other.State
	}

	r.PresentMetrics = append(r.PresentMetrics, other.PresentMetrics...)
	r.UnknownMetrics = append(r.UnknownMetrics, other.UnknownMetrics...)
	for metricName, tags := range other.TagFailures {
		r.TagFailures[metricName] = append(r.TagFailures[metricName], tags...)
	}

	r.MissingMetrics = computeMissingMetrics(r.ExpectedMetrics, r.PresentMetrics)
}

type validationResults struct {
	Results            []gpuConfigValidationResult `json:"results"`
	MetricsCount       int                         `json:"metrics_count"`
	ArchitecturesCount int                         `json:"architectures_count"`
	FailingCount       int                         `json:"failing_count"`
}

func (r *validationResults) update(other validationResults) {
	r.MetricsCount = max(r.MetricsCount, other.MetricsCount)
	r.ArchitecturesCount = max(r.ArchitecturesCount, other.ArchitecturesCount)
	r.FailingCount += other.FailingCount

	byKey := make(map[string]*gpuConfigValidationResult, len(r.Results))
	for idx := range r.Results {
		byKey[r.Results[idx].Config.key()] = &r.Results[idx]
	}

	for _, result := range other.Results {
		if existing, found := byKey[result.Config.key()]; found {
			existing.update(result)
			continue
		}
		resultCopy := result
		r.Results = append(r.Results, resultCopy)
		byKey[resultCopy.Config.key()] = &r.Results[len(r.Results)-1]
	}

}

func determineResultState(result gpuConfigValidationResult) validationState {
	if !result.Config.IsKnown {
		return validationStateUnknown
	}
	if len(result.MissingMetrics) > 0 || len(result.UnknownMetrics) > 0 || len(result.TagFailures) > 0 {
		return validationStateFail
	}
	return validationStateOK
}

func computeMissingMetrics(expectedMetrics, presentMetrics []string) []string {
	missingMetrics := make([]string, 0, len(expectedMetrics))
	for _, expectedMetric := range expectedMetrics {
		if !slices.Contains(presentMetrics, expectedMetric) {
			missingMetrics = append(missingMetrics, expectedMetric)
		}
	}
	return missingMetrics
}
