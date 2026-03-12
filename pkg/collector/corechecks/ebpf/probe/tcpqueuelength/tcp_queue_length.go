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

	manager "github.com/DataDog/ebpf-manager"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength/model"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	ebpfmaps "github.com/DataDog/datadog-agent/pkg/ebpf/maps"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	statsMapName = "tcp_queue_stats"

	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 512
)

// Tracer is the eBPF side of the TCP Queue Length check
type Tracer struct {
	m        *manager.Manager
	statsMap *ebpfmaps.GenericMap[StructStatsKey, []StructStatsValue]
}

// NewTracer creates a [Tracer]
func NewTracer(cfg *ebpf.Config) (*Tracer, error) {
	if cfg.EnableCORE {
		probe, err := loadTCPQueueLengthCOREProbe()
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
	m := &manager.Manager{
		Probes: []*manager.Probe{
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__tcp_recvmsg", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_recvmsg", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kprobe__tcp_sendmsg", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "kretprobe__tcp_sendmsg", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tcp_recvmsg_entry", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tcp_recvmsg_exit", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tcp_sendmsg_entry", UID: "tcpq"}},
			{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "tcp_sendmsg_exit", UID: "tcpq"}},
		},
		Maps: []*manager.Map{
			{Name: "tcp_queue_stats"},
			{Name: "who_recvmsg"},
			{Name: "who_sendmsg"},
		},
	}

	managerOptions.RemoveRlimit = true

	if features.SupportsFentry("tcp_recvmsg") {
		managerOptions.ExcludedFunctions = append(managerOptions.ExcludedFunctions,
			"kprobe__tcp_recvmsg",
			"kretprobe__tcp_recvmsg",
			"kprobe__tcp_sendmsg",
			"kretprobe__tcp_sendmsg",
		)
		managerOptions.ExcludedMaps = append(managerOptions.ExcludedMaps,
			"who_recvmsg",
			"who_sendmsg",
		)
	} else {
		managerOptions.DefaultKProbeMaxActive = maxActive
		managerOptions.ExcludedFunctions = append(managerOptions.ExcludedFunctions,
			"tcp_recvmsg_entry",
			"tcp_recvmsg_exit",
			"tcp_sendmsg_entry",
			"tcp_sendmsg_exit",
		)
	}

	if err := m.InitWithOptions(buf, managerOptions); err != nil {
		return nil, fmt.Errorf("failed to init manager: %w", err)
	}

	if err := m.Start(); err != nil {
		return nil, fmt.Errorf("failed to start manager: %w", err)
	}

	ebpf.AddProbeFDMappings(m)

	statsMap, err := ebpfmaps.GetMap[StructStatsKey, []StructStatsValue](m, statsMapName)
	if err != nil {
		return nil, fmt.Errorf("failed to get map '%s': %w", statsMapName, err)
	}
	ebpf.AddNameMappings(m, "tcp_queue_length")

	return &Tracer{
		m:        m,
		statsMap: statsMap,
	}, nil
}

// Close releases all associated resources
func (t *Tracer) Close() {
	ebpf.RemoveNameMappings(t.m)
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
	for it.Next(&statsKey, &statsValue) {
		cgroupName := unix.ByteSliceToString(statsKey.Cgroup[:])
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
		if err := t.statsMap.Delete(&k); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	return result
}

func loadTCPQueueLengthCOREProbe() (*Tracer, error) {
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("error detecting kernel version: %s", err)
	}
	if kv < kernel.VersionCode(4, 8, 0) {
		return nil, fmt.Errorf("detected kernel version %s, but tcp-queue-length probe requires a kernel version of at least 4.8.0", kv)
	}

	var probe *Tracer
	err = ebpf.LoadCOREAsset("tcp-queue-length.o", func(buf bytecode.AssetReader, opts manager.Options) error {
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
	compiledOutput, err := runtime.TcpQueueLength.Compile(cfg, []string{"-g"})
	if err != nil {
		return nil, err
	}
	defer compiledOutput.Close()

	return startTCPQueueLengthProbe(compiledOutput, manager.Options{})
}
