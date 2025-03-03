// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
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
)

type eBPFNetworkCollector struct {
	m        *ddebpf.Manager
	statsMap *ebpfmaps.GenericMap[NetworkStatsKey, NetworkStats]
}

func (c *eBPFNetworkCollector) setupManager(buf bytecode.AssetReader, options manager.Options) error {
	kv, err := kernel.HostVersion()
	if err != nil {
		return err
	}

	probes := []*manager.Probe{
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_recvmsg", UID: moduleName}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_sendmsg", UID: moduleName}},
	}

	if kprobeconfig.HasTCPSendPage(kv) {
		probes = append(probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_sendpage", UID: moduleName}})
	}

	c.m = ddebpf.NewManagerWithDefault(&manager.Manager{
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

	statsMap, err := ebpfmaps.GetMap[NetworkStatsKey, NetworkStats](c.m.Manager, statsMapName)
	if err != nil {
		return fmt.Errorf("failed to get map '%s': %w", statsMapName, err)
	}

	ddebpf.AddNameMappings(c.m.Manager, moduleName)

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

func runtimeCompile(cfg *discoveryConfig) (runtime.CompiledOutput, error) {
	return runtime.DiscoveryNet.Compile(&cfg.Config, getCFlags(cfg))
}

func getCFlags(cfg *discoveryConfig) []string {
	cflags := []string{"-g"}

	if cfg.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

func (c *eBPFNetworkCollector) initRuntimeCompiled(cfg *discoveryConfig) error {
	buf, err := runtimeCompile(cfg)
	if err != nil {
		return err
	}

	defer buf.Close()

	return c.setupManager(buf, manager.Options{})
}

func (c *eBPFNetworkCollector) initCORE(cfg *discoveryConfig) error {
	asset := getAssetName("discovery-net", cfg.BPFDebug)
	return ddebpf.LoadCOREAsset(asset, func(ar bytecode.AssetReader, o manager.Options) error {
		return c.setupManager(ar, o)
	})
}

func newNetworkCollector(cfg *discoveryConfig) (networkCollector, error) {
	collector := eBPFNetworkCollector{}

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

func (c *eBPFNetworkCollector) close() {
	if err := c.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping network collector: %v", err)
	}
}

func (c *eBPFNetworkCollector) addPid(pid uint32) error {
	key := NetworkStatsKey{Pid: pid}
	var val NetworkStats

	return c.statsMap.Put(&key, &val)
}

func (c *eBPFNetworkCollector) removePid(pid uint32) error {
	key := NetworkStatsKey{Pid: pid}
	return c.statsMap.Delete(&key)
}

func (c *eBPFNetworkCollector) getStats(pid uint32) (NetworkStats, error) {
	key := NetworkStatsKey{Pid: pid}
	var val NetworkStats
	err := c.statsMap.Lookup(&key, &val)
	return val, err
}
