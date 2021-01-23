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

// PerfBufferMonitor holds statistics about the number of lost and received events
//nolint:structcheck,unused
type PerfBufferMonitor struct {
	// probe is a pointer to the Probe
	probe *Probe
	// statsdClient is a pointer to the statsdClient used to report the metrics of the perf buffer monitor
	statsdClient *statsd.Client
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

	// lastTimestamp is used to track the timestamp of the last event retrieved from the perf map
	lastTimestamp uint64
}

// NewPerfBufferMonitor instantiates a new event statistics counter
func NewPerfBufferMonitor(p *Probe, client *statsd.Client) (*PerfBufferMonitor, error) {
	es := PerfBufferMonitor{
		probe:               p,
		statsdClient:        client,
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
		stats, ok, err := p.manager.GetMap(statsMapName)
		if !ok {
			return nil, errors.Errorf("map %s not found", statsMapName)
		}
		if err != nil {
			return nil, err
		}

		es.perfBufferStatsMaps[perfMapName] = stats
		// set default perf buffer size, it will be readjusted in the next loop if needed
		es.perfBufferSize[perfMapName] = float64(p.managerOptions.DefaultPerfRingBufferSize)
	}

	// Prepare user space counters
	for _, m := range p.manager.PerfMaps {
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
func (pbm *PerfBufferMonitor) getLostCount(perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&pbm.readLostEvents[perfMap][cpu])
}

// GetLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (pbm *PerfBufferMonitor) GetLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range pbm.readLostEvents[perfMap] {
			total += pbm.getLostCount(perfMap, i)
		}
		break
	case cpu >= 0 && pbm.cpuCount > cpu:
		total += pbm.getLostCount(perfMap, cpu)
	}

	return total
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getKernelLostCount(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&pbm.kernelStats[perfMap][cpu][eventType].Lost)
}

// GetAndResetKernelLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (pbm *PerfBufferMonitor) GetAndResetKernelLostCount(perfMap string, cpu int, evtTypes ...EventType) uint64 {
	var total uint64
	var shouldCount bool

	// query the kernel maps
	_ = pbm.collectAndSendKernelStats(nil)

	for cpuID := range pbm.kernelStats[perfMap] {
		if cpu == -1 || cpu == cpuID {
			for kernelEvtType := range pbm.kernelStats[perfMap][cpuID] {
				shouldCount = len(evtTypes) == 0
				if !shouldCount {
					for evtType := range evtTypes {
						if evtType == kernelEvtType {
							shouldCount = true
						}
					}
				}
				if shouldCount {
					total += pbm.getKernelLostCount(EventType(kernelEvtType), perfMap, cpuID)
				}
			}
		}
	}

	return total
}

// getAndResetReadLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetReadLostCount(perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&pbm.readLostEvents[perfMap][cpu], 0)
}

// GetAndResetLostCount returns the number of lost events and resets the counter for a given map and cpu. If a cpu of -1 is
// provided, the function will reset the counters of all the cpus for the provided map, and return the sum of all the
// lost events of all the cpus of the provided map.
func (pbm *PerfBufferMonitor) GetAndResetLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range pbm.readLostEvents[perfMap] {
			total += pbm.getAndResetReadLostCount(perfMap, i)
		}
		break
	case cpu >= 0 && pbm.cpuCount > cpu:
		total += pbm.getAndResetReadLostCount(perfMap, cpu)
	}
	return total
}

// getEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getEventCount(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&pbm.stats[perfMap][cpu][eventType].Count)
}

// getEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getEventBytes(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.LoadUint64(&pbm.stats[perfMap][cpu][eventType].Bytes)
}

// getKernelEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) swapKernelEventCount(eventType EventType, perfMap string, cpu int, value uint64) uint64 {
	return atomic.SwapUint64(&pbm.kernelStats[perfMap][cpu][eventType].Count, value)
}

// getKernelEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) swapKernelEventBytes(eventType EventType, perfMap string, cpu int, value uint64) uint64 {
	return atomic.SwapUint64(&pbm.kernelStats[perfMap][cpu][eventType].Bytes, value)
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) swapKernelLostCount(eventType EventType, perfMap string, cpu int, value uint64) uint64 {
	return atomic.SwapUint64(&pbm.kernelStats[perfMap][cpu][eventType].Lost, value)
}

// GetEventStats returns the number of received events of the specified type and resets the counter
func (pbm *PerfBufferMonitor) GetEventStats(eventType EventType, perfMap string, cpu int) PerfMapStats {
	var stats PerfMapStats
	var maps []string

	if eventType >= maxEventType {
		return stats
	}

	switch {
	case len(perfMap) == 0:
		for m := range pbm.stats {
			maps = append(maps, m)
		}
		break
	case pbm.stats[perfMap] != nil:
		maps = append(maps, perfMap)
	}

	for _, m := range maps {

		switch {
		case cpu == -1:
			for i := range pbm.stats[m] {
				stats.Count += pbm.getEventCount(eventType, perfMap, i)
				stats.Bytes += pbm.getEventBytes(eventType, perfMap, i)
			}
			break
		case cpu >= 0 && pbm.cpuCount > cpu:
			stats.Count += pbm.getEventCount(eventType, perfMap, cpu)
			stats.Bytes += pbm.getEventBytes(eventType, perfMap, cpu)
		}

	}
	return stats
}

// getAndResetEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetEventCount(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&pbm.stats[perfMap][cpu][eventType].Count, 0)
}

// getAndResetEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetEventBytes(eventType EventType, perfMap string, cpu int) uint64 {
	return atomic.SwapUint64(&pbm.stats[perfMap][cpu][eventType].Bytes, 0)
}

// CountLostEvent adds `count` to the counter of lost events
func (pbm *PerfBufferMonitor) CountLostEvent(count uint64, m *manager.PerfMap, cpu int) {
	// sanity check
	if (pbm.readLostEvents[m.Name] == nil) || (len(pbm.readLostEvents[m.Name]) <= cpu) {
		return
	}
	atomic.AddUint64(&pbm.readLostEvents[m.Name][cpu], count)
}

// CountEvent adds `count` to the counter of received events of the specified type
func (pbm *PerfBufferMonitor) CountEvent(eventType EventType, timestamp uint64, count uint64, size uint64, m *manager.PerfMap, cpu int) {
	// check event order
	if timestamp < pbm.lastTimestamp && pbm.lastTimestamp != 0 {
		tags := []string{
			fmt.Sprintf("map:%s", m.Name),
			fmt.Sprintf("cpu:%d", cpu),
			fmt.Sprintf("event_type:%s", eventType),
		}
		_ = pbm.statsdClient.Count(MetricPerfBufferSortingError, 1, tags, 1.0)
	} else {
		pbm.lastTimestamp = timestamp
	}

	// sanity check
	if (pbm.stats[m.Name] == nil) || (len(pbm.stats[m.Name]) <= cpu) || (len(pbm.stats[m.Name][cpu]) <= int(eventType)) {
		return
	}

	atomic.AddUint64(&pbm.stats[m.Name][cpu][eventType].Count, count)
	atomic.AddUint64(&pbm.stats[m.Name][cpu][eventType].Bytes, size)
}

func (pbm *PerfBufferMonitor) sendEventsAndBytesReadStats(client *statsd.Client) error {
	for m := range pbm.stats {
		for cpu := range pbm.stats[m] {
			for eventType := range pbm.stats[m][cpu] {
				evtType := EventType(eventType)
				tags := []string{
					fmt.Sprintf("map:%s", m),
					fmt.Sprintf("cpu:%d", cpu),
					fmt.Sprintf("event_type:%s", evtType),
				}

				if err := client.Count(MetricPerfBufferEventsRead, int64(pbm.getAndResetEventCount(evtType, m, cpu)), tags, 1.0); err != nil {
					return err
				}

				if err := client.Count(MetricPerfBufferBytesRead, int64(pbm.getAndResetEventBytes(evtType, m, cpu)), tags, 1.0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (pbm *PerfBufferMonitor) sendLostEventsReadStats(client *statsd.Client) error {
	for m := range pbm.readLostEvents {
		var total int64
		perCPU := map[int]int64{}

		for cpu := range pbm.readLostEvents[m] {
			tags := []string{
				fmt.Sprintf("map:%s", m),
				fmt.Sprintf("cpu:%d", cpu),
			}
			count := int64(pbm.getAndResetReadLostCount(m, cpu))
			if err := client.Count(MetricPerfBufferLostRead, count, tags, 1.0); err != nil {
				return err
			}

			total += count
			perCPU[cpu] += count
		}

		if total > 0 {
			pbm.probe.DispatchCustomEvent(
				NewEventLostReadEvent(m, perCPU),
			)
		}
	}
	return nil
}

func (pbm *PerfBufferMonitor) collectAndSendKernelStats(client *statsd.Client) error {
	var (
		id       uint32
		iterator *lib.MapIterator
		tags     []string
		tmpCount uint64
	)
	cpuStats := make([]PerfMapStats, runtime.NumCPU())

	// loop through the statistics buffers of each perf map
	for perfMapName, statsMap := range pbm.perfBufferStatsMaps {
		// total and perEventPerCPU are used for alerting
		var total uint64
		perEventPerCPU := map[string]map[int]uint64{}

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
				if (pbm.stats[perfMapName] == nil) || (len(pbm.stats[perfMapName]) <= cpu) || (len(pbm.stats[perfMapName][cpu]) <= int(evtType)) {
					return nil
				}

				// make sure perEventPerCPU is properly initialized
				if _, ok := perEventPerCPU[evtType.String()]; !ok {
					perEventPerCPU[evtType.String()] = map[int]uint64{}
				}

				// prepare metrics tags
				tags = []string{
					fmt.Sprintf("map:%s", perfMapName),
					fmt.Sprintf("cpu:%d", cpu),
					fmt.Sprintf("event_type:%s", evtType),
				}

				// Update stats to avoid sending twice the same data points
				if tmpCount = pbm.swapKernelEventBytes(evtType, perfMapName, cpu, stats.Bytes); tmpCount <= stats.Bytes {
					stats.Bytes -= tmpCount
				}
				if tmpCount = pbm.swapKernelEventCount(evtType, perfMapName, cpu, stats.Count); tmpCount <= stats.Count {
					stats.Count -= tmpCount
				}
				if tmpCount = pbm.swapKernelLostCount(evtType, perfMapName, cpu, stats.Lost); tmpCount <= stats.Lost {
					stats.Lost -= tmpCount
				}

				if client != nil {
					if err := pbm.sendKernelStats(client, stats, tags); err != nil {
						return err
					}
				}
				total += stats.Lost
				perEventPerCPU[evtType.String()][cpu] += stats.Lost
			}
		}
		if iterator.Err() != nil {
			return errors.Wrapf(iterator.Err(), "failed to dump the statistics buffer of map %s", perfMapName)
		}

		// send an alert if events were lost
		if total > 0 {
			pbm.probe.DispatchCustomEvent(
				NewEventLostWriteEvent(perfMapName, perEventPerCPU),
			)
		}
	}
	return nil
}

func (pbm *PerfBufferMonitor) sendKernelStats(client *statsd.Client, stats PerfMapStats, tags []string) error {
	if err := client.Count(MetricPerfBufferEventsWrite, int64(stats.Count), tags, 1.0); err != nil {
		return err
	}

	if err := client.Count(MetricPerfBufferBytesWrite, int64(stats.Bytes), tags, 1.0); err != nil {
		return err
	}

	return client.Count(MetricPerfBufferLostWrite, int64(stats.Lost), tags, 1.0)
}

// SendStats send event stats using the provided statsd client
func (pbm *PerfBufferMonitor) SendStats() error {
	if err := pbm.collectAndSendKernelStats(pbm.statsdClient); err != nil {
		return err
	}

	if err := pbm.sendEventsAndBytesReadStats(pbm.statsdClient); err != nil {
		return err
	}

	if err := pbm.sendLostEventsReadStats(pbm.statsdClient); err != nil {
		return err
	}
	return nil
}
