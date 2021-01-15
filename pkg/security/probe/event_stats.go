// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
)

// PerfMapStats contains the collected metrics for one event and one cpu in a perf buffer statistics map
type PerfMapStats struct {
	Bytes uint64
	Count uint64
	Lost  uint64
}

// UnmarshalBinary parses a map entry and populates the current PerfMapStats instance
func (s *PerfMapStats) UnmarshalBinary(data []byte) error {
	if len(data) < 24 {
		return ErrNotEnoughData
	}
	s.Bytes = ebpf.ByteOrder.Uint64(data[0:8])
	s.Count = ebpf.ByteOrder.Uint64(data[8:16])
	s.Lost = ebpf.ByteOrder.Uint64(data[16:24])
	return nil
}

// EventsStats holds statistics about the number of lost and received events
//nolint:structcheck,unused
type EventsStats struct {
	// config pointer to the config of the runtime security module
	config *config.Config
	// cpuCount holds the current count of CPU
	cpuCount int
	// perfBufferStatsMaps holds the pointers to the statistics kernel maps
	perfBufferStatsMaps map[string]*lib.Map
	// perfBufferSize holds the size of each perf buffer, indexed by the name of the perf buffer
	perfBufferSize map[string]float64

	// perfBufferMapNameToStatsMapsName maps a perf buffer to its statistics maps
	perfBufferMapNameToStatsMapsName map[string]string
	// statsMapsNamePerfBufferMapName maps a statistic map to its perf buffer
	statsMapsNameToPerfBufferMapName map[string]string

	// stats holds the collected user space metrics
	stats map[string][][maxEventType]PerfMapStats
	// kernelStats holds the aggregated kernel space metrics
	kernelStats map[string][][maxEventType]PerfMapStats
	// readLostEvents is the count of lost events, collected by reading the perf buffer
	readLostEvents map[string][]uint64
}

// NewEventsStats instantiates a new event statistics counter
func NewEventsStats(ebpfManager *manager.Manager, options manager.Options, config *config.Config) (*EventsStats, error) {
	es := EventsStats{
		config:              config,
		cpuCount:            runtime.NumCPU(),
		perfBufferStatsMaps: make(map[string]*lib.Map),
		perfBufferSize:      make(map[string]float64),

		perfBufferMapNameToStatsMapsName: probes.GetPerfBufferStatisticsMaps(),
		statsMapsNameToPerfBufferMapName: make(map[string]string),

		stats:          make(map[string][][maxEventType]PerfMapStats),
		kernelStats:    make(map[string][][maxEventType]PerfMapStats),
		readLostEvents: make(map[string][]uint64),
	}

	// compute statsMapPerfMap
	for perfMap, statsMap := range es.perfBufferMapNameToStatsMapsName {
		es.statsMapsNameToPerfBufferMapName[statsMap] = perfMap
	}

	// Select perf buffer statistics maps
	for perfMapName, statsMapName := range es.perfBufferMapNameToStatsMapsName {
		stats, ok, err := ebpfManager.GetMap(statsMapName)
		if !ok {
			return nil, errors.Errorf("map %s not found", statsMapName)
		}
		if err != nil {
			return nil, err
		}

		es.perfBufferStatsMaps[perfMapName] = stats
		// set default perf buffer size, it will be readjusted in the next loop if needed
		es.perfBufferSize[perfMapName] = float64(options.DefaultPerfRingBufferSize)
	}

	// Prepare user space counters
	for _, m := range ebpfManager.PerfMaps {
		var stats, kernelStats [][maxEventType]PerfMapStats
		var usrLostEvents []uint64

		for i := 0; i < es.cpuCount; i++ {
			stats = append(stats, [maxEventType]PerfMapStats{})
			kernelStats = append(kernelStats, [maxEventType]PerfMapStats{})
			usrLostEvents = append(usrLostEvents, 0)
		}

		es.stats[m.Name] = stats
		es.kernelStats[m.Name] = kernelStats
		es.readLostEvents[m.Name] = usrLostEvents

		// update perf buffer size if needed
		if m.PerfRingBufferSize != 0 {
			es.perfBufferSize[m.Name] = float64(m.PerfRingBufferSize)
		}
	}
	return &es, nil
}

// getLostCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getLostCount(perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&e.readLostEvents[perfMap][cpu])
}

// GetLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (e *EventsStats) GetLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range e.readLostEvents[perfMap] {
			total += e.getLostCount(perfMap, i)
		}
		break
	case cpu >= 0 && e.cpuCount > cpu:
		total += e.getLostCount(perfMap, cpu)
	}

	return total
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getKernelLostCount(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&e.kernelStats[perfMap][cpu][eventType].Lost)
}

// GetAndResetKernelLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (e *EventsStats) GetAndResetKernelLostCount(perfMap string, cpu int, evtTypes ...EventType) uint64 {
	var total uint64
	var shouldCount bool

	// query the kernel maps
	_ = e.collectAndSendKernelStats(nil)

	for cpuID := range e.kernelStats[perfMap] {
		if cpu == -1 || cpu == cpuID {
			for kernelEvtType := range e.kernelStats[perfMap][cpuID] {
				shouldCount = len(evtTypes) == 0
				if !shouldCount {
					for evtType := range evtTypes {
						if evtType == kernelEvtType {
							shouldCount = true
						}
					}
				}
				if shouldCount {
					total += e.getKernelLostCount(EventType(kernelEvtType), perfMap, cpuID)
				}
			}
		}
	}

	return total
}

// getAndResetReadLostCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getAndResetReadLostCount(perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&e.readLostEvents[perfMap][cpu], 0)
}

// GetAndResetLostCount returns the number of lost events and resets the counter for a given map and cpu. If a cpu of -1 is
// provided, the function will reset the counters of all the cpus for the provided map, and return the sum of all the
// lost events of all the cpus of the provided map.
func (e *EventsStats) GetAndResetLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range e.readLostEvents[perfMap] {
			total += e.getAndResetReadLostCount(perfMap, i)
		}
		break
	case cpu >= 0 && e.cpuCount > cpu:
		total += e.getAndResetReadLostCount(perfMap, cpu)
	}
	return total
}

// getEventCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getEventCount(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&e.stats[perfMap][cpu][eventType].Count)
}

// getEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getEventBytes(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&e.stats[perfMap][cpu][eventType].Bytes)
}

// getKernelEventCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) swapKernelEventCount(eventType EventType, perfMap string, cpu int, value uint64) uint64 {
	return atomic.SwapUint64(&e.kernelStats[perfMap][cpu][eventType].Count, value)
}

// getKernelEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) swapKernelEventBytes(eventType EventType, perfMap string, cpu int, value uint64) uint64 {
	return atomic.SwapUint64(&e.kernelStats[perfMap][cpu][eventType].Bytes, value)
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) swapKernelLostCount(eventType EventType, perfMap string, cpu int, value uint64) uint64 {
	return atomic.SwapUint64(&e.kernelStats[perfMap][cpu][eventType].Lost, value)
}

// GetEventStats returns the number of received events of the specified type and resets the counter
func (e *EventsStats) GetEventStats(eventType EventType, perfMap string, cpu int) PerfMapStats {
	var stats PerfMapStats
	var maps []string

	if eventType >= maxEventType {
		return stats
	}

	switch {
	case len(perfMap) == 0:
		for m := range e.stats {
			maps = append(maps, m)
		}
		break
	case e.stats[perfMap] != nil:
		maps = append(maps, perfMap)
	}

	for _, m := range maps {

		switch {
		case cpu == -1:
			for i := range e.stats[m] {
				stats.Count += e.getEventCount(eventType, perfMap, i)
				stats.Bytes += e.getEventBytes(eventType, perfMap, i)
			}
			break
		case cpu >= 0 && e.cpuCount > cpu:
			stats.Count += e.getEventCount(eventType, perfMap, cpu)
			stats.Bytes += e.getEventBytes(eventType, perfMap, cpu)
		}

	}
	return stats
}

// getAndResetEventCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getAndResetEventCount(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&e.stats[perfMap][cpu][eventType].Count, 0)
}

// getAndResetEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getAndResetEventBytes(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&e.stats[perfMap][cpu][eventType].Bytes, 0)
}

// CountLostEvent adds `count` to the counter of lost events
func (e *EventsStats) CountLostEvent(count uint64, m *manager.PerfMap, cpu int) {
	// sanity check
	if (e.readLostEvents[m.Name] == nil) || (len(e.readLostEvents[m.Name]) <= cpu) {
		return
	}
	atomic.AddUint64(&e.readLostEvents[m.Name][cpu], count)
}

// CountEvent adds `count` to the counter of received events of the specified type
func (e *EventsStats) CountEvent(eventType EventType, count uint64, size uint64, m *manager.PerfMap, cpu int) {
	// sanity check
	if (e.stats[m.Name] == nil) || (len(e.stats[m.Name]) <= cpu) || (len(e.stats[m.Name][cpu]) <= int(eventType)) {
		return
	}

	atomic.AddUint64(&e.stats[m.Name][cpu][eventType].Count, count)
	atomic.AddUint64(&e.stats[m.Name][cpu][eventType].Bytes, size)
}

func (e *EventsStats) sendEventsAndBytesReadStats(client *statsd.Client) error {
	var name string

	for m := range e.stats {
		for cpu := range e.stats[m] {
			for eventType := range e.stats[m][cpu] {
				evtType := EventType(eventType)
				tags := []string{
					fmt.Sprintf("map:%s", m),
					fmt.Sprintf("cpu:%d", cpu),
					fmt.Sprintf("event_type:%s", evtType),
				}

				name = MetricPrefix + ".perf_buffer.events.read"
				if err := client.Count(name, int64(e.getAndResetEventCount(evtType, m, cpu)), tags, 1.0); err != nil {
					return err
				}

				name = MetricPrefix + ".perf_buffer.bytes.read"
				if err := client.Count(name, int64(e.getAndResetEventBytes(evtType, m, cpu)), tags, 1.0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *EventsStats) sendLostEventsReadStats(client *statsd.Client) error {
	name := MetricPrefix + ".perf_buffer.lost_events.read"
	for m := range e.readLostEvents {
		for cpu := range e.readLostEvents[m] {
			tags := []string{
				fmt.Sprintf("map:%s", m),
				fmt.Sprintf("cpu:%d", cpu),
			}
			if err := client.Count(name, int64(e.getAndResetReadLostCount(m, cpu)), tags, 1.0); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *EventsStats) collectAndSendKernelStats(client *statsd.Client) error {
	var (
		id       uint32
		iterator *lib.MapIterator
		tags     []string
		tmpCount uint64
	)
	cpuStats := make([]PerfMapStats, runtime.NumCPU())

	// loop through the statistics buffers of each perf map
	for perfMapName, statsMap := range e.perfBufferStatsMaps {
		// loop through all the values of the active buffer
		iterator = statsMap.Iterate()
		for iterator.Next(&id, &cpuStats) {
			// retrieve event type from key
			evtType := EventType(id % uint32(maxEventType))

			// loop over each cpu entry
			for cpu, stats := range cpuStats {
				// sanity checks:
				//   - check if the computed cpu id is below the current cpu count
				//   - check if we collect some data on the provided perf map
				//   - check if the computed event id is below the current max event id
				if (e.stats[perfMapName] == nil) || (len(e.stats[perfMapName]) <= cpu) || (len(e.stats[perfMapName][cpu]) <= int(evtType)) {
					return nil
				}

				// prepare metrics tags
				tags = []string{
					fmt.Sprintf("map:%s", perfMapName),
					fmt.Sprintf("cpu:%d", cpu),
					fmt.Sprintf("event_type:%s", evtType),
				}

				// Update stats to avoid sending twice the same data points
				if tmpCount = e.swapKernelEventBytes(evtType, perfMapName, cpu, stats.Bytes); tmpCount <= stats.Bytes {
					stats.Bytes -= tmpCount
				}
				if tmpCount = e.swapKernelEventCount(evtType, perfMapName, cpu, stats.Count); tmpCount <= stats.Count {
					stats.Count -= tmpCount
				}
				if tmpCount = e.swapKernelLostCount(evtType, perfMapName, cpu, stats.Lost); tmpCount <= stats.Lost {
					stats.Lost -= tmpCount
				}

				if client != nil {
					if err := e.sendKernelStats(client, stats, tags); err != nil {
						return err
					}
				}
			}

		}
		if iterator.Err() != nil {
			return errors.Wrapf(iterator.Err(), "failed to dump the statistics buffer of map %s", perfMapName)
		}
	}
	return nil
}

func (e *EventsStats) sendKernelStats(client *statsd.Client, stats PerfMapStats, tags []string) error {
	metric := MetricPrefix + ".perf_buffer.events.write"
	if err := client.Count(metric, int64(stats.Count), tags, 1.0); err != nil {
		return err
	}

	metric = MetricPrefix + ".perf_buffer.bytes.write"
	if err := client.Count(metric, int64(stats.Bytes), tags, 1.0); err != nil {
		return err
	}

	metric = MetricPrefix + ".perf_buffer.lost_events.write"
	if err := client.Count(metric, int64(stats.Lost), tags, 1.0); err != nil {
		return err
	}
	return nil
}

// SendStats send event stats using the provided statsd client
func (e *EventsStats) SendStats(client *statsd.Client) error {
	if err := e.collectAndSendKernelStats(client); err != nil {
		return err
	}

	if err := e.sendEventsAndBytesReadStats(client); err != nil {
		return err
	}

	if err := e.sendLostEventsReadStats(client); err != nil {
		return err
	}
	return nil
}
