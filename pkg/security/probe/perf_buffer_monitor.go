// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/alecthomas/units"
	lib "github.com/cilium/ebpf"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PerfMapStats contains the collected metrics for one event and one cpu in a perf buffer statistics map
type PerfMapStats struct {
	Bytes *atomic.Uint64
	Count *atomic.Uint64
	Lost  *atomic.Uint64
}

// NewPerfMapStats returns a new PerfMapStats correctly initialized
func NewPerfMapStats() PerfMapStats {
	return PerfMapStats{
		Bytes: atomic.NewUint64(0),
		Count: atomic.NewUint64(0),
		Lost:  atomic.NewUint64(0),
	}
}

// UnmarshalBinary parses a map entry and populates the current PerfMapStats instance
func (s *PerfMapStats) UnmarshalBinary(data []byte) error {
	if len(data) < 24 {
		return model.ErrNotEnoughData
	}
	s.Bytes = atomic.NewUint64(model.ByteOrder.Uint64(data[0:8]))
	s.Count = atomic.NewUint64(model.ByteOrder.Uint64(data[8:16]))
	s.Lost = atomic.NewUint64(model.ByteOrder.Uint64(data[16:24]))
	return nil
}

// PerfBufferMonitor holds statistics about the number of lost and received events
type PerfBufferMonitor struct {
	// probe is a pointer to the Probe
	probe        *Probe
	config       *config.Config
	statsdClient statsd.ClientInterface
	eRPC         *erpc.ERPC

	// numCPU holds the current count of CPU
	numCPU int
	// perfBufferStatsMaps holds the pointers to the statistics kernel maps
	perfBufferStatsMaps map[string]*perfBufferStatMap

	// perfBufferMapNameToStatsMapsName maps a perf buffer to its statistics maps
	perfBufferMapNameToStatsMapsName map[string]string

	// ringBufferMapNameToStatsMapsName maps a ring buffer to its statistics maps
	ringBufferMapNameToStatsMapsName map[string]string

	// stats holds the collected user space metrics
	stats map[string][][model.MaxKernelEventType]PerfMapStats
	// kernelStats holds the aggregated kernel space metrics
	kernelStats map[string][][model.MaxKernelEventType]PerfMapStats
	// readLostEvents is the count of lost events, collected by reading the perf buffer.  Note that the
	// slices of Uint64 are properly aligned for atomic access, and are not moved after creation (they
	// are indexed by cpuid)
	readLostEvents map[string][]*atomic.Uint64
	// sortingErrorStats holds the count of events that indicate that at least 1 event is miss ordered
	sortingErrorStats map[string][model.MaxKernelEventType]*atomic.Int64

	// lastTimestamp is used to track the timestamp of the last event retrieved from the perf map
	lastTimestamp uint64
	// shouldBumpGeneration is used to track if the dentry cache generations should be bumped
	shouldBumpGeneration *atomic.Bool
}

type ringBufferStatMap struct {
	*lib.Map
	capacity uint64
}

type perfBufferStatMap struct {
	ebpfMap           *lib.Map
	ebpfRingBufferMap *ringBufferStatMap
}

// NewPerfBufferMonitor instantiates a new event statistics counter
func NewPerfBufferMonitor(p *Probe) (*PerfBufferMonitor, error) {
	pbm := PerfBufferMonitor{
		probe:               p,
		config:              p.Config.Probe,
		statsdClient:        p.StatsdClient,
		eRPC:                p.Erpc,
		perfBufferStatsMaps: make(map[string]*perfBufferStatMap),

		perfBufferMapNameToStatsMapsName: probes.GetPerfBufferStatisticsMaps(),
		ringBufferMapNameToStatsMapsName: probes.GetRingBufferStatisticsMaps(),

		stats:             make(map[string][][model.MaxKernelEventType]PerfMapStats),
		kernelStats:       make(map[string][][model.MaxKernelEventType]PerfMapStats),
		readLostEvents:    make(map[string][]*atomic.Uint64),
		sortingErrorStats: make(map[string][model.MaxKernelEventType]*atomic.Int64),

		shouldBumpGeneration: atomic.NewBool(false),
	}
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}
	pbm.numCPU = numCPU

	maps := make(map[string]int, len(p.Manager.PerfMaps)+len(p.Manager.RingBuffers))
	for _, pm := range p.Manager.PerfMaps {
		maps[pm.Name] = pm.PerfRingBufferSize
	}

	for _, rb := range p.Manager.RingBuffers {
		maps[rb.Name] = rb.RingBufferSize
	}

	// Select perf buffer statistics maps
	for perfMapName, statsMapName := range pbm.perfBufferMapNameToStatsMapsName {
		stats, ok, err := p.Manager.GetMap(statsMapName)
		if !ok {
			return nil, fmt.Errorf("map %s not found", statsMapName)
		}
		if err != nil {
			return nil, err
		}

		pbm.perfBufferStatsMaps[perfMapName] = &perfBufferStatMap{ebpfMap: stats}

		if p.UseRingBuffers() {
			// set default perf buffer size, it will be readjusted in the next loop if needed
			if ringbufStatsMapName := pbm.ringBufferMapNameToStatsMapsName[perfMapName]; ringbufStatsMapName != "" {
				ringBufferStats, found, _ := p.Manager.GetMap(ringbufStatsMapName)
				if !found {
					return nil, fmt.Errorf("map %s not found", ringbufStatsMapName)
				}

				ringBufferMap, found, _ := p.Manager.GetMap(perfMapName)
				if !found {
					return nil, fmt.Errorf("map %s not found", perfMapName)
				}

				pbm.perfBufferStatsMaps[perfMapName].ebpfRingBufferMap = &ringBufferStatMap{
					Map:      ringBufferStats,
					capacity: uint64(ringBufferMap.MaxEntries()),
				}
			}
		}
	}

	// Prepare user space counters
	for mapName := range maps {
		var stats, kernelStats [][model.MaxKernelEventType]PerfMapStats
		var usrLostEvents []*atomic.Uint64
		var sortingErrorStats [model.MaxKernelEventType]*atomic.Int64

		for i := 0; i < pbm.numCPU; i++ {
			stats = append(stats, initPerfMapStatsArray())
			kernelStats = append(kernelStats, initPerfMapStatsArray())
			usrLostEvents = append(usrLostEvents, atomic.NewUint64(0))
		}

		for i := 0; i < int(model.MaxKernelEventType); i++ {
			sortingErrorStats[i] = atomic.NewInt64(0)
		}

		pbm.stats[mapName] = stats
		pbm.kernelStats[mapName] = kernelStats
		pbm.readLostEvents[mapName] = usrLostEvents
		pbm.sortingErrorStats[mapName] = sortingErrorStats
	}
	log.Debugf("monitoring perf ring buffer on %d CPU, %d events", pbm.numCPU, model.MaxKernelEventType)
	return &pbm, nil
}

func initPerfMapStatsArray() [model.MaxKernelEventType]PerfMapStats {
	var arr [model.MaxKernelEventType]PerfMapStats
	for i := 0; i < len(arr); i++ {
		arr[i] = NewPerfMapStats()
	}
	return arr
}

// getLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getLostCount(perfMap string, cpu int) uint64 {
	return pbm.readLostEvents[perfMap][cpu].Load()
}

// GetLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus. (only used in tests)
func (pbm *PerfBufferMonitor) GetLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range pbm.readLostEvents[perfMap] {
			total += pbm.getLostCount(perfMap, i)
		}
	case cpu >= 0 && pbm.numCPU > cpu:
		total += pbm.getLostCount(perfMap, cpu)
	}

	return total
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getKernelLostCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Lost.Load()
}

// GetKernelLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (pbm *PerfBufferMonitor) GetKernelLostCount(perfMap string, cpu int, evtTypes ...model.EventType) uint64 {
	var total uint64
	var shouldCount bool

	// query the kernel maps
	_ = pbm.collectAndSendKernelStats(&statsd.NoOpClient{})

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
					total += pbm.getKernelLostCount(model.EventType(kernelEvtType), perfMap, cpuID)
				}
			}
		}
	}

	return total
}

// getAndResetReadLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetReadLostCount(perfMap string, cpu int) uint64 {
	return pbm.readLostEvents[perfMap][cpu].Swap(0)
}

// GetAndResetLostCount returns the number of lost events and resets the counter for a given map and cpu. If a cpu of -1 is
// provided, the function will reset the counters of all the cpus for the provided map, and return the sum of all the
// lost events of all the cpus of the provided map.  (only used in tests)
func (pbm *PerfBufferMonitor) GetAndResetLostCount(perfMap string, cpu int) uint64 {
	var total uint64

	switch {
	case cpu == -1:
		for i := range pbm.readLostEvents[perfMap] {
			total += pbm.getAndResetReadLostCount(perfMap, i)
		}
	case cpu >= 0 && pbm.numCPU > cpu:
		total += pbm.getAndResetReadLostCount(perfMap, cpu)
	}
	return total
}

// getEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getEventCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.stats[perfMap][cpu][eventType].Count.Load()
}

// getEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getEventBytes(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.stats[perfMap][cpu][eventType].Bytes.Load()
}

// getKernelEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getKernelEventCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Count.Load()
}

// getEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getKernelEventBytes(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Bytes.Load()
}

// getKernelEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) swapKernelEventCount(eventType model.EventType, perfMap string, cpu int, value uint64) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Count.Swap(value)
}

// getKernelEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) swapKernelEventBytes(eventType model.EventType, perfMap string, cpu int, value uint64) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Bytes.Swap(value)
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) swapKernelLostCount(eventType model.EventType, perfMap string, cpu int, value uint64) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Lost.Swap(value)
}

// GetEventStats returns the number of received events of the specified type (only used in tests)
func (pbm *PerfBufferMonitor) GetEventStats(eventType model.EventType, perfMap string, cpu int) (PerfMapStats, PerfMapStats) {
	stats, kernelStats := NewPerfMapStats(), NewPerfMapStats()
	var maps []string

	if eventType >= model.MaxKernelEventType {
		return stats, kernelStats
	}

	switch {
	case len(perfMap) == 0:
		for m := range pbm.stats {
			maps = append(maps, m)
		}
	case pbm.stats[perfMap] != nil:
		maps = append(maps, perfMap)
	}

	for _, m := range maps {

		switch {
		case cpu == -1:
			for i := range pbm.stats[m] {
				stats.Count.Add(pbm.getEventCount(eventType, perfMap, i))
				stats.Bytes.Add(pbm.getEventBytes(eventType, perfMap, i))

				kernelStats.Count.Add(pbm.getKernelEventCount(eventType, perfMap, i))
				kernelStats.Bytes.Add(pbm.getKernelEventBytes(eventType, perfMap, i))
				kernelStats.Lost.Add(pbm.getKernelLostCount(eventType, perfMap, i))
			}
		case cpu >= 0 && pbm.numCPU > cpu:
			stats.Count.Add(pbm.getEventCount(eventType, perfMap, cpu))
			stats.Bytes.Add(pbm.getEventBytes(eventType, perfMap, cpu))

			kernelStats.Count.Add(pbm.getKernelEventCount(eventType, perfMap, cpu))
			kernelStats.Bytes.Add(pbm.getKernelEventBytes(eventType, perfMap, cpu))
			kernelStats.Lost.Add(pbm.getKernelLostCount(eventType, perfMap, cpu))
		}

	}
	return stats, kernelStats
}

// getAndResetEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetEventCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.stats[perfMap][cpu][eventType].Count.Swap(0)
}

// getAndResetEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetEventBytes(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.stats[perfMap][cpu][eventType].Bytes.Swap(0)
}

// getAndResetSortingErrorCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *PerfBufferMonitor) getAndResetSortingErrorCount(eventType model.EventType, perfMap string) int64 {
	return pbm.sortingErrorStats[perfMap][eventType].Swap(0)
}

// CountLostEvent adds `count` to the counter of lost events
func (pbm *PerfBufferMonitor) CountLostEvent(count uint64, mapName string, cpu int) {
	// sanity check
	if (pbm.readLostEvents[mapName] == nil) || (len(pbm.readLostEvents[mapName]) <= cpu) {
		return
	}
	pbm.readLostEvents[mapName][cpu].Add(count)
}

// CountEvent adds `count` to the counter of received events of the specified type
func (pbm *PerfBufferMonitor) CountEvent(eventType model.EventType, timestamp uint64, count uint64, size uint64, mapName string, cpu int) {
	// check event order
	if timestamp < pbm.lastTimestamp && pbm.lastTimestamp != 0 {
		pbm.sortingErrorStats[mapName][eventType].Inc()
		pbm.shouldBumpGeneration.Store(true)
	} else {
		pbm.lastTimestamp = timestamp
	}

	// sanity check
	if (pbm.stats[mapName] == nil) || (len(pbm.stats[mapName]) <= cpu) || (len(pbm.stats[mapName][cpu]) <= int(eventType)) {
		return
	}

	pbm.stats[mapName][cpu][eventType].Count.Add(count)
	pbm.stats[mapName][cpu][eventType].Bytes.Add(size)
}

func (pbm *PerfBufferMonitor) sendEventsAndBytesReadStats(client statsd.ClientInterface) error {
	var count int64
	var err error
	tags := []string{pbm.config.StatsTagsCardinality, "", ""}

	for m := range pbm.stats {
		tags[1] = fmt.Sprintf("map:%s", m)
		for cpu := range pbm.stats[m] {
			for eventType := range pbm.stats[m][cpu] {
				evtType := model.EventType(eventType)
				tags[2] = fmt.Sprintf("event_type:%s", evtType)

				if count = int64(pbm.getAndResetEventCount(evtType, m, cpu)); count > 0 {
					if err = client.Count(metrics.MetricPerfBufferEventsRead, count, tags, 1.0); err != nil {
						return err
					}
				}

				if count = int64(pbm.getAndResetEventBytes(evtType, m, cpu)); count > 0 {
					if err = client.Count(metrics.MetricPerfBufferBytesRead, count, tags, 1.0); err != nil {
						return err
					}
				}

				if count = pbm.getAndResetSortingErrorCount(evtType, m); count > 0 {
					if err = pbm.statsdClient.Count(metrics.MetricPerfBufferSortingError, count, tags, 1.0); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (pbm *PerfBufferMonitor) sendLostEventsReadStats(client statsd.ClientInterface) error {
	tags := []string{pbm.config.StatsTagsCardinality, ""}

	for m := range pbm.readLostEvents {
		var total float64
		tags[1] = fmt.Sprintf("map:%s", m)

		for cpu := range pbm.readLostEvents[m] {
			if count := float64(pbm.getAndResetReadLostCount(m, cpu)); count > 0 {
				if err := client.Count(metrics.MetricPerfBufferLostRead, int64(count), tags, 1.0); err != nil {
					return err
				}
				total += count
			}
		}

		// Disable custom event for now
		// if total > 0 {
		// 	pbm.probe.DispatchCustomEvent(
		// 		NewEventLostReadEvent(m, total),
		// 	)
		// }
	}
	return nil
}

func (pbm *PerfBufferMonitor) getRingbufUsage(statsMap *perfBufferStatMap) (uint64, error) {
	if err := pbm.eRPC.Request(&erpc.ERPCRequest{OP: erpc.GetRingbufUsage}); err != nil {
		return 0, err
	}

	var ringUsage uint64
	if err := statsMap.ebpfRingBufferMap.Lookup(int32(0), &ringUsage); err != nil {
		return 0, fmt.Errorf("failed to retrieve ring buffer usage")
	}

	return ringUsage, nil
}

func (pbm *PerfBufferMonitor) collectAndSendKernelStats(client statsd.ClientInterface) error {
	var (
		id       uint32
		iterator *lib.MapIterator
		tmpCount uint64
	)

	cpuStats := make([]PerfMapStats, pbm.numCPU)
	for i := 0; i < pbm.numCPU; i++ {
		cpuStats[i] = NewPerfMapStats()
	}

	tags := []string{pbm.config.StatsTagsCardinality, "", ""}

	// loop through the statistics buffers of each perf map
	for perfMapName, statsMap := range pbm.perfBufferStatsMaps {
		// total and perEvent are used for alerting
		var total uint64
		perEvent := map[string]uint64{}
		mapNameTag := fmt.Sprintf("map:%s", perfMapName)
		tags[1] = mapNameTag

		// loop through all the values of the active buffer
		iterator = statsMap.ebpfMap.Iterate()
		for iterator.Next(&id, &cpuStats) {
			if id == 0 {
				// first event type is 1
				continue
			}

			// retrieve event type from key
			evtType := model.EventType(id % uint32(model.MaxKernelEventType))
			tags[2] = fmt.Sprintf("event_type:%s", evtType)

			// loop over each cpu entry
			for cpu, stats := range cpuStats {
				// sanity checks:
				//   - check if the computed cpu id is below the current cpu count
				//   - check if we collect some data on the provided perf map
				//   - check if the computed event id is below the current max event id
				if (pbm.stats[perfMapName] == nil) || (len(pbm.stats[perfMapName]) <= cpu) || (len(pbm.stats[perfMapName][cpu]) <= int(evtType)) {
					return nil
				}

				// make sure perEvent is properly initialized
				if _, ok := perEvent[evtType.String()]; !ok {
					perEvent[evtType.String()] = 0
				}

				// Update stats to avoid sending twice the same data points
				if tmpCount = pbm.swapKernelEventBytes(evtType, perfMapName, cpu, stats.Bytes.Load()); tmpCount <= stats.Bytes.Load() {
					stats.Bytes.Sub(tmpCount)
				}
				if tmpCount = pbm.swapKernelEventCount(evtType, perfMapName, cpu, stats.Count.Load()); tmpCount <= stats.Count.Load() {
					stats.Count.Sub(tmpCount)
				}
				if tmpCount = pbm.swapKernelLostCount(evtType, perfMapName, cpu, stats.Lost.Load()); tmpCount <= stats.Lost.Load() {
					stats.Lost.Sub(tmpCount)

					// purge dentry resolver generation if needed
					if evtType == model.FileRenameEventType || evtType == model.FileUnlinkEventType || evtType == model.FileRmdirEventType {
						pbm.shouldBumpGeneration.Store(true)
					}
				}

				if err := pbm.sendKernelStats(client, stats, tags); err != nil {
					return err
				}
				total += stats.Lost.Load()
				perEvent[evtType.String()] += stats.Lost.Load()
			}
		}
		if err := iterator.Err(); err != nil {
			return fmt.Errorf("failed to dump the statistics buffer of map %s: %w", perfMapName, err)
		}

		if statsMap.ebpfRingBufferMap != nil {
			ringUsage, err := pbm.getRingbufUsage(statsMap)
			if err != nil {
				return err
			}

			// The capacity of ring buffer has to be a power of 2 and a multiple of 4096,
			// the cardinality is low so we use it as a tag.
			tags := []string{pbm.config.StatsTagsCardinality, mapNameTag, units.ToString(int64(statsMap.ebpfRingBufferMap.capacity), 1024, "", "M")}
			if err := client.Gauge(metrics.MetricPerfBufferBytesInUse, float64(ringUsage*100/statsMap.ebpfRingBufferMap.capacity), tags, 1.0); err != nil {
				return err
			}
		}

		// send an alert if events were lost
		if total > 0 {
			pbm.probe.DispatchCustomEvent(
				NewEventLostWriteEvent(perfMapName, perEvent),
			)

			// snapshot traced cgroups if a CgroupTracing event was lost
			if pbm.probe.IsActivityDumpEnabled() && perEvent[model.CgroupTracingEventType.String()] > 0 {
				pbm.probe.monitor.activityDumpManager.SnapshotTracedCgroups()
			}
		}
	}
	return nil
}

func (pbm *PerfBufferMonitor) sendKernelStats(client statsd.ClientInterface, stats PerfMapStats, tags []string) error {
	if stats.Count.Load() > 0 {
		if err := client.Count(metrics.MetricPerfBufferEventsWrite, int64(stats.Count.Load()), tags, 1.0); err != nil {
			return err
		}
	}

	if stats.Bytes.Load() > 0 {
		if err := client.Count(metrics.MetricPerfBufferBytesWrite, int64(stats.Bytes.Load()), tags, 1.0); err != nil {
			return err
		}
	}

	if stats.Lost.Load() > 0 {
		if err := client.Count(metrics.MetricPerfBufferLostWrite, int64(stats.Lost.Load()), tags, 1.0); err != nil {
			return err
		}
	}

	return nil
}

// SendStats send event stats using the provided statsd client
func (pbm *PerfBufferMonitor) SendStats() error {
	if err := pbm.collectAndSendKernelStats(pbm.statsdClient); err != nil {
		return err
	}

	if pbm.shouldBumpGeneration.Swap(false) {
		pbm.probe.resolvers.DentryResolver.BumpCacheGenerations()
	}

	if err := pbm.sendEventsAndBytesReadStats(pbm.statsdClient); err != nil {
		return err
	}

	return pbm.sendLostEventsReadStats(pbm.statsdClient)
}
