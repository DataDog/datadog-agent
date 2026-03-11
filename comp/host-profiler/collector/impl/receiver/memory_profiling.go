//go:build linux

package receiver

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/ebpf-profiler/tracer"
	"go.uber.org/zap"
)

type memoryProfiler struct {
	config   MemoryProfilingConfig
	logger   *zap.Logger
	tracerCh <-chan *tracer.Tracer
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func newMemoryProfiler(config MemoryProfilingConfig, logger *zap.Logger, tracerCh <-chan *tracer.Tracer, cancel context.CancelFunc) *memoryProfiler {
	return &memoryProfiler{
		config:   config,
		logger:   logger,
		tracerCh: tracerCh,
		cancel:   cancel,
	}
}

// Start must be called as a goroutine; caller must call wg.Add(1) first.
func (mp *memoryProfiler) Start(ctx context.Context) {
	defer mp.wg.Done()

	var trc *tracer.Tracer
	select {
	case trc = <-mp.tracerCh:
		mp.logger.Info("Memory profiler received tracer")
	case <-ctx.Done():
		mp.logger.Info("Memory profiler stopped before receiving tracer")
		return
	}

	// New processes are handled via SetPIDNewCallback registered in factory.go.
	if err := mp.attachToProcesses(ctx, trc); err != nil {
		mp.logger.Error("Initial attach to processes failed", zap.Error(err))
	}

	mp.logStats(ctx, trc)
}

func (mp *memoryProfiler) Shutdown() error {
	mp.logger.Info("Shutting down memory profiler")
	mp.cancel()
	mp.wg.Wait()
	return nil
}

func (mp *memoryProfiler) attachToProcesses(ctx context.Context, trc *tracer.Tracer) error {
	pids := trc.GetTrackedPIDs()

	successCount := 0
	skippedCount := 0

	for _, pid := range pids {
		select {
		case <-ctx.Done():
			mp.logger.Info("attachToProcesses cancelled",
				zap.Int("processed", successCount+skippedCount),
				zap.Int("total", len(pids)))
			return ctx.Err()
		default:
		}

		if !shouldProfileProcess(pid, mp.config) {
			skippedCount++
			continue
		}

		if err := trc.AttachMemoryProfilingForPID(pid); err != nil {
			mp.logger.Warn("Failed to attach memory profiling to process",
				zap.Int("pid", pid),
				zap.Error(err))
			skippedCount++
			continue
		}

		successCount++
	}

	mp.logger.Info("Memory profiling initial attach summary",
		zap.Int("success", successCount),
		zap.Int("skipped", skippedCount),
		zap.Int("total", len(pids)))

	return nil
}

func (mp *memoryProfiler) logStats(ctx context.Context, trc *tracer.Tracer) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := trc.ReadMemoryStatistics()
			if err != nil {
				mp.logger.Error("Failed to read memory statistics", zap.Error(err))
				continue
			}

			if stats.MallocCount > 0 {
				mp.logger.Info("Memory profiling statistics",
					zap.Int64("allocated_mb", int64(stats.TotalAllocatedBytes/(1024*1024))),
					zap.Int64("freed_mb", int64(stats.TotalFreedBytes/(1024*1024))),
					zap.Int64("net_growth_mb", stats.NetGrowth/(1024*1024)),
					zap.Uint64("malloc_count", stats.MallocCount),
					zap.Uint64("free_count", stats.FreeCount),
					zap.Uint64("realloc_count", stats.ReallocCount))
			}
		case <-ctx.Done():
			return
		}
	}
}
