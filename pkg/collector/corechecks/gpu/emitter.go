// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"errors"
	"fmt"
	"time"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	agenterrors "github.com/DataDog/datadog-agent/pkg/errors"
)

// SampleEmitter sends the samples collected for GPU devices: it removes
// duplicates by priority, applies the static metric cadence and rates, and
// adds device and workload tags. Each GPU check owns one, with its own
// telemetry and workload tag cache.
type SampleEmitter struct {
	WorkloadTagCache *WorkloadTagCache
	RateCalculator   *nvidia.RateCalculator
	StrictIntervals  *nvidia.StrictIntervalProcessor
	Telemetry        EmitterTelemetry
}

// EmitterTelemetry holds the telemetry a SampleEmitter reports to. Each check
// registers its own metrics, under its own subsystem.
type EmitterTelemetry struct {
	CollectionRuns   telemetry.Counter
	CollectionErrors telemetry.Counter
	CollectionTime   telemetry.Histogram
	MetricsSent      telemetry.Counter
	DuplicateMetrics telemetry.Counter
}

type deviceSamplesCollection struct {
	collectorSamples map[nvidia.CollectorName][]nvidia.Sample
	totalCount       int
}

// CollectorSamples is the result of one collection of samples for a device.
type CollectorSamples struct {
	Name          nvidia.CollectorName
	DeviceUUID    string
	TelemetryTags []string
	Samples       []nvidia.Sample
	Err           error
	Duration      time.Duration
}

// Emit sends the samples of the given collector results. deviceTags returns
// the tags of a device, and gpuToContainers the containers each device is
// allocated to, whose tags are added to samples without associated workloads.
func (e *SampleEmitter) Emit(snd sender.Sender, collectorResults []CollectorSamples, gpuToContainers map[string][]*workloadmeta.Container, deviceTags func(deviceUUID string) []string, currentExecutionTime time.Time) error {
	var multiErr []error
	perDeviceSamples := make(map[string]*deviceSamplesCollection)

	for _, collectorResult := range collectorResults {
		e.Telemetry.CollectionRuns.Inc(collectorResult.TelemetryTags...)
		e.Telemetry.CollectionTime.Observe(float64(collectorResult.Duration.Milliseconds()), collectorResult.TelemetryTags...)

		if collectorResult.Err != nil {
			e.Telemetry.CollectionErrors.Add(1, collectorResult.TelemetryTags...)
			multiErr = append(multiErr, fmt.Errorf("collector %s failed. %w", collectorResult.Name, collectorResult.Err))
		}

		if len(collectorResult.Samples) > 0 {
			deviceUUID := collectorResult.DeviceUUID
			if perDeviceSamples[deviceUUID] == nil {
				perDeviceSamples[deviceUUID] = &deviceSamplesCollection{
					collectorSamples: make(map[nvidia.CollectorName][]nvidia.Sample),
				}
			}
			perDeviceSamples[deviceUUID].collectorSamples[collectorResult.Name] = collectorResult.Samples
			perDeviceSamples[deviceUUID].totalCount += len(collectorResult.Samples)
		}

		e.Telemetry.MetricsSent.Add(float64(len(collectorResult.Samples)), string(collectorResult.Name))
	}

	// Iterate through devices to emit their samples.
	for deviceUUID, deviceData := range perDeviceSamples {
		deduplicatedSamples := nvidia.RemoveDuplicateSamples(deviceData.collectorSamples)
		e.Telemetry.DuplicateMetrics.Add(float64(deviceData.totalCount-len(deduplicatedSamples)), deviceUUID)
		deviceContainers := gpuToContainers[deviceUUID]
		tags := deviceTags(deviceUUID)

		deduplicatedSamples = e.StrictIntervals.ProcessSamples(deduplicatedSamples, currentExecutionTime, deviceUUID)
		deduplicatedSamples = e.RateCalculator.ProcessSamples(deduplicatedSamples, currentExecutionTime, deviceUUID)

		for _, sample := range deduplicatedSamples {
			if err := e.EmitSample(sample, snd, currentExecutionTime, deviceContainers, tags); err != nil {
				multiErr = append(multiErr, fmt.Errorf("error emitting sample %s: %w", sample.Key(), err))
			}
		}
	}

	return errors.Join(multiErr...)
}

// EmitSample sends a sample with the given device tags and the tags of its
// workloads, or of the device containers when it has no associated workload.
func (e *SampleEmitter) EmitSample(sample nvidia.Sample, snd sender.Sender, currentExecutionTime time.Time, deviceContainers []*workloadmeta.Container, deviceTags []string) error {
	var multiErr []error

	metricWorkloads := sample.AssociatedWorkloads()

	// Metrics with no associated workloads are assumed to apply to all workloads on the device.
	if len(metricWorkloads) == 0 {
		for _, deviceContainer := range deviceContainers {
			metricWorkloads = append(metricWorkloads, deviceContainer.EntityID)
		}
	}

	metricTags := []string{}
	for _, workloadID := range metricWorkloads {
		tags, err := e.WorkloadTagCache.GetOrCreateWorkloadTags(workloadID)
		if err != nil && !agenterrors.IsNotFound(err) { // Only report errors that are not "not found"
			multiErr = append(multiErr, fmt.Errorf("error collecting workload tags for workload %s of type %s: %w", workloadID.ID, workloadID.Kind, err))
		}

		// always continue with whatever tags we can get even if there are errors
		metricTags = append(metricTags, tags...)
	}

	sample = sample.Clone() // avoid modifying the original sample
	sample.AppendTags(metricTags)
	sample.AppendTags(deviceTags)

	err := sample.Emit(gpuMetricsNs, snd, currentExecutionTime)
	if err != nil {
		multiErr = append(multiErr, fmt.Errorf("error emitting sample: %w", err))
	}

	return errors.Join(multiErr...)
}
