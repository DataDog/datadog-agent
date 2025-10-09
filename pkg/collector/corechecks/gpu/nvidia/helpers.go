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
	"strconv"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"golang.org/x/exp/constraints"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	secutils "github.com/DataDog/datadog-agent/pkg/security/utils"
)

var logLimiter = log.NewLogLimit(20, 10*time.Minute)

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
		if err == nil || !ddnvml.IsUnsupported(err) {
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

	allPhysicalDevices, err := deviceCache.AllPhysicalDevices()
	if err != nil {
		if logLimiter.ShouldLog() {
			log.Warnf("Error getting all physical devices: %s", err)
		}
		return nil
	}
	for _, dev := range allPhysicalDevices {
		uuid := dev.GetDeviceInfo().UUID
		entityID := taggertypes.NewEntityID(taggertypes.GPU, uuid)
		tags, err := tagger.Tag(entityID, taggertypes.ChecksConfigCardinality)
		if err != nil {
			log.Warnf("Error collecting GPU tags for GPU UUID %s: %s", uuid, err)
		}

		if len(tags) == 0 {
			// If we get no tags (either WMS hasn't collected GPUs yet, or we are running the check standalone with 'agent check')
			// add at least the UUID as a tag to distinguish the values.
			tags = []string{fmt.Sprintf("gpu_uuid:%s", uuid)}
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
	nameToCollectorMetrics := make(map[string]map[CollectorName][]Metric)

	for collectorID, metrics := range allMetrics {
		for _, m := range metrics {
			if _, ok := nameToCollectorMetrics[m.Name]; !ok {
				nameToCollectorMetrics[m.Name] = make(map[CollectorName][]Metric)
			}
			nameToCollectorMetrics[m.Name][collectorID] = append(nameToCollectorMetrics[m.Name][collectorID], m)
		}
	}

	var result []Metric

	// For each metric name, pick all matching metrics from the collector with the highest-priority metric of that name
	for _, collectorMetrics := range nameToCollectorMetrics {
		maxPriority := Low
		var winningCollectorID CollectorName
		for collectorID, metrics := range collectorMetrics {
			for _, m := range metrics {
				if m.Priority >= maxPriority {
					maxPriority = m.Priority
					winningCollectorID = collectorID
				}
			}
		}
		// Add all metrics for that name from the winning collector
		result = append(result, collectorMetrics[winningCollectorID]...)
	}

	return result
}

type nsPidCacheEntry struct {
	nspid uint32
	valid bool
}

// NsPidCache obtains the pid relative to the innermost namespace of a process given its host pid.
type NsPidCache struct {
	pidToNsPid map[uint32]*nsPidCacheEntry
}

// Invalidate invalidates all cache entries. They might still be used through GetNsPid
// by using the useInvalidCacheOnProcfsFail flag.
func (c *NsPidCache) Invalidate() {
	for _, entry := range c.pidToNsPid {
		entry.valid = false
	}
}

// GetNsPid returns the innermost namespaced pid for the process with the given host pid.
// Returns a cached value is available and valid. Otherwise, it attempts reading from procs
// and store the result in cache. If that fails, and useInvalidCacheOnProcfsFail is true,
// an invalidated cached value is stil returned if available. Otherwise, returns an error.
func (c *NsPidCache) GetNsPid(hostPid uint32, useInvalidCacheOnProcfsFail bool) (uint32, error) {
	if c.pidToNsPid == nil {
		c.pidToNsPid = map[uint32]*nsPidCacheEntry{}
	}

	entry, ok := c.pidToNsPid[hostPid]
	if ok && entry.valid {
		return entry.nspid, nil
	}

	nsPid, err := c.readProcFs(hostPid)
	if err == nil {
		c.pidToNsPid[hostPid] = &nsPidCacheEntry{nspid: nsPid, valid: true}
		return nsPid, nil
	}

	if ok && !entry.valid && useInvalidCacheOnProcfsFail {
		return entry.nspid, nil
	}

	return 0, err
}

// GetNsPidOrHostPid is the same as GetNsPid, but returns the host pid in case of error.
// This makes it impossible to determine whether or not the process really runs in the host,
// however since this is used mostly for tags and allow us to always have non-empty values.
func (c *NsPidCache) GetNsPidOrHostPid(hostPid uint32, useInvalidCacheOnProcfsFail bool) uint32 {
	nsPid, err := c.GetNsPid(hostPid, useInvalidCacheOnProcfsFail)
	if err == nil {
		return nsPid
	}

	if logLimiter.ShouldLog() {
		log.Debugf("failed getting nspid for %d, fallback to host pid: %v", hostPid, err)
	}
	return hostPid

}

// note: given /proc/X/task/Y/status, we have no guarantee that tasks Y will all
// have the same NSpid values, specially in case of unusual pid namespace setups.
// As such, we attempt reading the nspid for only on the main thread (group leader)
// in /proc/X/task/X/status, or fail otherwise
func (c *NsPidCache) readProcFs(hostPid uint32) (nsPid uint32, err error) {
	nspids, err := secutils.GetNsPids(hostPid, strconv.FormatUint(uint64(hostPid), 10))
	if err != nil {
		return 0, fmt.Errorf("failed reading nspids for host pid %d: %w", hostPid, err)
	}
	if len(nspids) == 0 {
		return 0, fmt.Errorf("found no nspids for host pid %d", hostPid)
	}

	// we look only at the last one, as it the most inner one and corresponding to its /proc/pid/ns/pid
	return nspids[len(nspids)-1], nil
}
