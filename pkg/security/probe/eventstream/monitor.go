// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package eventstream holds eventstream related files
package eventstream

import (
	"encoding/binary"
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
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

// GenericStats defines a generic stats struct
type GenericStats struct {
	Bytes *atomic.Uint64
	Count *atomic.Uint64
}

// EventStats defines a event stats struct
type EventStats struct {
	GenericStats
	ADSaved      *atomic.Uint64
	SortingError *atomic.Uint64
}

// LostEventCounter interface used to monitor event loss
type LostEventCounter interface {
	CountLostEvent(count uint64, perfMapName string, CPU int)
}

// MapStats contains the collected metrics for one event and one cpu in a perf buffer statistics map
type MapStats struct {
	GenericStats
	Lost *atomic.Uint64
}

// InvalidEventCause is an enum that represents the cause of an invalid event
type InvalidEventCause int

const (
	// InvalidType indicates that the type of an event is invalid
	InvalidType InvalidEventCause = iota
	maxInvalidEventCause
)

func (cause InvalidEventCause) String() string {
	if cause < 0 || cause >= maxInvalidEventCause {
		return "unknown"
	}
	return [...]string{"invalid_type"}[cause]
}

func makeGenericStats() GenericStats {
	return GenericStats{
		Bytes: atomic.NewUint64(0),
		Count: atomic.NewUint64(0),
	}
}

func makeEventStats() EventStats {
	return EventStats{
		GenericStats: makeGenericStats(),
		ADSaved:      atomic.NewUint64(0),
		SortingError: atomic.NewUint64(0),
	}
}

func makeMapStats() MapStats {
	return MapStats{
		GenericStats: makeGenericStats(),
		Lost:         atomic.NewUint64(0),
	}
}

// UnmarshalBinary parses a map entry and populates the current EventStreamMapStats instance
func (s *MapStats) UnmarshalBinary(data []byte) error {
	if len(data) < 24 {
		return model.ErrNotEnoughData
	}
	s.Bytes = atomic.NewUint64(binary.NativeEndian.Uint64(data[0:8]))
	s.Count = atomic.NewUint64(binary.NativeEndian.Uint64(data[8:16]))
	s.Lost = atomic.NewUint64(binary.NativeEndian.Uint64(data[16:24]))
	return nil
}

// Monitor holds statistics about the number of lost and received events
type Monitor struct {
	config       *config.Config
	statsdClient statsd.ClientInterface
	eRPC         *erpc.ERPC
	manager      *manager.Manager

	// numCPU holds the current count of CPU
	numCPU int
	// perfBufferStatsMaps holds the pointers to the statistics kernel maps
	perfBufferStatsMaps map[string]*statMap

	// perfBufferMapNameToStatsMapsName maps a perf buffer to its statistics maps
	perfBufferMapNameToStatsMapsName map[string]string

	// ringBufferMapNameToStatsMapsName maps a ring buffer to its statistics maps
	ringBufferMapNameToStatsMapsName map[string]string

	// stats holds the collected user space metrics
	eventStats map[string][][model.MaxKernelEventType]EventStats
	// kernelStats holds the aggregated kernel space metrics
	kernelStats map[string][][model.MaxKernelEventType]MapStats
	// readLostEvents is the count of lost events, collected by reading the perf buffer.  Note that the
	// slices of Uint64 are properly aligned for atomic access, and are not moved after creation (they
	// are indexed by cpuid)
	readLostEvents map[string][]*atomic.Uint64
	// invalidEventStats tracks statistics for invalid events retrieved from the eventstream
	invalidEventStats map[string][maxInvalidEventCause]*GenericStats

	// lastTimestamp is used to track the timestamp of the last event retrieved from the perf map
	lastTimestamp uint64

	// call that can be used to get notify when events are lost
	onEventLost func(perfMapName string, perEvent map[string]uint64)
}

type ringBufferStatMap struct {
	*lib.Map
	capacity uint64
}

type statMap struct {
	ebpfMap           *lib.Map
	ebpfRingBufferMap *ringBufferStatMap
}

// NewEventStreamMonitor instantiates a new event statistics counter
func NewEventStreamMonitor(config *config.Config, eRPC *erpc.ERPC, manager *manager.Manager, statsdClient statsd.ClientInterface, onEventLost func(perfMapName string, perEvent map[string]uint64), useRingBuffers bool) (*Monitor, error) {
	pbm := Monitor{
		config:              config,
		statsdClient:        statsdClient,
		eRPC:                eRPC,
		manager:             manager,
		perfBufferStatsMaps: make(map[string]*statMap),

		perfBufferMapNameToStatsMapsName: probes.GetPerfBufferStatisticsMaps(),
		ringBufferMapNameToStatsMapsName: probes.GetRingBufferStatisticsMaps(),

		eventStats:        make(map[string][][model.MaxKernelEventType]EventStats),
		kernelStats:       make(map[string][][model.MaxKernelEventType]MapStats),
		readLostEvents:    make(map[string][]*atomic.Uint64),
		invalidEventStats: make(map[string][maxInvalidEventCause]*GenericStats),

		onEventLost: onEventLost,
	}
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}
	pbm.numCPU = numCPU

	maps := make(map[string]int, len(manager.PerfMaps)+len(manager.RingBuffers))
	for _, pm := range manager.PerfMaps {
		maps[pm.Name] = pm.PerfRingBufferSize
	}

	for _, rb := range manager.RingBuffers {
		maps[rb.Name] = rb.BufferSize()
	}

	// Select perf buffer statistics maps
	for perfMapName, statsMapName := range pbm.perfBufferMapNameToStatsMapsName {
		stats, ok, err := manager.GetMap(statsMapName)
		if !ok {
			return nil, fmt.Errorf("map %s not found", statsMapName)
		}
		if err != nil {
			return nil, err
		}

		pbm.perfBufferStatsMaps[perfMapName] = &statMap{ebpfMap: stats}

		if useRingBuffers {
			// set default perf buffer size, it will be readjusted in the next loop if needed
			if ringbufStatsMapName := pbm.ringBufferMapNameToStatsMapsName[perfMapName]; ringbufStatsMapName != "" {
				ringBufferStats, found, _ := manager.GetMap(ringbufStatsMapName)
				if !found {
					return nil, fmt.Errorf("map %s not found", ringbufStatsMapName)
				}

				ringBufferMap, found, _ := manager.GetMap(perfMapName)
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
		var (
			eventStats        [][model.MaxKernelEventType]EventStats
			kernelStats       [][model.MaxKernelEventType]MapStats
			usrLostEvents     []*atomic.Uint64
			invalidEventStats [maxInvalidEventCause]*GenericStats
		)

		for i := 0; i < pbm.numCPU; i++ {
			eventStats = append(eventStats, initEventStatsArray())
			kernelStats = append(kernelStats, initMapStatsArray())
			usrLostEvents = append(usrLostEvents, atomic.NewUint64(0))
		}

		for i := 0; i < int(maxInvalidEventCause); i++ {
			stats := makeGenericStats()
			invalidEventStats[i] = &stats
		}

		pbm.eventStats[mapName] = eventStats
		pbm.kernelStats[mapName] = kernelStats
		pbm.readLostEvents[mapName] = usrLostEvents
		pbm.invalidEventStats[mapName] = invalidEventStats
	}
	log.Debugf("monitoring perf ring buffer on %d CPU, %d events", pbm.numCPU, model.MaxKernelEventType)
	return &pbm, nil
}

func (s *GenericStats) getAndReset() (uint64, uint64) {
	return s.Count.Swap(0), s.Bytes.Swap(0)
}

func initEventStatsArray() [model.MaxKernelEventType]EventStats {
	var arr [model.MaxKernelEventType]EventStats
	for i := 0; i < len(arr); i++ {
		arr[i] = makeEventStats()
	}
	return arr
}

func initMapStatsArray() [model.MaxKernelEventType]MapStats {
	var arr [model.MaxKernelEventType]MapStats
	for i := 0; i < len(arr); i++ {
		arr[i] = makeMapStats()
	}
	return arr
}

// getLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) getLostCount(perfMap string, cpu int) uint64 {
	return pbm.readLostEvents[perfMap][cpu].Load()
}

// GetLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus. (only used in tests)
func (pbm *Monitor) GetLostCount(perfMap string, cpu int) uint64 {
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
func (pbm *Monitor) getKernelLostCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Lost.Load()
}

// GetKernelLostCount returns the number of lost events for a given map and cpu. If a cpu of -1 is provided, the function will
// return the sum of all the lost events of all the cpus.
func (pbm *Monitor) GetKernelLostCount(perfMap string, cpu int, evtTypes ...model.EventType) uint64 {
	var total uint64

	// query the kernel maps
	_ = pbm.collectAndSendKernelStats(&statsd.NoOpClient{})

	for cpuID := range pbm.kernelStats[perfMap] {
		if cpu == -1 || cpu == cpuID {
			for kernelEvtType := range pbm.kernelStats[perfMap][cpuID] {
				var shouldCount bool

				for _, evtType := range evtTypes {
					if evtType == model.EventType(kernelEvtType) || evtType == model.MaxKernelEventType {
						shouldCount = true
						break
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
func (pbm *Monitor) getAndResetReadLostCount(perfMap string, cpu int) uint64 {
	return pbm.readLostEvents[perfMap][cpu].Swap(0)
}

// GetAndResetLostCount returns the number of lost events and resets the counter for a given map and cpu. If a cpu of -1 is
// provided, the function will reset the counters of all the cpus for the provided map, and return the sum of all the
// lost events of all the cpus of the provided map.  (only used in tests)
func (pbm *Monitor) GetAndResetLostCount(perfMap string, cpu int) uint64 {
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
func (pbm *Monitor) getEventCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.eventStats[perfMap][cpu][eventType].Count.Load()
}

// getEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) getEventBytes(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.eventStats[perfMap][cpu][eventType].Bytes.Load()
}

// getKernelEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) getKernelEventCount(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Count.Load()
}

// getEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) getKernelEventBytes(eventType model.EventType, perfMap string, cpu int) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Bytes.Load()
}

// getKernelEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) swapKernelEventCount(eventType model.EventType, perfMap string, cpu int, value uint64) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Count.Swap(value)
}

// getKernelEventBytes is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) swapKernelEventBytes(eventType model.EventType, perfMap string, cpu int, value uint64) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Bytes.Swap(value)
}

// getKernelLostCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) swapKernelLostCount(eventType model.EventType, perfMap string, cpu int, value uint64) uint64 {
	return pbm.kernelStats[perfMap][cpu][eventType].Lost.Swap(value)
}

// GetEventStats returns the number of received events of the specified type (only used in tests)
func (pbm *Monitor) GetEventStats(eventType model.EventType, perfMap string, cpu int) (EventStats, MapStats) {
	eventStats, kernelStats := makeEventStats(), makeMapStats()
	var maps []string

	if eventType >= model.MaxKernelEventType {
		return eventStats, kernelStats
	}

	switch {
	case len(perfMap) == 0:
		for m := range pbm.eventStats {
			maps = append(maps, m)
		}
	case pbm.eventStats[perfMap] != nil:
		maps = append(maps, perfMap)
	}

	for _, m := range maps {

		switch {
		case cpu == -1:
			for i := range pbm.eventStats[m] {
				eventStats.Count.Add(pbm.getEventCount(eventType, perfMap, i))
				eventStats.Bytes.Add(pbm.getEventBytes(eventType, perfMap, i))

				kernelStats.Count.Add(pbm.getKernelEventCount(eventType, perfMap, i))
				kernelStats.Bytes.Add(pbm.getKernelEventBytes(eventType, perfMap, i))
				kernelStats.Lost.Add(pbm.getKernelLostCount(eventType, perfMap, i))
			}
		case cpu >= 0 && pbm.numCPU > cpu:
			eventStats.Count.Add(pbm.getEventCount(eventType, perfMap, cpu))
			eventStats.Bytes.Add(pbm.getEventBytes(eventType, perfMap, cpu))

			kernelStats.Count.Add(pbm.getKernelEventCount(eventType, perfMap, cpu))
			kernelStats.Bytes.Add(pbm.getKernelEventBytes(eventType, perfMap, cpu))
			kernelStats.Lost.Add(pbm.getKernelLostCount(eventType, perfMap, cpu))
		}

	}
	return eventStats, kernelStats
}

// getAndResetEventCount is an internal function, it can segfault if its parameters are incorrect.
func (pbm *Monitor) getAndResetEventStats(eventType model.EventType, perfMap string, cpu int) (uint64, uint64, uint64, uint64) {
	stats := pbm.eventStats[perfMap][cpu][eventType]
	return stats.Count.Swap(0), stats.Bytes.Swap(0), stats.ADSaved.Swap(0), stats.SortingError.Swap(0)
}

// CountLostEvent adds `count` to the counter of lost events
func (pbm *Monitor) CountLostEvent(count uint64, mapName string, cpu int) {
	// sanity check
	if (pbm.readLostEvents[mapName] == nil) || (len(pbm.readLostEvents[mapName]) <= cpu) {
		return
	}
	pbm.readLostEvents[mapName][cpu].Add(count)
}

// CountEvent adds `count` to the counter of received events of the specified type
func (pbm *Monitor) CountEvent(eventType model.EventType, event *model.Event, size uint64, cpu int, checkOrder bool) {
	const mapName = EventStreamMap

	// sanity check
	if (pbm.eventStats[mapName] == nil) || (len(pbm.eventStats[mapName]) <= cpu) || (len(pbm.eventStats[mapName][cpu]) <= int(eventType)) {
		return
	}

	stats := pbm.eventStats[mapName][cpu][eventType]

	stats.Count.Inc()
	stats.Bytes.Add(size)

	if event.IsSavedByActivityDumps() {
		stats.ADSaved.Inc()
	}

	if checkOrder {
		timestamp := event.TimestampRaw

		// check event order
		if timestamp < pbm.lastTimestamp && pbm.lastTimestamp != 0 {
			stats.SortingError.Inc()
		} else {
			pbm.lastTimestamp = timestamp
		}
	}
}

// CountInvalidEvent counts the size of one invalid event of the specified cause
func (pbm *Monitor) CountInvalidEvent(size uint64) {
	const (
		mapName = EventStreamMap
		cause   = InvalidType
	)

	// sanity check
	if len(pbm.invalidEventStats[mapName]) <= int(cause) {
		return
	}
	pbm.invalidEventStats[mapName][cause].Count.Add(1)
	pbm.invalidEventStats[mapName][cause].Bytes.Add(size)
}

func (pbm *Monitor) sendEventsAndBytesReadStats(client statsd.ClientInterface) error {
	// cardinality, map, event_type
	tags := []string{pbm.config.StatsTagsCardinality, "", "", ""}

	for m := range pbm.eventStats {
		tags[1] = fmt.Sprintf("map:%s", m)
		for cpu := range pbm.eventStats[m] {
			for eventType := range pbm.eventStats[m][cpu] {
				evtType := model.EventType(eventType)
				tags[2], tags[3] = fmt.Sprintf("event_type:%s", evtType), ""

				count, bytes, adSaved, sortingError := pbm.getAndResetEventStats(evtType, m, cpu)

				if bytes > 0 {
					if err := client.Count(metrics.MetricPerfBufferBytesRead, int64(bytes), tags, 1.0); err != nil {
						return err
					}
				}

				if sortingError > 0 {
					if err := pbm.statsdClient.Count(metrics.MetricPerfBufferSortingError, int64(sortingError), tags, 1.0); err != nil {
						return err
					}
				}

				// event not saved by activity dumps
				if adSaved >= count {
					count = 0
				}

				if count > 0 {
					if err := client.Count(metrics.MetricPerfBufferEventsRead, int64(count), tags, 1.0); err != nil {
						return err
					}
				}

				if adSaved > 0 {
					tags[3] = "cause:activity_dump"
					if err := client.Count(metrics.MetricPerfBufferEventsRead, int64(adSaved), tags, 1.0); err != nil {
						return err
					}
				}
			}
		}
	}

	for mapName, causes := range pbm.invalidEventStats {
		for cause, stats := range causes {
			count, bytes := stats.getAndReset()
			tags := []string{fmt.Sprintf("map:%s", mapName), fmt.Sprintf("cause:%s", InvalidEventCause(cause).String())}
			if count > 0 {
				if err := client.Count(metrics.MetricPerfBufferInvalidEventsCount, int64(count), tags, 1.0); err != nil {
					return err
				}
			}
			if bytes > 0 {
				if err := client.Count(metrics.MetricPerfBufferInvalidEventsBytes, int64(bytes), tags, 1.0); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (pbm *Monitor) sendLostEventsReadStats(client statsd.ClientInterface) error {
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
	}
	return nil
}

func (pbm *Monitor) getRingbufUsage(statsMap *statMap) (uint64, error) {
	req := erpc.NewERPCRequest(erpc.GetRingbufUsage)
	if err := pbm.eRPC.Request(req); err != nil {
		return 0, err
	}

	var ringUsage uint64
	if err := statsMap.ebpfRingBufferMap.Lookup(int32(0), &ringUsage); err != nil {
		return 0, fmt.Errorf("failed to retrieve ring buffer usage")
	}

	return ringUsage, nil
}

func (pbm *Monitor) collectAndSendKernelStats(client statsd.ClientInterface) error {
	var (
		id       uint32
		iterator *lib.MapIterator
		tmpCount uint64
	)

	perCPUMapStats := make([]MapStats, pbm.numCPU)
	for i := 0; i < pbm.numCPU; i++ {
		perCPUMapStats[i] = makeMapStats()
	}

	// cardinality, map, event_type, category
	tags := []string{pbm.config.StatsTagsCardinality, "", "", ""}

	// loop through the statistics buffers of each perf map
	for perfMapName, statsMap := range pbm.perfBufferStatsMaps {
		// total and perEvent are used for alerting
		var total uint64
		perEvent := map[string]uint64{}
		mapNameTag := fmt.Sprintf("map:%s", perfMapName)
		tags[1] = mapNameTag

		// loop through all the values of the active buffer
		iterator = statsMap.ebpfMap.Iterate()
		for iterator.Next(&id, &perCPUMapStats) {
			if id == 0 {
				// first event type is 1
				continue
			}

			// retrieve event type from key
			evtType := model.EventType(id % uint32(model.MaxKernelEventType))
			tags[2] = fmt.Sprintf("event_type:%s", evtType)
			tags[3] = fmt.Sprintf("category:%s", model.GetEventTypeCategory(evtType.String()))

			// loop over each cpu entry
			for cpu, stats := range perCPUMapStats {
				// sanity checks:
				//   - check if the computed cpu id is below the current cpu count
				//   - check if we collect some data on the provided perf map
				//   - check if the computed event id is below the current max event id
				if (pbm.kernelStats[perfMapName] == nil) || (len(pbm.kernelStats[perfMapName]) <= cpu) || (len(pbm.kernelStats[perfMapName][cpu]) <= int(evtType)) {
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
			if pbm.onEventLost != nil {
				pbm.onEventLost(perfMapName, perEvent)
			}
		}
	}
	return nil
}

func (pbm *Monitor) sendKernelStats(client statsd.ClientInterface, stats MapStats, tags []string) error {
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
func (pbm *Monitor) SendStats() error {
	if err := pbm.collectAndSendKernelStats(pbm.statsdClient); err != nil {
		return err
	}

	if err := pbm.sendEventsAndBytesReadStats(pbm.statsdClient); err != nil {
		return err
	}

	return pbm.sendLostEventsReadStats(pbm.statsdClient)
}
