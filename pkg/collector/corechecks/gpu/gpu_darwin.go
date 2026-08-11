// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && !ios && cgo

// Package gpu contains the macOS GPU core-check implementation.
package gpu

import (
	"crypto/sha1" // #nosec G505 -- SHA-1 is used only to derive a non-secret UUIDv5-style identifier.
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const gpuMetricsNamespace = "gpu."

var (
	collectAGXDevices     = readAGXDevices
	darwinDiagnosticLimit = log.NewLogLimit(3, 10*time.Minute)
	tagSeparator          = regexp.MustCompile(`[^a-z0-9_.-]+`)
)

type darwinCheckTelemetry struct {
	collectionResults  telemetry.Counter
	deviceCount        telemetry.Gauge
	propertyReadErrors telemetry.Counter
}

// Check represents the macOS GPU check.
type Check struct {
	core.CheckBase
	telemetry *darwinCheckTelemetry
}

// Factory creates a new check factory. The tagger and workloadmeta dependencies
// remain in the cross-platform signature, but macOS device-level metrics do not
// currently perform workload attribution.
func Factory(_ tagger.Component, tm telemetry.Component, _ workloadmeta.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tm)
	})
}

func newCheck(tm telemetry.Component) check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		telemetry: &darwinCheckTelemetry{
			collectionResults:  tm.NewCounter(CheckName, "darwin_collection_results", []string{"status"}, "Number of macOS GPU collections by result"),
			deviceCount:        tm.NewGauge(CheckName, "device_total", nil, "Number of GPU devices"),
			propertyReadErrors: tm.NewCounter(CheckName, "darwin_property_read_errors", nil, "Number of AGX devices whose IOKit properties could not be read"),
		},
	}
}

// NewCheck creates a new GPU check instance. This is exported for integration testing.
func NewCheck(_ tagger.Component, tm telemetry.Component, _ workloadmeta.Component) check.Check {
	return newCheck(tm)
}

// Configure parses the check configuration.
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string, provider string) error {
	if !pkgconfigsetup.Datadog().GetBool("gpu.enabled") {
		return errors.New("GPU check is disabled")
	}
	return c.CommonConfigure(senderManager, initConfig, config, source, provider)
}

// Interval returns the configured scheduling interval.
func (c *Check) Interval() time.Duration {
	if interval := pkgconfigsetup.Datadog().GetInt("gpu.collection_interval_override"); interval > 0 {
		return time.Duration(interval) * time.Second
	}
	return c.CheckBase.Interval()
}

// Run collects device-level Apple GPU metrics from IOKit.
func (c *Check) Run() error {
	snd, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}
	defer snd.Commit()

	collection, err := collectAGXDevices()
	if err != nil {
		c.telemetry.collectionResults.Inc("error")
		c.telemetry.deviceCount.Set(0)
		return err
	}
	c.telemetry.collectionResults.Inc("success")
	c.telemetry.deviceCount.Set(float64(len(collection.devices)))
	if collection.propertyReadErrors > 0 {
		c.telemetry.propertyReadErrors.Add(float64(collection.propertyReadErrors))
		if darwinDiagnosticLimit.ShouldLog() {
			log.Warnf("failed to read IOKit properties for %d Apple GPU device(s)", collection.propertyReadErrors)
		}
	}
	if collection.truncated && darwinDiagnosticLimit.ShouldLog() {
		log.Warnf("Apple GPU enumeration exceeded the limit of %d devices; extra devices were ignored", maxAGXDevices)
	}
	if len(collection.devices) == 0 && darwinDiagnosticLimit.ShouldLog() {
		log.Debug("no Apple AGX GPU devices were found")
	}

	timestamp := float64(time.Now().UnixNano()) / float64(time.Second)
	var metricErrors []error
	if err := snd.GaugeWithTimestamp(
		gpuMetricsNamespace+"apple.device.count",
		float64(len(collection.devices)),
		"",
		[]string{"gpu_vendor:apple", "gpu_host:true"},
		timestamp,
	); err != nil {
		metricErrors = append(metricErrors, fmt.Errorf("send apple.device.count: %w", err))
	}
	for index, device := range collection.devices {
		tags := appleGPUDeviceTags(device, index)
		for _, metric := range appleGPUMetrics(device) {
			if err := snd.GaugeWithTimestamp(gpuMetricsNamespace+metric.name, metric.value, "", tags, timestamp); err != nil {
				metricErrors = append(metricErrors, fmt.Errorf("send %s: %w", metric.name, err))
			}
		}
		if !device.hasUtilization && darwinDiagnosticLimit.ShouldLog() {
			log.Debugf("Apple GPU %d does not expose the IOKit Device Utilization %% property", index)
		}
	}

	return errors.Join(metricErrors...)
}

type appleGPUMetric struct {
	name  string
	value float64
}

func appleGPUMetrics(device agxDeviceSnapshot) []appleGPUMetric {
	metrics := []appleGPUMetric{}
	if device.hasCoreCount && device.coreCount > 0 {
		metrics = append(metrics, appleGPUMetric{name: "apple.core.count", value: float64(device.coreCount)})
	}
	if device.hasUtilization && !math.IsNaN(device.utilization) && !math.IsInf(device.utilization, 0) && device.utilization >= 0 && device.utilization <= 100 {
		metrics = append(metrics, appleGPUMetric{name: "apple.device.utilization", value: device.utilization})
	}
	// These are system-memory counters reported by the Apple GPU driver. They are
	// not discrete VRAM, total unified memory, or memory unavailable to the CPU.
	if device.hasAllocatedSystemMemory && device.allocatedSystemMemory >= 0 {
		metrics = append(metrics, appleGPUMetric{name: "apple.system_memory.allocated", value: float64(device.allocatedSystemMemory)})
	}
	if device.hasInUseSystemMemory && device.inUseSystemMemory >= 0 {
		metrics = append(metrics, appleGPUMetric{name: "apple.system_memory.in_use", value: float64(device.inUseSystemMemory)})
	}
	return metrics
}

func appleGPUDeviceTags(device agxDeviceSnapshot, index int) []string {
	model := strings.TrimSpace(device.model)
	if model == "" {
		model = "Apple GPU"
	}
	normalizedModel := normalizeAppleGPUTag(model)
	gpuType := strings.TrimPrefix(normalizedModel, "apple_")
	if gpuType == "" || gpuType == "gpu" {
		gpuType = "apple_silicon"
	}

	return []string{
		"gpu_uuid:" + syntheticAppleGPUUUID(model, index),
		"gpu_device:" + normalizedModel,
		"gpu_vendor:apple",
		"gpu_architecture:apple_silicon",
		"gpu_type:" + gpuType,
		"gpu_virtualization_mode:none",
		"gpu_slicing_mode:none",
		"gpu_host:true",
	}
}

func normalizeAppleGPUTag(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = tagSeparator.ReplaceAllString(normalized, "_")
	normalized = strings.Trim(normalized, "_.-")
	if normalized == "" {
		return "apple_gpu"
	}
	return normalized
}

// syntheticAppleGPUUUID returns a deterministic UUID-shaped identifier from
// non-sensitive device attributes. GPU identifiers are scoped by the host in
// Datadog, so model plus logical index is sufficient and remains stable across
// reboots without collecting a platform serial number.
func syntheticAppleGPUUUID(model string, index int) string {
	const namespace = "datadog-agent/macos-gpu/v1"
	hash := sha1.Sum([]byte(fmt.Sprintf("%s\x00%s\x00%d", namespace, model, index)))
	hash[6] = (hash[6] & 0x0f) | 0x50 // UUID version 5.
	hash[8] = (hash[8] & 0x3f) | 0x80 // RFC 4122 variant.
	return fmt.Sprintf("gpu-%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}
