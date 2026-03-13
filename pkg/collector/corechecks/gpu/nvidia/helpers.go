// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"golang.org/x/exp/constraints"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var logLimiter = log.NewLogLimit(20, 10*time.Minute)

var eccErrorTypeToName = map[nvml.MemoryErrorType]string{
	nvml.MEMORY_ERROR_TYPE_CORRECTED:   "corrected",
	nvml.MEMORY_ERROR_TYPE_UNCORRECTED: "uncorrected",
}

var memoryLocationToName = map[nvml.MemoryLocation]string{
	nvml.MEMORY_LOCATION_L1_CACHE:       "l1_cache",
	nvml.MEMORY_LOCATION_L2_CACHE:       "l2_cache",
	nvml.MEMORY_LOCATION_DEVICE_MEMORY:  "device_memory",
	nvml.MEMORY_LOCATION_REGISTER_FILE:  "register_file",
	nvml.MEMORY_LOCATION_TEXTURE_MEMORY: "texture_memory",
	nvml.MEMORY_LOCATION_TEXTURE_SHM:    "texture_shm",
	nvml.MEMORY_LOCATION_CBU:            "cbu",
	nvml.MEMORY_LOCATION_SRAM:           "sram",
}

// boolToFloat converts a boolean value to float64 (1.0 for true, 0.0 for false)
func boolToFloat(val bool) float64 {
	if val {
		return 1
	}
	return 0
}

// number interface for numeric type constraints
type number interface {
	constraints.Integer | constraints.Float
}

// readNumberFromBuffer reads a number from a binary reader and converts it to the target type
func readNumberFromBuffer[T number, V number](reader io.Reader) (V, error) {
	var value T
	err := binary.Read(reader, binary.LittleEndian, &value)
	return V(value), err
}

// fieldValueToNumber converts an NVML field value to a numeric type based on its value type
func fieldValueToNumber[V number](valueType nvml.ValueType, value [8]byte) (V, error) {
	reader := bytes.NewReader(value[:])

	switch valueType {
	case nvml.VALUE_TYPE_DOUBLE:
		return readNumberFromBuffer[float64, V](reader)
	case nvml.VALUE_TYPE_UNSIGNED_INT:
		return readNumberFromBuffer[uint32, V](reader)
	case nvml.VALUE_TYPE_UNSIGNED_LONG, nvml.VALUE_TYPE_UNSIGNED_LONG_LONG:
		return readNumberFromBuffer[uint64, V](reader)
	case nvml.VALUE_TYPE_SIGNED_LONG_LONG: // No typo, there's no SIGNED_LONG in the NVML API
		return readNumberFromBuffer[int64, V](reader)
	case nvml.VALUE_TYPE_SIGNED_INT:
		return readNumberFromBuffer[int32, V](reader)

	default:
		return 0, fmt.Errorf("unsupported value type %d", valueType)
	}
}

// filterSupportedAPIs tests each API call against the device and returns only the supported ones
func filterSupportedAPIs(device ddnvml.Device, apiCalls []apiCallInfo) []apiCallInfo {
	var supportedAPIs []apiCallInfo
	for _, apiCall := range apiCalls {
		// Test API support by calling the handler with timestamp=0 and ignoring results
		_, _, err := apiCall.Handler(device, 0)
		if err == nil || !ddnvml.IsAPIUnsupportedOnDevice(err, device) {
			supportedAPIs = append(supportedAPIs, apiCall)
		}
	}

	return supportedAPIs
}

// GetDeviceTagsMapping returns the mapping of tags per GPU device.
func GetDeviceTagsMapping(deviceCache ddnvml.DeviceCache, tagger tagger.Component) map[string][]string {
	devCount, err := deviceCache.Count()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("Error getting device count: %s", err)
		}
		return nil
	}
	if devCount == 0 {
		return nil
	}

	tagsMapping := make(map[string][]string, devCount)

	allDevices, err := deviceCache.All()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("Error getting all physical devices: %s", err)
		}
		return nil
	}
	for _, dev := range allDevices {
		uuid := dev.GetDeviceInfo().UUID
		entityID := taggertypes.NewEntityID(taggertypes.GPU, uuid)
		tags, err := tagger.Tag(entityID, taggertypes.ChecksConfigCardinality)
		if err != nil {
			log.Warnf("Error collecting GPU tags for GPU UUID %s: %s", uuid, err)
		}

		if len(tags) == 0 {
			// If we get no tags (either WMS hasn't collected GPUs yet, or we are running the check standalone with 'agent check')
			// add at least the UUID as a tag to distinguish the values.
			tags = []string{"gpu_uuid:" + uuid}
		}

		tagsMapping[uuid] = tags
	}

	return tagsMapping
}

// RemoveDuplicateMetrics filters metrics by priority across collectors while preserving all metrics within each collector.
// For each metric name, it finds the collector with the highest priority metric of that name, then includes
// ALL metrics with that name from the winning collector. This preserves multiple metrics with the same name
// but different tags (e.g., multiple memory.usage metrics with different PIDs) from the same collector,
// while still allowing cross-collector deduplication based on priority.
//
// Input: map from collector ID to slice of metrics from that collector
// Output: flat slice of metrics with duplicates removed according to the priority rules
//
// Example:
//
//	CollectorA: [
//	  {Name: "process.memory.usage", Priority: 10, Tags: ["pid:1001"]},
//	  {Name: "process.memory.usage", Priority: 10, Tags: ["pid:1002"]},
//	  {Name: "core.temp", Priority: 0}
//	]
//	CollectorB: [
//	  {Name: "process.memory.usage", Priority: 5, Tags: ["pid:1003"]},
//	  {Name: "fan.speed", Priority: 0}
//	]
//
// Result: [
//
//	{Name: "process.memory.usage", Priority: 10, Tags: ["pid:1001"]},  // From CollectorA (winner)
//	{Name: "process.memory.usage", Priority: 10, Tags: ["pid:1002"]},  // From CollectorA (winner)
//	{Name: "core.temp", Priority: 0},                          // From CollectorA (unique)
//	{Name: "fan.speed", Priority: 0}                           // From CollectorB (unique)
//
// ]
func RemoveDuplicateMetrics(allMetrics map[CollectorName][]Metric) []Metric {
	// Map metric name -> collector ID -> []Metric (with that name)
	nameToCollectorMetrics := make(map[string]map[CollectorName]map[MetricPriority][]Metric)

	for collectorID, metrics := range allMetrics {
		for _, m := range metrics {
			if _, ok := nameToCollectorMetrics[m.Name]; !ok {
				nameToCollectorMetrics[m.Name] = make(map[CollectorName]map[MetricPriority][]Metric)
			}
			if _, ok := nameToCollectorMetrics[m.Name][collectorID]; !ok {
				nameToCollectorMetrics[m.Name][collectorID] = make(map[MetricPriority][]Metric)
			}
			nameToCollectorMetrics[m.Name][collectorID][m.Priority] = append(nameToCollectorMetrics[m.Name][collectorID][m.Priority], m)
		}
	}

	var result []Metric

	// For each metric name, pick all matching metrics from the collector with the highest-priority metric of that name
	for _, collectorMetrics := range nameToCollectorMetrics {
		maxPriority := Low
		var winningMetrics []Metric
		for _, priorityMetrics := range collectorMetrics {
			for priority, metrics := range priorityMetrics {
				if priority >= maxPriority {
					maxPriority = priority
					winningMetrics = metrics
				}
			}
		}
		// Add all metrics for that name from the winning collector
		result = append(result, winningMetrics...)
	}

	return result
}
