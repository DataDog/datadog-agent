// +build linux_bpf,bcc

package probe

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	bpflib "github.com/iovisor/gobpf/bcc"
	"github.com/iovisor/gobpf/pkg/cpupossible"
)

/*
#include <string.h>
#include "../c/tcp-queue-length-kern-user.h"
*/
import "C"

type TCPQueueLengthTracer struct {
	m        *bpflib.Module
	statsMap *bpflib.Table
}

func NewTCPQueueLengthTracer(cfg *ebpf.Config) (*TCPQueueLengthTracer, error) {
	source, err := ebpf.PreprocessFile(cfg.BPFDir, "tcp-queue-length-kern.c")
	if err != nil {
		return nil, fmt.Errorf("Couldn’t process headers for asset “tcp-queue-length-kern.c”: %v", err)
	}

	m := bpflib.NewModule(source.String(), []string{})
	if m == nil {
		return nil, fmt.Errorf("Failed to compile “tcp-queue-length-kern.c”")
	}

	kprobe_recvmsg, err := m.LoadKprobe("kprobe__tcp_recvmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKprobe("tcp_recvmsg", kprobe_recvmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_recvmsg: %s\n", err)
	}

	kretprobe_recvmsg, err := m.LoadKprobe("kretprobe__tcp_recvmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kretprobe__tcp_recvmsg: %s\n", err)
	}

	if err := m.AttachKretprobe("tcp_recvmsg", kretprobe_recvmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_recvmsg: %s\n", err)
	}

	kprobe_sendmsg, err := m.LoadKprobe("kprobe__tcp_sendmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kprobe__tcp_sendmsg: %s\n", err)
	}

	if err := m.AttachKprobe("tcp_sendmsg", kprobe_sendmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_sendmsg: %s\n", err)
	}

	kretprobe_sendmsg, err := m.LoadKprobe("kretprobe__tcp_sendmsg")
	if err != nil {
		return nil, fmt.Errorf("Failed to load kretprobe__tcp_sendmsg: %s\n", err)
	}

	if err := m.AttachKretprobe("tcp_sendmsg", kretprobe_sendmsg, -1); err != nil {
		return nil, fmt.Errorf("Failed to attach tcp_sendmsg: %s\n", err)
	}

	table := bpflib.NewTable(m.TableId("tcp_queue_stats"), m)

	return &TCPQueueLengthTracer{
		m:        m,
		statsMap: table,
	}, nil
}

func (t *TCPQueueLengthTracer) Close() {
	t.m.Close()
}

func (t *TCPQueueLengthTracer) Get() TCPQueueLengthStats {
	if t == nil {
		return nil
	}

	cpus, err := cpupossible.Get()
	if err != nil {
		log.Errorf("Failed to get online CPUs: %v", err)
		return TCPQueueLengthStats{}
	}
	nbCpus := len(cpus)

	result := make(TCPQueueLengthStats)

	for it := t.statsMap.Iter(); it.Next(); {
		var statsKey C.struct_stats_key
		data := it.Key()
		if len(data) != C.sizeof_struct_stats_key {
			log.Errorf("Unexpected tcp_queue_stats eBPF map key size: %d instead of %d.", len(data), C.sizeof_struct_stats_key)
			break
		}
		C.memcpy(unsafe.Pointer(&statsKey), unsafe.Pointer(&data[0]), C.sizeof_struct_stats_key)
		containerID := C.GoString(&statsKey.cgroup_name[0])
		// This cannot happen because statsKey.cgroup_name is filled by bpf_probe_read_str which ensures a NULL-terminated string
		if len(containerID) >= C.sizeof_struct_stats_key {
			log.Critical("statsKey.cgroup_name wasn’t properly NULL-terminated")
			break
		}

		statsValue := make([]C.struct_stats_value, nbCpus)
		data = it.Leaf()
		if len(data) != C.sizeof_struct_stats_value*nbCpus {
			log.Errorf("Unexpected tcp_queue_length eBPF map value size: %d instead of %d.", len(data), C.sizeof_struct_stats_value*nbCpus)
			break
		}
		C.memcpy(unsafe.Pointer(&statsValue[0]), unsafe.Pointer(&data[0]), C.sizeof_struct_stats_value*C.ulong(nbCpus))

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
	}

	return result
}

func (t *TCPQueueLengthTracer) GetAndFlush() TCPQueueLengthStats {
	result := t.Get()
	t.statsMap.DeleteAll()
	return result
}
