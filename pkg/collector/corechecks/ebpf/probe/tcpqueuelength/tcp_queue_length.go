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
	"errors"
	"fmt"
	"strings"

	manager "github.com/DataDog/ebpf-manager"
	ebpflib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
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
	coll     *ebpflib.Collection
	links    []link.Link
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
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, err
	}

	collSpec, err := ebpflib.LoadCollectionSpecFromReader(buf)
	if err != nil {
		return nil, fmt.Errorf("load collection spec: %s", err)
	}

	if features.SupportsFentry("tcp_recvmsg") {
		delete(collSpec.Programs, "kprobe__tcp_recvmsg")
		delete(collSpec.Programs, "kretprobe__tcp_recvmsg")
		delete(collSpec.Programs, "kprobe__tcp_sendmsg")
		delete(collSpec.Programs, "kretprobe__tcp_sendmsg")
		delete(collSpec.Maps, "who_recvmsg")
		delete(collSpec.Maps, "who_sendmsg")

		tcpRecvEntry := collSpec.Programs["tcp_recvmsg_entry"]

		tcpRecvExit := tcpRecvEntry.Copy()
		tcpRecvExit.AttachType = ebpflib.AttachTraceFExit
		tcpRecvExit.AttachTo = "tcp_recvmsg"
		tcpRecvExit.SectionName = "fexit/tcp_recvmsg"
		collSpec.Programs["tcp_recvmsg_exit"] = tcpRecvExit

		tcpSendEntry := tcpRecvEntry.Copy()
		tcpSendEntry.AttachType = ebpflib.AttachTraceFEntry
		tcpSendEntry.AttachTo = "tcp_sendmsg"
		tcpSendEntry.SectionName = "fentry/tcp_sendmsg"
		collSpec.Programs["tcp_sendmsg_entry"] = tcpSendEntry

		tcpSendExit := tcpRecvEntry.Copy()
		tcpSendExit.AttachType = ebpflib.AttachTraceFExit
		tcpSendExit.AttachTo = "tcp_sendmsg"
		tcpSendExit.SectionName = "fexit/tcp_sendmsg"
		collSpec.Programs["tcp_sendmsg_exit"] = tcpSendExit
	} else {
		delete(collSpec.Programs, "tcp_recvmsg_entry")
	}

	coll, err := ebpflib.NewCollectionWithOptions(collSpec, managerOptions.VerifierOptions)
	if err != nil {
		var ve *ebpflib.VerifierError
		if errors.As(err, &ve) {
			return nil, fmt.Errorf("verifier error loading tcpq collection: %s\n%+v", err, ve)
		}
		return nil, fmt.Errorf("new tcpq collection: %s", err)
	}
	ebpf.AddNameMappingsCollection(coll, "tcp_queue_length")

	statsMap, err := ebpfmaps.Map[StructStatsKey, []StructStatsValue](coll.Maps[statsMapName])
	if err != nil {
		return nil, fmt.Errorf("failed to get map '%s': %w", statsMapName, err)
	}

	t := &Tracer{
		coll:     coll,
		statsMap: statsMap,
	}
	if err := t.attach(collSpec); err != nil {
		return nil, err
	}
	return t, nil
}

// Close releases all associated resources
func (t *Tracer) Close() {
	ebpf.RemoveNameMappingsCollection(t.coll)
	for _, l := range t.links {
		if err := l.Close(); err != nil {
			log.Warnf("error unlinking program: %s", err)
		}
	}
	t.coll.Close()
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
		maxStats := model.TCPQueueLengthStatsValue{}
		for cpu := 0; cpu < nbCpus; cpu++ {
			maxStats.ReadBufferMaxUsage = max(statsValue[cpu].Read_buffer_max_usage, maxStats.ReadBufferMaxUsage)
			maxStats.WriteBufferMaxUsage = max(statsValue[cpu].Write_buffer_max_usage, maxStats.WriteBufferMaxUsage)
		}
		result[cgroupName] = maxStats
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

func (t *Tracer) attach(collSpec *ebpflib.CollectionSpec) (err error) {
	defer func() {
		// if anything fails, we need to close/detach everything
		if err != nil {
			t.Close()
		}
	}()

	for name, prog := range t.coll.Programs {
		spec := collSpec.Programs[name]
		switch prog.Type() {
		case ebpflib.Kprobe:
			const kprobePrefix, kretprobePrefix = "kprobe/", "kretprobe/"
			if strings.HasPrefix(spec.SectionName, kprobePrefix) {
				attachPoint := spec.SectionName[len(kprobePrefix):]
				l, err := link.Kprobe(attachPoint, prog, &link.KprobeOptions{
					TraceFSPrefix: "ddtcpq",
				})
				if err != nil {
					return fmt.Errorf("link kprobe %s to %s: %s", spec.Name, attachPoint, err)
				}
				t.links = append(t.links, l)
			} else if strings.HasPrefix(spec.SectionName, kretprobePrefix) {
				attachPoint := spec.SectionName[len(kretprobePrefix):]
				manager.TraceFSLock.Lock()
				l, err := link.Kretprobe(attachPoint, prog, &link.KprobeOptions{
					TraceFSPrefix:     "ddtcpq",
					RetprobeMaxActive: maxActive,
				})
				manager.TraceFSLock.Unlock()
				if err != nil {
					return fmt.Errorf("link kretprobe %s to %s: %s", spec.Name, attachPoint, err)
				}
				t.links = append(t.links, l)
			} else {
				return fmt.Errorf("unknown section prefix: %s", spec.SectionName)
			}
		case ebpflib.Tracing:
			l, err := link.AttachTracing(link.TracingOptions{
				Program: prog,
			})
			if err != nil {
				return fmt.Errorf("link tracing %s to %s: %s", name, spec.AttachTo, err)
			}
			t.links = append(t.links, l)
		default:
			return fmt.Errorf("unknown program %s type: %T", name, prog.Type())
		}
	}
	return nil
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
