// +build linux_bpf,bcc

package probe

import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	bpflib "github.com/iovisor/gobpf/bcc"
	"github.com/iovisor/gobpf/pkg/cpuonline"
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

	cpus, err := cpuonline.Get()
	if err != nil {
		log.Errorf("Failed to get online CPUs: %v", err)
		return TCPQueueLengthStats{}
	}

	result := make(TCPQueueLengthStats)

	for it := t.statsMap.Iter(); it.Next(); {
		var statsKey C.struct_stats_key
		data := it.Key()
		C.memcpy(unsafe.Pointer(&statsKey), unsafe.Pointer(&data[0]), C.sizeof_struct_stats_key)
		containerID := C.GoString(&statsKey.cgroup_name[0])

		var statsValue [256]C.struct_stats_value
		data = it.Leaf()
		C.memcpy(unsafe.Pointer(&statsValue), unsafe.Pointer(&data[0]), C.sizeof_struct_stats_value*C.ulong(len(cpus)))

		max := TCPQueueLengthStatsValue{}
		for _, cpu := range cpus {
			if cpu > 256 {
				log.Error("Too many CPUs")
				continue
			}
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
