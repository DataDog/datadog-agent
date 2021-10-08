// +build linux_bpf

//go:generate go run ../../../../ebpf/include_headers.go ../c/runtime/tcp-queue-length-kern.c ../../../../ebpf/bytecode/build/runtime/tcp-queue-length.c ../../../../ebpf/c
//go:generate go run ../../../../ebpf/bytecode/runtime/integrity.go ../../../../ebpf/bytecode/build/runtime/tcp-queue-length.c ../../../../ebpf/bytecode/runtime/tcp-queue-length.go runtime

package probe

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/iovisor/gobpf/pkg/cpupossible"
	"golang.org/x/sys/unix"

	bpflib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

/*
#include <string.h>
#include "../c/runtime/tcp-queue-length-kern-user.h"
*/
import "C"

const (
	TCPQueueLengthUID = "tcp-queue-length"
	statsMapName      = "tcp_queue_stats"
)

type TCPQueueLengthTracer struct {
	m        *manager.Manager
	statsMap *bpflib.Map
}

func NewTCPQueueLengthTracer(cfg *ebpf.Config) (*TCPQueueLengthTracer, error) {
	compiledOutput, err := runtime.TcpQueueLength.Compile(cfg, nil)
	if err != nil {
		return nil, err
	}
	defer compiledOutput.Close()

	probes := []*manager.Probe{
		{Section: "kprobe/tcp_recvmsg"},
		{Section: "kretprobe/tcp_recvmsg"},
		{Section: "kprobe/tcp_sendmsg"},
		{Section: "kretprobe/tcp_sendmsg"},
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

	managerOptions := manager.Options{
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
	}

	if err := m.InitWithOptions(compiledOutput, managerOptions); err != nil {
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

	return &TCPQueueLengthTracer{
		m:        m,
		statsMap: statsMap,
	}, nil
}

func (t *TCPQueueLengthTracer) Close() {
	t.m.Stop(manager.CleanAll)
}

func (t *TCPQueueLengthTracer) GetAndFlush() TCPQueueLengthStats {
	cpus, err := cpupossible.Get()
	if err != nil {
		log.Errorf("Failed to get online CPUs: %v", err)
		return TCPQueueLengthStats{}
	}
	nbCpus := len(cpus)

	result := make(TCPQueueLengthStats)

	var statsKey C.struct_stats_key
	statsValue := make([]C.struct_stats_value, nbCpus)
	it := t.statsMap.Iterate()
	for it.Next(unsafe.Pointer(&statsKey), unsafe.Pointer(&statsValue[0])) {
		containerID := C.GoString(&statsKey.cgroup_name[0])
		// This cannot happen because statsKey.cgroup_name is filled by bpf_probe_read_str which ensures a NULL-terminated string
		if len(containerID) >= C.sizeof_struct_stats_key {
			log.Critical("statsKey.cgroup_name wasn’t properly NULL-terminated")
			break
		}

		max := TCPQueueLengthStatsValue{}
		for _, cpu := range cpus {
			if uint32(statsValue[cpu].read_buffer_max_usage) > max.ReadBufferMaxUsage {
				max.ReadBufferMaxUsage = uint32(statsValue[cpu].read_buffer_max_usage)
			}
			if uint32(statsValue[cpu].write_buffer_max_usage) > max.WriteBufferMaxUsage {
				max.WriteBufferMaxUsage = uint32(statsValue[cpu].write_buffer_max_usage)
			}
		}
		result[containerID] = max

		if err := t.statsMap.Delete(unsafe.Pointer(&statsKey)); err != nil {
			log.Warnf("failed to delete stat: %s", err)
		}
	}

	if err := it.Err(); err != nil {
		log.Warnf("failed to iterate on TCP queue length stats while flushing: %s", err)
	}

	return result
}
