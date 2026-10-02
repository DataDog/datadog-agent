// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PipelineOpts are the optional dependencies of a Pipeline
type PipelineOpts struct {
	// Compiler compiles the rules. Defaults to DefaultCompiler(), the engine of this build.
	Compiler Compiler
	// ContainerPIDs is passed to the FileReader, to open files through the other processes of a
	// container. May be nil.
	ContainerPIDs ContainerPIDsFunc
	// MetricsInterval is the metrics flush interval. Zero means the Metrics default.
	MetricsInterval time.Duration
	// ExtraReporter, when set, is called for every report after the structured log line. It must
	// be safe for concurrent use (e.g. a test recorder).
	ExtraReporter Reporter
	// ExtraReporterFor, when set, is called once at build time with the loaded rules version to
	// build an extra reporter that needs it (e.g. the CWS BackendReporter). It may return nil,
	// and is wired after ExtraReporter. The rules version isn't known until the scanner is
	// loaded, which is why this is a factory rather than a plain reporter.
	ExtraReporterFor func(rulesVersion string) Reporter
}

// Pipeline is the whole YARA exec scanner, wired together:
//
//	ExecConsumer → FileReader.Process → Pool → StructuredReporter (+ ExtraReporter)
//
// with Metrics reading the Stats every stage writes, and the scan duration hook installed. The
// consumer's Start and Stop start and stop the pipeline, so registering the consumer with the
// event monitor is all it takes to run it.
type Pipeline struct {
	stats        *Stats
	deduper      *LRUDeduper
	scanner      Scanner
	rulesVersion string
	pool         *Pool
	reader       *FileReader
	metrics      *Metrics
	consumer     *ExecConsumer

	mu      sync.Mutex
	started bool
	stopped bool
}

// NewPipeline builds the pipeline from cfg, and sends its metrics to client. It loads and
// compiles the rules of cfg.RulesDir: it fails when they can't be loaded (unset or empty
// directory, unsafe permissions, compile error), or when cfg is invalid. Nothing runs until
// Start (or the consumer's Start), so a failed or discarded Pipeline leaks no goroutine.
func NewPipeline(cfg *Config, client statsd.ClientInterface, opts PipelineOpts) (*Pipeline, error) {
	if cfg == nil {
		return nil, errors.New("yara: nil config")
	}
	if client == nil {
		client = &statsd.NoOpClient{}
	}
	compile := opts.Compiler
	if compile == nil {
		compile = DefaultCompiler()
	}

	scanner, rulesVersion, err := LoadScanner(cfg.RulesDir, compile)
	if err != nil {
		return nil, err
	}

	p, err := newPipeline(cfg, client, opts, scanner, rulesVersion)
	if err != nil {
		closeScanner(scanner)
		return nil, err
	}
	return p, nil
}

func newPipeline(cfg *Config, client statsd.ClientInterface, opts PipelineOpts, scanner Scanner, rulesVersion string) (*Pipeline, error) {
	// the hook must be installed before any stage runs
	stats := &Stats{ObserveScanDuration: NewScanDurationObserver(client, nil)}

	deduper, err := NewLRUDeduper(cfg.IdentityCacheSize, DefaultHashSetSize, cfg.RecheckTTL)
	if err != nil {
		return nil, fmt.Errorf("yara: invalid identity_cache_size %d: %w", cfg.IdentityCacheSize, err)
	}

	reporters := []Reporter{NewStructuredReporter(rulesVersion, StructuredReporterOptions{})}
	if opts.ExtraReporter != nil {
		reporters = append(reporters, opts.ExtraReporter)
	}
	if opts.ExtraReporterFor != nil {
		if r := opts.ExtraReporterFor(rulesVersion); r != nil {
			reporters = append(reporters, r)
		}
	}
	reporter := reporters[0]
	if len(reporters) > 1 {
		reporter = teeReporter(reporters)
	}

	pool, err := NewPool(PoolConfig{
		Workers:     cfg.Workers,
		QueueSize:   cfg.QueueSize,
		ScanTimeout: cfg.ScanTimeout,
		MaxFileSize: cfg.MaxFileSize,
	}, scanner, deduper, reporter, stats)
	if err != nil {
		return nil, err
	}

	reader := NewFileReader(deduper, pool, stats, FileReaderOpts{
		MaxFileSize:   cfg.MaxFileSize,
		ContainerPIDs: opts.ContainerPIDs,
	})

	p := &Pipeline{
		stats:        stats,
		deduper:      deduper,
		scanner:      scanner,
		rulesVersion: rulesVersion,
		pool:         pool,
		reader:       reader,
		metrics:      NewMetrics(client, stats, deduper, pool, MetricsOptions{Interval: opts.MetricsInterval}),
	}

	p.consumer, err = newExecConsumer(cfg, stats, reader.Process)
	if err != nil {
		return nil, err
	}
	p.consumer.lifecycle = p
	return p, nil
}

// Consumer returns the exec consumer feeding the pipeline. Its Start and Stop start and stop the
// pipeline.
func (p *Pipeline) Consumer() *ExecConsumer {
	return p.consumer
}

// Stats returns the pipeline counters
func (p *Pipeline) Stats() *Stats {
	return p.stats
}

// RulesVersion returns the version of the loaded rules
func (p *Pipeline) RulesVersion() string {
	return p.rulesVersion
}

// Start starts the metrics flush and the scan workers. It is a no-op when already started or
// stopped. It never fails; the error is for the eventmonitor.EventConsumer interface.
func (p *Pipeline) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.stopped {
		return nil
	}
	p.started = true

	p.metrics.Start(context.Background())
	p.pool.Start()
	return nil
}

// Stop stops the scan workers (cancelling the scans in flight and dropping the queued ones), then
// the metrics with a last flush, then frees the compiled rules. It is safe to call more than
// once, and without Start. The pipeline can't be restarted.
//
// Files handed to the pipeline after Stop are still read and hashed, but their scan is dropped:
// callers should stop feeding it first. The event monitor does, by stopping the probe (and its
// consumer goroutines) before calling the consumers' Stop.
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true

	// the pool first: its workers report and count until they exit, and the last metrics flush
	// must see those counts
	p.pool.Stop()
	p.metrics.Stop()
	// no scan is running anymore
	closeScanner(p.scanner)
}

// closeScanner frees the compiled rules of scanner if it holds any
func closeScanner(scanner Scanner) {
	if c, ok := scanner.(io.Closer); ok {
		if err := c.Close(); err != nil {
			log.Warnf("yara: failed to free the compiled rules: %v", err)
		}
	}
}

// teeReporter sends every report to each of its reporters, in order
type teeReporter []Reporter

// Report implements Reporter
func (t teeReporter) Report(f ExecFile, sum [32]byte, matches []Match, err error) {
	for _, r := range t {
		r.Report(f, sum, matches, err)
	}
}

// NewExecScanner builds the pipeline for the event monitor evm, and registers its consumer.
// Metrics go to the event monitor's statsd client, and the container PIDs used to open files
// come from the CWS cgroup resolver when the eBPF probe is running.
//
// On error nothing is registered, and nothing is left running. The caller must log the error
// and carry on without the YARA scanner: it must never prevent system-probe from starting.
func NewExecScanner(evm *eventmonitor.EventMonitor, cfg *Config) (*Pipeline, error) {
	p, err := NewPipeline(cfg, evm.StatsdClient, PipelineOpts{
		ContainerPIDs: containerPIDsFromEventMonitor(evm),
		// Report matches to the CWS backend in addition to the structured log line. The factory
		// returns nil (log-only) when the probe can't build the serializer; a dispatch when CWS
		// is disabled is a silent no-op, so the log line always stands on its own.
		ExtraReporterFor: func(rulesVersion string) Reporter {
			// typed return so a nil *BackendReporter isn't wrapped into a non-nil interface
			if r := NewBackendReporter(evm, rulesVersion); r != nil {
				return r
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	if err := p.consumer.register(evm); err != nil {
		p.Stop()
		return nil, err
	}
	return p, nil
}

// containerPIDsFromEventMonitor returns a ContainerPIDsFunc backed by the cgroup resolver of the
// eBPF probe of evm, or nil when there is none (e.g. ebpfless mode)
func containerPIDsFromEventMonitor(evm *eventmonitor.EventMonitor) ContainerPIDsFunc {
	if evm == nil || evm.Probe == nil {
		return nil
	}
	ebpfProbe, ok := evm.Probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok || ebpfProbe.Resolvers == nil || ebpfProbe.Resolvers.CGroupResolver == nil {
		return nil
	}
	resolver := ebpfProbe.Resolvers.CGroupResolver
	return func(id containerutils.ContainerID) []uint32 {
		entry := resolver.GetCacheEntryContainerID(id)
		if entry == nil {
			return nil
		}
		return entry.GetPIDs()
	}
}
