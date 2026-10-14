// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package jvmmemory

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory/model"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/ebpf/usdt"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	moduleName = "jvm_memory"

	poolsMapName = "jvm_pools"
	gcMapName    = "jvm_gc"

	// mapCleanerBatchSize is the batch size of the iteration and deletion of
	// the entries of the exited JVMs.
	mapCleanerBatchSize = 100

	// pidNamespaceInoConstant is the constant of the eBPF program holding the
	// PID namespace in which the tgids are reported, see pidNamespaceConstant.
	pidNamespaceInoConstant = "jvm_pidns_ino"

	// defaultScanInterval is the period of the procfs scan that discovers the
	// JVMs and reaps the exited ones.
	defaultScanInterval = 10 * time.Second
)

// Minimum kernel versions to attach uprobes to the USDT probe sites and to
// load the CO-RE program: the ones of the uprobes of USM on x86-64 and arm64.
var (
	minKernelVersion      = kernel.VersionCode(4, 14, 0)
	minKernelVersionARM64 = kernel.VersionCode(5, 5, 0)
)

// Probe is the eBPF side of the jvm_memory check.
type Probe struct {
	procRoot     string
	scanInterval time.Duration

	mgr          *ddebpf.Manager
	pools        *maps.GenericMap[ebpfPoolKey, ebpfPoolValue]
	gcStats      *maps.GenericMap[uint32, ebpfGCStats]
	poolsCleaner *ddebpf.MapCleaner[ebpfPoolKey, ebpfPoolValue]
	gcCleaner    *ddebpf.MapCleaner[uint32, ebpfGCStats]
	attacher     *usdtAttacher
	registry     *utils.FileRegistry
	scanner      *processScanner
	cids         *containerIDCache
	// jvms is the number of JVMs with attached probes, at the last scan.
	jvms atomic.Int64

	stop      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// probeOptions are the options of newProbe, overridden by the tests.
type probeOptions struct {
	procRoot     string
	scanInterval time.Duration
	// pidNamespaceIno overrides the value of the jvm_pidns_ino constant when
	// not nil.
	pidNamespaceIno *uint64
}

// NewProbe creates a [Probe] and starts discovering the JVMs.
func NewProbe(cfg *ddebpf.Config) (*Probe, error) {
	procRoot := cfg.ProcRoot
	if procRoot == "" {
		procRoot = kernel.ProcFSRoot()
	}
	return newProbe(cfg, probeOptions{procRoot: procRoot, scanInterval: defaultScanInterval})
}

func newProbe(cfg *ddebpf.Config, opts probeOptions) (*Probe, error) {
	if err := checkKernelVersion(); err != nil {
		return nil, err
	}
	hostMachine, ok := usdt.HostMachine()
	if !ok {
		return nil, fmt.Errorf("unsupported architecture %s", runtime.GOARCH)
	}

	var pidNamespaceIno uint64
	if opts.pidNamespaceIno != nil {
		pidNamespaceIno = *opts.pidNamespaceIno
	} else {
		var err error
		if pidNamespaceIno, err = pidNamespaceConstant(opts.procRoot); err != nil {
			// Correct whenever system-probe sees the root PID namespace,
			// which is the case of every regular deployment.
			log.Warnf("jvm_memory: cannot determine the PID namespace of %s, assuming the root one: %s", opts.procRoot, err)
		}
	}

	p := &Probe{
		procRoot:     opts.procRoot,
		scanInterval: opts.scanInterval,
		cids:         newContainerIDCache(opts.procRoot),
		stop:         make(chan struct{}),
	}

	filename := "jvm-memory.o"
	if cfg.BPFDebug {
		filename = "jvm-memory-debug.o"
	}
	err := ddebpf.LoadCOREAsset(filename, func(buf bytecode.AssetReader, mgrOpts manager.Options) error {
		p.mgr = ddebpf.NewManagerWithDefault(&manager.Manager{
			Maps: []*manager.Map{
				{Name: poolsMapName},
				{Name: gcMapName},
				{Name: usdt.SpecsMapName},
			},
		}, moduleName, &ebpftelemetry.ErrorsTelemetryModifier{})
		// No probe is declared: the programs are attached at runtime to the
		// probe sites of the libjvm.so files. Their specs are kept to create
		// the clones reading the USDT arguments.
		mgrOpts.KeepUnmappedProgramSpecs = true
		mgrOpts.ConstantEditors = append(mgrOpts.ConstantEditors, manager.ConstantEditor{
			Name:  pidNamespaceInoConstant,
			Value: pidNamespaceIno,
		})
		if err := p.mgr.InitWithOptions(buf, &mgrOpts); err != nil {
			return fmt.Errorf("failed to init ebpf manager: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := p.mgr.Start(); err != nil {
		p.stopManager()
		return nil, fmt.Errorf("failed to start ebpf manager: %w", err)
	}
	ddebpf.AddNameMappings(p.mgr.Manager, moduleName)

	specMap, err := p.getMaps()
	if err != nil {
		p.stopManager()
		return nil, err
	}

	p.attacher = newUSDTAttacher(p.mgr.Manager, specMap, hostMachine)
	p.registry = utils.NewFileRegistry(moduleName, "libjvm")
	p.scanner = newProcessScanner(p.procRoot, p.registry, p.attacher.attach, p.attacher.detach)

	p.wg.Add(1)
	go p.run()
	return p, nil
}

func checkKernelVersion() error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return fmt.Errorf("kernel version: %w", err)
	}
	minVersion := minKernelVersion
	if runtime.GOARCH == "arm64" {
		minVersion = minKernelVersionARM64
	}
	if kv < minVersion {
		return fmt.Errorf("minimum kernel version %s not met, read %s", minVersion, kv)
	}
	return nil
}

func (p *Probe) getMaps() (*maps.GenericMap[uint32, usdt.Spec], error) {
	var err error
	if p.pools, err = maps.GetMap[ebpfPoolKey, ebpfPoolValue](p.mgr.Manager, poolsMapName); err != nil {
		return nil, err
	}
	if p.gcStats, err = maps.GetMap[uint32, ebpfGCStats](p.mgr.Manager, gcMapName); err != nil {
		return nil, err
	}
	if p.poolsCleaner, err = ddebpf.NewMapCleaner[ebpfPoolKey, ebpfPoolValue](p.pools.Map(), mapCleanerBatchSize, poolsMapName, moduleName); err != nil {
		return nil, err
	}
	if p.gcCleaner, err = ddebpf.NewMapCleaner[uint32, ebpfGCStats](p.gcStats.Map(), mapCleanerBatchSize, gcMapName, moduleName); err != nil {
		return nil, err
	}
	return maps.GetMap[uint32, usdt.Spec](p.mgr.Manager, usdt.SpecsMapName)
}

// run scans procfs periodically, until Close is called.
func (p *Probe) run() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.scanInterval)
	defer ticker.Stop()
	for {
		p.scanOnce()
		select {
		case <-p.stop:
			return
		case <-ticker.C:
		}
	}
}

// scanOnce discovers the new JVMs, and deletes the eBPF map entries of the
// exited ones.
func (p *Probe) scanOnce() {
	res, err := p.scanner.scan()
	if err != nil {
		log.Warnf("jvm_memory: scanning %s: %s", p.procRoot, err)
		return
	}
	p.jvms.Store(int64(res.jvms))
	probeTelemetry.jvms.Set(float64(res.jvms))

	// The entries are deleted for every PID which is not alive, and not only
	// for the exited JVMs seen by the scanner: a short-lived JVM sharing a
	// libjvm.so with a monitored one is traced without ever being scanned.
	alive := func(pid uint32) bool {
		if _, ok := res.reused[pid]; ok {
			// the entries are the ones of the JVM which exited
			return false
		}
		if _, ok := res.alive[pid]; ok {
			return true
		}
		// started after the listing of procfs
		return p.pidExists(pid)
	}
	p.reap(alive)
	p.cids.prune(alive)
}

// reap deletes the eBPF map entries of the PIDs which are not alive.
func (p *Probe) reap(alive func(pid uint32) bool) {
	p.gcCleaner.Clean(nil, nil, func(_ int64, tgid uint32, _ ebpfGCStats) bool {
		return !alive(tgid)
	})
	p.poolsCleaner.Clean(nil, nil, func(_ int64, key ebpfPoolKey, _ ebpfPoolValue) bool {
		return !alive(key.Tgid)
	})
}

// pidExists returns whether pid is alive, in the PID namespace of procRoot.
func (p *Probe) pidExists(pid uint32) bool {
	_, err := os.Stat(procPidPath(p.procRoot, pid))
	return !errors.Is(err, fs.ErrNotExist)
}

// GetStats returns the memory pools and GC pauses of the alive JVMs, sorted
// by PID.
func (p *Probe) GetStats() []model.JVMStats {
	pools := readMap(p.pools, poolsMapName)
	gcs := readMap(p.gcStats, gcMapName)

	monotonicNs, err := ddebpf.NowNanoseconds()
	if err != nil {
		log.Warnf("jvm_memory: reading the monotonic clock: %s", err)
		return nil
	}
	c := clock{monotonicNs: monotonicNs, wallNs: time.Now().UnixNano()}
	return buildStats(pools, gcs, c, p.pidExists, p.cids.get)
}

// readMap returns the entries of an eBPF map. Iterating a hash map while the
// reaper deletes from it can return a key more than once: the entries are
// deduplicated, so that a pool is not counted twice.
func readMap[K comparable, V any](m *maps.GenericMap[K, V], name string) map[K]V {
	entries := make(map[K]V)
	var key K
	var value V
	it := m.IterateWithBatchSize(0)
	for it.Next(&key, &value) {
		entries[key] = value
	}
	if err := it.Err(); err != nil {
		log.Warnf("jvm_memory: iterating %s: %s", name, err)
	}
	return entries
}

// DebugStats returns the state of the probe, for the module stats.
func (p *Probe) DebugStats() map[string]interface{} {
	files, specs := p.attacher.stats()
	return map[string]interface{}{
		"jvms":         p.jvms.Load(),
		"libjvm_files": files,
		"usdt_specs":   specs,
	}
}

// Close detaches the probes and releases all the resources.
func (p *Probe) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		p.wg.Wait()
		// detaches the probes of all the files
		p.registry.Clear()
		p.attacher.close()
		p.stopManager()
	})
}

func (p *Probe) stopManager() {
	if p.mgr == nil {
		return
	}
	ddebpf.RemoveNameMappings(p.mgr.Manager)
	if err := p.mgr.Stop(manager.CleanAll); err != nil {
		log.Warnf("jvm_memory: error stopping ebpf manager: %s", err)
	}
}
