// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"
	"time"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	ebpfmaps "github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	kprobeconfig "github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statsMapName = "network_stats"
	moduleName   = "discovery"
	// maxActive configures the number of instances of functions that this module can probe simultaneously.
	maxActive = 512
)

type eBPFNetworkCollector struct {
	m          *ebpf.Manager
	statsMap   *ebpfmaps.GenericMap[core.NetworkStatsKey, core.NetworkStats]
	errorLimit *log.Limit
}

func (c *eBPFNetworkCollector) setupManager(buf bytecode.AssetReader, options manager.Options) error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return err
	}

	probes := []*manager.Probe{
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_recvmsg", UID: moduleName}, KProbeMaxActive: maxActive},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_sendmsg", UID: moduleName}, KProbeMaxActive: maxActive},
	}

	if kprobeconfig.HasTCPSendPage(kv) {
		probes = append(probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_sendpage", UID: moduleName}, KProbeMaxActive: maxActive})
	}

	c.m = ebpf.NewManagerWithDefault(&manager.Manager{
		Probes: probes,
		Maps: []*manager.Map{
			{Name: statsMapName},
		},
	}, moduleName)

	if err := c.m.InitWithOptions(buf, &options); err != nil {
		return fmt.Errorf("failed to init manager: %w", err)
	}

	if err := c.m.Start(); err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	statsMap, err := ebpfmaps.GetMap[core.NetworkStatsKey, core.NetworkStats](c.m.Manager, statsMapName)
	if err != nil {
		return fmt.Errorf("failed to get map '%s': %w", statsMapName, err)
	}

	ebpf.AddNameMappings(c.m.Manager, moduleName)
	ebpf.AddProbeFDMappings(c.m.Manager)

	c.statsMap = statsMap

	return nil
}

func getAssetName(module string, debug bool) string {
	if debug {
		return fmt.Sprintf("%s-debug.o", module)
	}

	return fmt.Sprintf("%s.o", module)
}

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/servicediscovery/c/ebpf/runtime/discovery-net.c pkg/ebpf/bytecode/build/runtime/discovery-net.c pkg/ebpf/c pkg/collector/corechecks/servicediscovery/c/ebpf/runtime pkg/network/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/discovery-net.c pkg/ebpf/bytecode/runtime/discovery-net.go runtime

func runtimeCompile(cfg *ebpf.Config) (runtime.CompiledOutput, error) {
	return runtime.DiscoveryNet.Compile(cfg, getCFlags(cfg))
}

func getCFlags(cfg *ebpf.Config) []string {
	cflags := []string{"-g"}

	if cfg.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

func (c *eBPFNetworkCollector) initRuntimeCompiled(cfg *ebpf.Config) error {
	buf, err := runtimeCompile(cfg)
	if err != nil {
		return err
	}

	defer buf.Close()

	return c.setupManager(buf, manager.Options{})
}

func (c *eBPFNetworkCollector) initCORE(cfg *ebpf.Config) error {
	asset := getAssetName("discovery-net", cfg.BPFDebug)
	return ebpf.LoadCOREAsset(asset, func(ar bytecode.AssetReader, o manager.Options) error {
		return c.setupManager(ar, o)
	})
}

func newNetworkCollectorWithConfig(cfg *ebpf.Config) (core.NetworkCollector, error) {
	collector := eBPFNetworkCollector{}
	collector.errorLimit = log.NewLogLimit(10, 10*time.Minute)

	if cfg.EnableCORE {
		err := collector.initCORE(cfg)
		if err == nil {
			return &collector, nil
		}

		if !cfg.AllowRuntimeCompiledFallback {
			return nil, fmt.Errorf("error loading CO-RE %w", err)
		}

		log.Warnf("%s: error loading CO-RE, falling back to runtime compiled: %v", moduleName, err)
	}

	if !cfg.EnableRuntimeCompiler {
		return nil, fmt.Errorf("%s: cannot compile probe", moduleName)
	}

	err := collector.initRuntimeCompiled(cfg)
	if err != nil {
		return nil, err
	}

	return &collector, nil
}

func newNetworkCollector(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
	return newNetworkCollectorWithConfig(ebpf.NewConfig())
}

func (c *eBPFNetworkCollector) Close() {
	if err := c.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping network collector: %v", err)
	}
}

func (c *eBPFNetworkCollector) GetStats(pids core.PidSet) (map[uint32]core.NetworkStats, error) {
	stats := make(map[uint32]core.NetworkStats)
	var toDelete []core.NetworkStatsKey

	it := c.statsMap.IterateWithBatchSize(0)
	var key core.NetworkStatsKey
	var val core.NetworkStats
	for it.Next(&key, &val) {
		if !pids.Has(int32(key.Pid)) {
			toDelete = append(toDelete, key)
			continue
		}

		stats[key.Pid] = val
	}

	// Delete pids which were in the eBPF map but not in the requested pids set
	for _, key := range toDelete {
		err := c.statsMap.Delete(&key)
		if err != nil {
			log.Warnf("error deleting pid %d from eBPF map: %v", key.Pid, err)
		}
	}

	// Add new pids which were not in the eBPF map
	for pid := range pids {
		if _, ok := stats[uint32(pid)]; ok {
			continue
		}

		err := c.statsMap.Put(&core.NetworkStatsKey{Pid: uint32(pid)}, &core.NetworkStats{})
		if err != nil && c.errorLimit.ShouldLog() {
			// This error can occur if the eBPF map is full.
			log.Warnf("error adding pid %d to eBPF map: %v", pid, err)
		}
	}

	return stats, nil
}
