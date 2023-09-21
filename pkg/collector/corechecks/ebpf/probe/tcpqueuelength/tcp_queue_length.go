// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

//go:generate $GOPATH/bin/include_headers pkg/collector/corechecks/ebpf/c/runtime/tcp-queue-length-kern.c pkg/ebpf/bytecode/build/runtime/tcp-queue-length.c pkg/ebpf/c
//go:generate $GOPATH/bin/integrity pkg/ebpf/bytecode/build/runtime/tcp-queue-length.c pkg/ebpf/bytecode/runtime/tcp-queue-length.go runtime

// Package tcpqueuelength is the system-probe side of the TCP Queue Length check
package tcpqueuelength

import (
	"fmt"
	"math"
	"unsafe"

	"golang.org/x/sys/unix"

	manager "github.com/DataDog/ebpf-manager"
	bpflib "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/process/statsd"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statsMapName = "tcp_queue_stats"
)

// Tracer is the eBPF side of the TCP Queue Length check
type Tracer struct {
	m        *manager.Manager
	statsMap *bpflib.Map
}

// NewTracer creates a [Tracer]
func NewTracer(cfg *ebpf.Config) (*Tracer, error) {
	if cfg.EnableCORE {
		probe, err := loadTCPQueueLengthCOREProbe(cfg)
		if err != nil {
			if !cfg.AllowRuntimeCompiledFallback {
				return nil, fmt.Errorf("error loading CO-RE tcp-queue-length probe: %s. set system_probe_config.allow_runtime_compiled_fallback to true to allow fallback to runtime compilation", err)
			}
			log.Warnf("error loading CO-RE tcp-queue-length probe: %s. falling back to runtime compiled probe", err)
		} else {
			return probe, nil
		}
	}

	return loadTCPQueueLengthRuntimeCompiledProbe(cfg)
}

func startTCPQueueLengthProbe(buf bytecode.AssetReader, managerOptions manager.Options) (*Tracer, error) {
	probes := []*manager.Probe{
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__tcp_recvmsg", UID: "tcpq"}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_recvmsg", UID: "tcpq"}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__tcp_sendmsg", UID: "tcpq"}},
		{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_sendmsg", UID: "tcpq"}},
	}

	maps := []*manager.Map{
		{Name: "tcp_queue_stats"},
		{Name: "who_recvmsg"},
		{Name: "who_sendmsg"},
	}

	m := &manager.Manager{
		Probes: probes,
		Maps:   maps,
	}

	managerOptions.RLimit = &unix.Rlimit{
		Cur: math.MaxUint64,
		Max: math.MaxUint64,
	}

	if err := m.InitWithOptions(buf, managerOptions); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	statsMap, ok, err := m.GetMap(statsMapName)
	if err != nil {
		return nil, fmt.Errorf("failed to get map '%s': %w", statsMapName, err)
	} else if !ok {
		return nil, fmt.Errorf("failed to get map '%s'", statsMapName)
	}
	ebpfcheck.AddNameMappings(m, "tcp_queue_length")

	return &Tracer{
		m:        m,
		statsMap: statsMap,
	}, nil
}

// Close releases all associated resources
func (t *Tracer) Close() {
	ebpfcheck.RemoveNameMappings(t.m)
	if err := t.m.Stop(manager.CleanAll); err != nil {
		log.Errorf("error stopping TCP Queue Length: %s", err)
	}
}

// GetAndFlush gets the stats
func (t *Tracer) GetAndFlush() model.TCPQueueLengthStats {
	nbCpus, err := kernel.PossibleCPUs()
	if err != nil {
		log.Errorf("Failed to get online CPUs: %v", err)
		return model.TCPQueueLengthStats{}
	}

	result := make(model.TCPQueueLengthStats)

	var statsKey StructStatsKey
	var keys []StructStatsKey
	statsValue := make([]StructStatsValue, nbCpus)
	it := t.statsMap.Iterate()
	for it.Next(unsafe.Pointer(&statsKey), &statsValue) {
		cgroupName := string(statsKey.Cgroup[:])
		max := model.TCPQueueLengthStatsValue{}
		for cpu := 0; cpu < nbCpus; cpu++ {
			if statsValue[cpu].Read_buffer_max_usage > max.ReadBufferMaxUsage {
				max.ReadBufferMaxUsage = statsValue[cpu].Read_buffer_max_usage
			}
			if statsValue[cpu].Write_buffer_max_usage > max.WriteBufferMaxUsage {
				max.WriteBufferMaxUsage = statsValue[cpu].Write_buffer_max_usage
			}
		}
		result[cgroupName] = max
		keys = append(keys, statsKey)
	}
	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate on TCP queue length stats while flushing: %s", err)
	}
	for _, k := range keys {
		if err := t.statsMap.Delete(unsafe.Pointer(&k)); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	return result
}

func loadTCPQueueLengthCOREProbe(cfg *ebpf.Config) (*Tracer, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %s", err)
	}
	if kv < kernel.VersionCode(4, 8, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but tcp-queue-length probe requires a kernel version of at least 4.8.0", kv)
	}

	filename := "tcp-queue-length.o"
	if cfg.BPFDebug {
		filename = "tcp-queue-length-debug.o"
	}

	var probe *Tracer
	err = ebpf.LoadCOREAsset(cfg, filename, func(buf bytecode.AssetReader, opts manager.Options) error {
		probe, err = startTCPQueueLengthProbe(buf, opts)
		return err
	})
	if err != nil {
		return nil, err
	}

	log.Debugf("successfully loaded CO-RE version of tcp-queue-length probe")
	return probe, nil
}

func loadTCPQueueLengthRuntimeCompiledProbe(cfg *ebpf.Config) (*Tracer, error) {
	compiledOutput, err := runtime.TcpQueueLength.Compile(cfg, []string{"-g"}, statsd.Client)
	if err != nil {
		return nil, err
	}
	defer compiledOutput.Close()

	return startTCPQueueLengthProbe(compiledOutput, manager.Options{})
}
