// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package processmanager manages the loading and unloading of information related to processes.
package processmanager

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/internal/log"
	lru "github.com/elastic/go-freelru"

	eim "github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/processmanager/execinfomanager"
	"go.opentelemetry.io/ebpf-profiler/host"
	"go.opentelemetry.io/ebpf-profiler/interpreter"
	"go.opentelemetry.io/ebpf-profiler/interpreter/apmint"
	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfunsafe"
	"go.opentelemetry.io/ebpf-profiler/lpm"
	"go.opentelemetry.io/ebpf-profiler/metrics"
	"go.opentelemetry.io/ebpf-profiler/nativeunwind"
	"go.opentelemetry.io/ebpf-profiler/periodiccaller"
	pmebpf "go.opentelemetry.io/ebpf-profiler/processmanager/ebpfapi"
	"go.opentelemetry.io/ebpf-profiler/reporter"
	"go.opentelemetry.io/ebpf-profiler/reporter/samples"
	"go.opentelemetry.io/ebpf-profiler/times"
	"go.opentelemetry.io/ebpf-profiler/traceutil"
	"go.opentelemetry.io/ebpf-profiler/util"

	tracertypes "github.com/DataDog/datadog-agent/comp/host-profiler/ebpfprofiler/tracer/types"
)

const (
	// Maximum size of the LRU cache holding the executables' ELF information.
	elfInfoCacheSize = 16384

	// TTL of entries in the LRU cache holding the executables' ELF information.
	elfInfoCacheTTL = 6 * time.Hour

	// Maximum size of the LRU cache for frames.
	frameCacheSize = 16384

	// TTL of entries in the frame cache.
	frameCacheLifetime = 5 * time.Minute
)

// dummyPrefix is the LPM prefix installed to indicate the process is known
var dummyPrefix = lpm.Prefix{Key: 0, Length: 64}

var (
	errSymbolizationNotSupported = errors.New("symbolization not supported")
	// errPIDGone indicates that a process is no longer managed by the process manager.
	errPIDGone = errors.New("interpreter process gone")
)

// New creates a new ProcessManager which is responsible for keeping track of loading
// and unloading of symbols for processes.
func New(ctx context.Context, includeTracers tracertypes.IncludedTracers, monitorInterval time.Duration,
	executableUnloadDelay time.Duration, ebpf pmebpf.EbpfHandler, traceReporter reporter.TraceReporter,
	exeReporter reporter.ExecutableReporter, sdp nativeunwind.StackDeltaProvider,
	filterErrorFrames bool, includeEnvVars libpf.Set[string]) (*ProcessManager, error) {
	if exeReporter == nil {
		exeReporter = executableReporterStub{}
	}

	elfInfoCache, err := lru.New[util.OnDiskFileIdentifier, elfInfo](elfInfoCacheSize,
		util.OnDiskFileIdentifier.Hash32)
	if err != nil {
		return nil, fmt.Errorf("unable to create elfInfoCache: %v", err)
	}
	elfInfoCache.SetLifetime(elfInfoCacheTTL)

	frameCache, err := lru.New[frameCacheKey, libpf.Frames](frameCacheSize, hashFrameCacheKey)
	if err != nil {
		return nil, err
	}
	frameCache.SetLifetime(frameCacheLifetime)

	em, err := eim.NewExecutableInfoManager(sdp, ebpf, includeTracers)
	if err != nil {
		return nil, fmt.Errorf("unable to create ExecutableInfoManager: %v", err)
	}

	periodiccaller.Start(ctx, executableUnloadDelay, func() {
		err := em.CleanupUnused(executableUnloadDelay)
		if err != nil {
			log.Errorf("Failed to cleanup unused executables: %v", err)
		}
	})

	interpreters := make(map[libpf.PID]map[util.OnDiskFileIdentifier]interpreter.Instance)

	pm := &ProcessManager{
		interpreterTracerEnabled: em.NumInterpreterLoaders() > 0,
		eim:                      em,
		interpreters:             interpreters,
		exitEvents:               make(map[libpf.PID]times.KTime),
		pidToProcessInfo:         make(map[libpf.PID]*processInfo),
		ebpf:                     ebpf,
		elfInfoCache:             elfInfoCache,
		frameCache:               frameCache,
		traceReporter:            traceReporter,
		exeReporter:              exeReporter,
		metricsAddSlice:          metrics.AddSlice,
		filterErrorFrames:        filterErrorFrames,
		includeEnvVars:           includeEnvVars,
	}

	collectInterpreterMetrics(ctx, pm, monitorInterval)

	return pm, nil
}

// metricSummaryToSlice creates a metrics.Metric slice from a map of metric IDs to values.
func metricSummaryToSlice(summary metrics.Summary) []metrics.Metric {
	result := make([]metrics.Metric, 0, len(summary))
	for mID, mVal := range summary {
		result = append(result, metrics.Metric{ID: mID, Value: mVal})
	}
	return result
}

// updateMetricSummary gets the metrics from the provided interpreter instance and updates the
// provided summary by aggregating the new metrics into the summary.
// The caller is responsible to hold the lock on the interpreter.Instance to avoid race conditions.
func updateMetricSummary(ii interpreter.Instance, summary metrics.Summary) error {
	instanceMetrics, err := ii.GetAndResetMetrics()
	// Update metrics even if there was an error, because it's possible ii is a MultiInstance
	// and some of the instances may have returned metrics.
	for _, metric := range instanceMetrics {
		summary[metric.ID] += metric.Value
	}

	return err
}

// collectInterpreterMetrics starts a goroutine that periodically fetches and reports interpreter
// metrics.
func collectInterpreterMetrics(ctx context.Context, pm *ProcessManager,
	monitorInterval time.Duration,
) {
	periodiccaller.Start(ctx, monitorInterval, func() {
		pm.mu.RLock()
		defer pm.mu.RUnlock()

		summary := make(map[metrics.MetricID]metrics.MetricValue)

		for pid := range pm.interpreters {
			for addr, ii := range pm.interpreters[pid] {
				if err := updateMetricSummary(ii, summary); err != nil {
					log.Errorf("Failed to get/reset metrics for PID %d at 0x%x: %v",
						pid, addr, err)
				}
			}
		}

		summary[metrics.IDHashmapPidPageToMappingInfo] = metrics.MetricValue(pm.pidPageToMappingInfoSize)

		summary[metrics.IDELFInfoCacheHit] = metrics.MetricValue(pm.elfInfoCacheHit.Swap(0))
		summary[metrics.IDELFInfoCacheMiss] = metrics.MetricValue(pm.elfInfoCacheMiss.Swap(0))

		summary[metrics.IDTraceCacheHit] = metrics.MetricValue(pm.frameCacheHit.Swap(0))
		summary[metrics.IDTraceCacheMiss] = metrics.MetricValue(pm.frameCacheMiss.Swap(0))

		summary[metrics.IDErrProcNotExist] = metrics.MetricValue(pm.mappingStats.errProcNotExist.Swap(0))
		summary[metrics.IDErrProcESRCH] = metrics.MetricValue(pm.mappingStats.errProcESRCH.Swap(0))
		summary[metrics.IDErrProcPerm] = metrics.MetricValue(pm.mappingStats.errProcPerm.Swap(0))
		summary[metrics.IDNumProcAttempts] = metrics.MetricValue(pm.mappingStats.numProcAttempts.Swap(0))
		summary[metrics.IDMaxProcParseUsec] = metrics.MetricValue(pm.mappingStats.maxProcParseUsec.Swap(0))
		summary[metrics.IDTotalProcParseUsec] = metrics.MetricValue(pm.mappingStats.totalProcParseUsec.Swap(0))
		summary[metrics.IDErrProcParse] = metrics.MetricValue(pm.mappingStats.numProcParseErrors.Swap(0))

		mapsMetrics := pm.ebpf.CollectMetrics()
		for _, metric := range mapsMetrics {
			summary[metric.ID] = metric.Value
		}

		pm.eim.UpdateMetricSummary(summary)
		pm.metricsAddSlice(metricSummaryToSlice(summary))
	})
}

func (pm *ProcessManager) Close() {
}

func (pm *ProcessManager) symbolizeFrame(pid libpf.PID, data []uint64, frames *libpf.Frames, mapping libpf.FrameMapping) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.interpreters[pid]) == 0 {
		return errPIDGone
	}

	for _, instance := range pm.interpreters[pid] {
		if err := instance.Symbolize(data, frames, mapping); err != nil {
			if errors.Is(err, interpreter.ErrMismatchInterpreterType) {
				// The interpreter type of instance did not match the type of frame.
				// So continue with the next interpreter instance for this PID.
				continue
			}
			return err
		}
		return nil
	}

	return fmt.Errorf("no matching interpreter instance (of len %d): %w",
		len(pm.interpreters[pid]), errSymbolizationNotSupported)
}

// convertFrame converts one host Frame to one or more libpf.Frames. It returns true
// if non-trivial cacheable conversion was done.
func (pm *ProcessManager) convertFrame(pid libpf.PID, ef libpf.EbpfFrame, dst *libpf.Frames) bool {
	switch ef.Type().Interpreter() {
	case libpf.UnknownInterp, libpf.Kernel:
		log.Errorf("Unexpected frame type 0x%02X (neither error nor usermode frame)",
			uint8(ef.Type()))
	case libpf.Native:
		fileID := host.FileID(ef.Variable(0))
		address := libpf.Address(ef.Data())

		// Locate mapping info for the frame.
		mapping := pm.findMappingForTrace(pid, fileID, address)

		// Attempt symbolization of native frames. It is best effort and
		// provides non-symbolized frames if no native symbolizer is active.
		if err := pm.symbolizeFrame(pid, ef, dst, mapping); err == nil {
			return true
		}

		// The BPF code classifies whether an address is a return address or not.
		// Return addresses are where execution resumes when returning to the stack
		// frame and point to the **next instruction** after the call instruction
		// that produced the frame.
		//
		// For these return addresses we subtract 1 from the address in order to
		// make it point into the call that precedes it: the instruction at the
		// return address may already be part of whatever code follows after the
		// call, and we want the return addresses to resolve to the call itself
		// during symbolization.
		//
		// Optimally we'd subtract the size of the call instruction here instead
		// of doing `- 1`, but disassembling backwards is quite difficult for
		// variable length instruction sets like X86.
		if ef.Flags().ReturnAddress() {
			address--
		}
		dst.Append(&libpf.Frame{
			Type:            ef.Type(),
			AddressOrLineno: libpf.AddressOrLineno(address),
			Mapping:         mapping,
		})
	default:
		err := pm.symbolizeFrame(pid, ef, dst, libpf.FrameMapping{})
		if err == nil {
			return true
		}
		log.Debugf("symbolization failed for PID %d, frame type %d: %v",
			pid, ef.Type(), err)
		dst.Append(&libpf.Frame{Type: ef.Type()})
	}
	return false
}

func (pm *ProcessManager) maybeNotifyAPMAgent(
	rawTrace *libpf.EbpfTrace, umTraceHash libpf.TraceHash, count uint16,
) string {
	pm.mu.RLock()
	// Keeping the lock until end of the function is needed because inner map can be modified
	// concurrently (by synchronizeMappings/newFrameMapping).
	defer pm.mu.RUnlock()
	pidInterp, ok := pm.interpreters[rawTrace.PID]
	if !ok {
		return ""
	}
	var serviceName string
	for _, mapping := range pidInterp {
		if apm, ok := mapping.(*apmint.Instance); ok {
			apm.NotifyAPMAgent(rawTrace.PID, rawTrace, umTraceHash, count)
			if serviceName != "" {
				log.Warnf("Overwriting APM service name from '%s' to '%s' for PID %d",
					serviceName,
					apm.APMServiceName(),
					rawTrace.PID)
			}
			// It's pretty unusual to have more than one APM agent in a
			// single process, but in case there is, just pick the last one.
			serviceName = apm.APMServiceName()
		}
	}

	return serviceName
}

func hashFrameCacheKey(fk frameCacheKey) uint32 {
	h := fnv.New32a()
	h.Write(pfunsafe.FromSlice(fk.data[:]))
	return h.Sum32()
}

// HandleTrace processes and reports the given host.Trace. This function
// is not re-entrant due to frameCache not being synced. If the tracer is
// later updated to distribute trace handling to goroutine pool, the caching
// strategy needs to be updated accordingly.
func (pm *ProcessManager) HandleTrace(bpfTrace *libpf.EbpfTrace) {
	meta := &samples.TraceEventMeta{
		Timestamp:      libpf.UnixTime64(times.KTime(bpfTrace.KTime).UnixNano()),
		Comm:           bpfTrace.Comm,
		PID:            bpfTrace.PID,
		TID:            bpfTrace.TID,
		APMServiceName: "", // filled in below
		CPU:            bpfTrace.CPU,
		ProcessName:    bpfTrace.ProcessName,
		ExecutablePath: bpfTrace.ExecutablePath,
		ContainerID:    bpfTrace.ContainerID,
		Origin:         bpfTrace.Origin,
		OffTime:        bpfTrace.OffTime,
		EnvVars:        bpfTrace.EnvVars,
	}

	pid := bpfTrace.PID
	kernelFramesLen := len(bpfTrace.KernelFrames)
	trace := &libpf.Trace{
		Frames:       make(libpf.Frames, kernelFramesLen, kernelFramesLen+bpfTrace.NumFrames),
		CustomLabels: bpfTrace.CustomLabels,
	}
	copy(trace.Frames, bpfTrace.KernelFrames)

	cacheMiss := uint64(0)
	cacheHit := uint64(0)

	for frames := libpf.EbpfFrame(bpfTrace.FrameData); len(frames) > 0; frames = frames[frames.Length():] {
		frame := frames[:frames.Length()]
		if frame.Flags().Error() {
			if !pm.filterErrorFrames {
				trace.Frames.Append(&libpf.Frame{
					Type:            frame.Type().Error(),
					AddressOrLineno: libpf.AddressOrLineno(frame.Data()),
				})
			}
			continue
		}

		oldLen := len(trace.Frames)
		key := frameCacheKey{}
		if frame.Flags().PIDSpecific() {
			key.pid = pid
		}
		copy(key.data[:], frame)
		if cached, ok := pm.frameCache.GetAndRefresh(key, frameCacheLifetime); ok {
			// Fast path
			cacheHit++
			trace.Frames = append(trace.Frames, cached...)
		} else {
			// Slow path: convert trace.
			if pm.convertFrame(pid, frame, &trace.Frames) {
				cacheMiss++
				pm.frameCache.Add(key, slices.Clone(trace.Frames[oldLen:len(trace.Frames)]))
			}
		}
	}
	if cacheMiss != 0 {
		pm.frameCacheMiss.Add(cacheMiss)
	}
	if cacheHit != 0 {
		pm.frameCacheHit.Add(cacheHit)
	}
	pm.mu.RLock()
	// Release resources that were used to symbolize this stack.
	for _, instance := range pm.interpreters[pid] {
		if err := instance.ReleaseResources(); err != nil {
			log.Warnf("Failed to release resources for %d: %v", pid, err)
		}
	}
	pm.mu.RUnlock()

	trace.Hash = traceutil.HashTrace(trace)
	meta.APMServiceName = pm.maybeNotifyAPMAgent(bpfTrace, trace.Hash, 1)

	if err := pm.traceReporter.ReportTraceEvent(trace, meta); err != nil {
		log.Errorf("Failed to report trace event: %v", err)
	}
}
