// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"fmt"
	"math"
	"runtime"
	"sync/atomic"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
)

// PerfMapStats contains the collected metrics for one event and one cpu in a perf buffer statistics map
type PerfMapStats struct {
	Bytes uint64
	Count uint64
	Lost  uint64

	Usage   int64
	InQueue int64
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

// MarshalBinary encodes the relevant fields of the current PerfMapStats instance into a byte array
func (s *PerfMapStats) MarshalBinary() ([]byte, error) {
	b := make([]byte, 24)
	ebpf.ByteOrder.PutUint64(b, s.Bytes)
	ebpf.ByteOrder.PutUint64(b, s.Count)
	ebpf.ByteOrder.PutUint64(b, s.Lost)
	return b, nil
}

// EventsStats holds statistics about the number of lost and received events
//nolint:structcheck,unused
type EventsStats struct {
	// config pointer to the config of the runtime security module
	config *config.Config
	// cpuCount holds the current count of CPU
	cpuCount int
	// perfBufferStatsMaps holds the pointers to the statistics kernel maps
	perfBufferStatsMaps map[string][2]*lib.Map
	// perfBufferSize holds the size of each perf buffer, indexed by the name of the perf buffer
	perfBufferSize map[string]float64

	// perfBufferMapNameToStatsMapsName maps a perf buffer to its statistics maps
	perfBufferMapNameToStatsMapsName map[string][2]string
	// statsMapsNamePerfBufferMapName maps a statistic map to its perf buffer
	statsMapsNameToPerfBufferMapName map[string]string

	// bufferSelector is the kernel map used to select the active buffer ID
	bufferSelector *lib.Map
	// activeMapIndex is the index of the statistic maps we are currently collecting data from
	activeMapIndex uint32

	// stats holds the collected metrics
	stats map[string][][maxEventType]PerfMapStats
	// readLostEvents is the count of lost events, collected by reading the perf buffer
	readLostEvents map[string][]uint64
}

// NewEventsStats instantiates a new event statistics counter
func NewEventsStats(ebpfManager *manager.Manager, options manager.Options, config *config.Config) (*EventsStats, error) {
	es := EventsStats{
		config:              config,
		cpuCount:            runtime.NumCPU(),
		perfBufferStatsMaps: make(map[string][2]*lib.Map),
		perfBufferSize:      make(map[string]float64),

		perfBufferMapNameToStatsMapsName: ebpf.GetPerfBufferStatisticsMaps(),
		statsMapsNameToPerfBufferMapName: make(map[string]string),

		stats:          make(map[string][][maxEventType]PerfMapStats),
		readLostEvents: make(map[string][]uint64),
	}

	// compute statsMapPerfMap
	for perfMap, statsMaps := range es.perfBufferMapNameToStatsMapsName {
		for _, statsMap := range statsMaps {
			es.statsMapsNameToPerfBufferMapName[statsMap] = perfMap
		}
	}

	// Select perf buffer statistics maps
	for perfMapName, statsMapsNames := range es.perfBufferMapNameToStatsMapsName {
		var maps [2]*lib.Map
		for i, statsMapName := range statsMapsNames {
			stats, ok, err := ebpfManager.GetMap(statsMapName)
			if !ok {
				return nil, errors.Errorf("map %s not found", statsMapName)
			}
			if err != nil {
				return nil, err
			}
			maps[i] = stats
		}
		es.perfBufferStatsMaps[perfMapName] = maps
		// set default perf buffer size, it will be readjusted in the next loop if needed
		es.perfBufferSize[perfMapName] = float64(options.DefaultPerfRingBufferSize)
	}

	// Prepare user space counters
	for _, m := range ebpfManager.PerfMaps {
		var stats [][maxEventType]PerfMapStats
		var usrLostEvents []uint64

		for i := 0; i < es.cpuCount; i++ {
			stats = append(stats, [maxEventType]PerfMapStats{})
			usrLostEvents = append(usrLostEvents, 0)
		}

		es.stats[m.Name] = stats
		es.readLostEvents[m.Name] = usrLostEvents

		// update perf buffer size if needed
		if m.PerfRingBufferSize != 0 {
			es.perfBufferSize[m.Name] = float64(m.PerfRingBufferSize)
		}
	}

	// select the buffer selector map
	bufferSelector, ok, err := ebpfManager.GetMap("buffer_selector")
	if !ok {
		return nil, errors.Errorf("map buffer_selector not found")
	}
	if err != nil {
		return nil, err
	}
	es.bufferSelector = bufferSelector
	return &es, nil
}

// getPerfMapFromStatsMap returns the perf map associated with a stats map
func (e *EventsStats) getPerfMapFromStatsMap(statsMap string) string {
	perfMap, ok := e.statsMapsNameToPerfBufferMapName[statsMap]
	if ok {
		return perfMap
	}
	return fmt.Sprintf("unknown_%s", statsMap)
}

// getReadLostCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getReadLostCount(perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&e.readLostEvents[perfMap][cpu])
}

// GetReadLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (e *EventsStats) GetReadLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range e.readLostEvents[perfMap] {
			total += e.getReadLostCount(perfMap, i)
		}
		break
	case cpu >= 0:
		if e.cpuCount <= cpu {
			break
		}
		total += e.getReadLostCount(perfMap, cpu)
	}

	return total
}

// getAndResetReadLostCount is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getAndResetReadLostCount(perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&e.readLostEvents[perfMap][cpu], 0)
}

// GetAndResetReadLostCount returns the number of lost events and resets the counter for a given map and cpu. If a cpu of -1 is
// provided, the function will reset the counters of all the cpus for the provided map, and return the sum of all the
// lost events of all the cpus of the provided map.
func (e *EventsStats) GetAndResetReadLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range e.readLostEvents[perfMap] {
			total += e.getAndResetReadLostCount(perfMap, i)
		}
		break
	case cpu >= 0:
		if e.cpuCount <= cpu {
			break
		}
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

// getEventUsage is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getEventUsage(eventType EventType, perfMap string, cpu int) int64 {
	return atomic.LoadInt64(&e.stats[perfMap][cpu][eventType].Usage)
}

// getEventInQueue is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getEventInQueue(eventType EventType, perfMap string, cpu int) int64 {
	return atomic.LoadInt64(&e.stats[perfMap][cpu][eventType].InQueue)
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
				stats.Usage += e.getEventUsage(eventType, perfMap, i)
				stats.InQueue += e.getEventUsage(eventType, perfMap, i)
			}
			break
		case cpu >= 0:
			if e.cpuCount <= cpu {
				break
			}
			stats.Count += e.getEventCount(eventType, perfMap, cpu)
			stats.Bytes += e.getEventBytes(eventType, perfMap, cpu)
			stats.Usage += e.getEventUsage(eventType, perfMap, cpu)
			stats.InQueue += e.getEventUsage(eventType, perfMap, cpu)
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

// getAndResetEventUsage is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getAndResetEventUsage(eventType EventType, perfMap string, cpu int) int64 {
	return atomic.SwapInt64(&e.stats[perfMap][cpu][eventType].Usage, 0)
}

// getAndResetEventInQueue is an internal function, it can segfault if its parameters are incorrect.
func (e *EventsStats) getAndResetEventInQueue(eventType EventType, perfMap string, cpu int) int64 {
	return atomic.SwapInt64(&e.stats[perfMap][cpu][eventType].InQueue, 0)
}

// GetAndResetEventStats returns the number of received events of the specified type and resets the counter
func (e *EventsStats) GetAndResetEventStats(eventType EventType, perfMap string, cpu int) PerfMapStats {
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
				stats.Count += e.getAndResetEventCount(eventType, perfMap, i)
				stats.Bytes += e.getAndResetEventBytes(eventType, perfMap, i)
				stats.Usage += e.getAndResetEventUsage(eventType, perfMap, i)
				stats.InQueue += e.getAndResetEventInQueue(eventType, perfMap, i)
			}
			break
		case cpu >= 0:
			if e.cpuCount <= cpu {
				break
			}
			stats.Count += e.getAndResetEventCount(eventType, perfMap, cpu)
			stats.Bytes += e.getAndResetEventBytes(eventType, perfMap, cpu)
			stats.Usage += e.getAndResetEventUsage(eventType, perfMap, cpu)
			stats.InQueue += e.getAndResetEventInQueue(eventType, perfMap, cpu)
		}

	}
	return stats
}

// CountLostEvent adds `count` to the counter of lost events
func (e *EventsStats) CountLostEvent(count uint64, m *manager.PerfMap, cpu int) {
	// sanity check
	if (e.readLostEvents[m.Name] == nil) || (len(e.readLostEvents[m.Name]) <= cpu) {
		return
	}
	atomic.AddUint64(&e.readLostEvents[m.Name][cpu], count)
}

// CountEventType adds `count` to the counter of received events of the specified type
func (e *EventsStats) CountEvent(eventType EventType, count uint64, size uint64, m *manager.PerfMap, cpu int) {
	// sanity check
	if (e.stats[m.Name] == nil) || (len(e.stats[m.Name]) <= cpu) || (len(e.stats[m.Name][cpu]) <= int(eventType)) {
		return
	}

	atomic.AddUint64(&e.stats[m.Name][cpu][eventType].Count, count)
	atomic.AddUint64(&e.stats[m.Name][cpu][eventType].Bytes, size)

	if e.config.PerfBufferMonitor {
		atomic.AddInt64(&e.stats[m.Name][cpu][eventType].Usage, -int64(size))
		atomic.AddInt64(&e.stats[m.Name][cpu][eventType].InQueue, -int64(count))
	}
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
		id          uint32
		stats, zero PerfMapStats
		iterator    *lib.MapIterator
		tags        []string
	)

	// loop through the statistics buffers of each perf map
	for perfMapName, statsMaps := range e.perfBufferStatsMaps {
		// select the current statistics buffer in use
		statsMap := statsMaps[1-e.activeMapIndex]

		// loop through all the values of the active buffer
		iterator = statsMap.Iterate()
		for iterator.Next(&id, &stats) {
			// compute cpu and event type from id
			cpu := int(id / uint32(maxEventType))
			evtType := EventType(id % uint32(maxEventType))

			// sanity checks:
			//   - check if the computed cpu id is below the current cpu count
			//   - check if we collect some data on the provided perf map
			//   - check if the computed event id is below the current max event id
			if (e.stats[perfMapName] == nil) || (len(e.stats[perfMapName]) <= cpu) || (len(e.stats[perfMapName][cpu]) <= int(evtType)) {
				continue
			}

			// remove the entry from the kernel
			if err := statsMap.Put(&id, &zero); err != nil {
				return err
			}

			// prepare metrics tags
			tags = []string{
				fmt.Sprintf("map:%s", perfMapName),
				fmt.Sprintf("cpu:%d", cpu),
				fmt.Sprintf("event_type:%s", evtType),
			}

			if err := e.sendKernelStats(client, stats, tags, perfMapName, cpu, evtType); err != nil {
				return err
			}
		}
		if iterator.Err() != nil {
			return errors.Wrapf(iterator.Err(), "failed to dump statistics buffer %d of map %s", 1-e.activeMapIndex, perfMapName)
		}
	}
	return nil
}

func (e *EventsStats) sendKernelStats(client *statsd.Client, stats PerfMapStats, tags []string, perfMapName string, cpu int, evtType EventType) error {
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

	// update usage metric
	newUsage := atomic.AddInt64(&e.stats[perfMapName][cpu][evtType].Usage, int64(stats.Bytes))
	newInQueue := atomic.AddInt64(&e.stats[perfMapName][cpu][evtType].InQueue, int64(stats.Count))

	if evtType == FileOpenEventType && perfMapName == "events" {
		metric = MetricPrefix + ".perf_buffer.usage"
		// There is a race condition when the system is under pressure: between the time we read the perf buffer stats map
		// and the time we reach this point, the kernel might have written more events in the perf map, and those events
		// might have already been read in user space. In that case, usage will yield a negative value. In that case, set
		// the map usage to 0 as it makes more sense than a negative value.
		usage := math.Max(float64(newUsage)/e.perfBufferSize[perfMapName], 0)
		if err := client.Gauge(metric, usage*100, tags, 1.0); err != nil {
			return err
		}

		metric = MetricPrefix + ".perf_buffer.in_queue"
		// There is a race condition when the system is under pressure: between the time we read the perf buffer stats map
		// and the time we reach this point, the kernel might have written more events in the perf map, and those events
		// might have already been read in user space. In that case, usage will yield a negative value. In that case, set
		// the amount of queued events to 0 as it makes more sens than a negative value.
		if err := client.Count(metric, int64(math.Max(float64(newInQueue), 0)), tags, 1.0); err != nil {
			return err
		}
	}
	return nil
}

func (e *EventsStats) SendStats(client *statsd.Client) error {
	if e.config.PerfBufferMonitor {
		if err := e.collectAndSendKernelStats(client); err != nil {
			return err
		}
	}

	if err := e.sendEventsAndBytesReadStats(client); err != nil {
		return err
	}

	if err := e.sendLostEventsReadStats(client); err != nil {
		return err
	}

	// Update the active statistics map id
	if err := e.bufferSelector.Put(ebpf.BufferSelectorPerfBufferMonitorKey, 1-e.activeMapIndex); err != nil {
		return err
	}
	atomic.SwapUint32(&e.activeMapIndex, 1-e.activeMapIndex)
	return nil
}
