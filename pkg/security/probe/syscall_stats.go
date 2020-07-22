// +build linux_bpf

package probe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/statsd"
)

const syscallMetric = MetricPrefix + ".syscalls"

// ProcessSyscall represents a syscall made by a process
type ProcessSyscall struct {
	Process string
	Pid     uint32
	ID      uint32
}

// UnmarshalBinary unmarshals a binary representation of a ProcessSyscall
func (p *ProcessSyscall) UnmarshalBinary(data []byte) error {
	var comm [16]byte
	if err := binary.Read(bytes.NewBuffer(data[0:16]), byteOrder, &comm); err != nil {
		return err
	}
	p.Process = string(bytes.Trim(comm[:], "\x00"))
	p.Pid = byteOrder.Uint32(data[16:20])
	p.ID = byteOrder.Uint32(data[20:24])
	return nil
}

// SyscallStatsCollector is the interface implemented by an object that collect syscall statistics
type SyscallStatsCollector interface {
	Count(process string, syscallID Syscall, count uint64) error
}

// SyscallStats collects syscall statistics and store them in memory
type SyscallStats map[Syscall]map[string]uint64

// Count the number of calls of a syscall by a process
func (s *SyscallStats) Count(process string, syscallID Syscall, count uint64) error {
	if (*s)[syscallID] == nil {
		(*s)[syscallID] = make(map[string]uint64)
	}
	(*s)[syscallID][process] = count
	return nil
}

// SyscallStatsdCollector collects syscall statistics and sends them to statsd
type SyscallStatsdCollector struct {
	statsdClient *statsd.Client
}

// Count the number of calls of a syscall by a process
func (s *SyscallStatsdCollector) Count(process string, syscallID Syscall, count uint64) error {
	syscall := strings.ToLower(strings.TrimPrefix(syscallID.String(), "Sys"))
	tags := []string{
		fmt.Sprintf("process:%s", process),
		fmt.Sprintf("syscall:%s", syscall),
	}

	return s.statsdClient.Count(syscallMetric, int64(count), tags, 1.0)
}

// SyscallMonitor monitors syscalls using eBPF maps filled using kernel tracepoints
type SyscallMonitor struct {
	bufferSelector     *ebpf.Table
	buffers            [2]*ebpf.Table
	activeKernelBuffer ebpf.Uint32TableItem
}

// GetStats returns the syscall statistics
func (sm *SyscallMonitor) GetStats() (*SyscallStats, error) {
	stats := make(SyscallStats)
	if err := sm.CollectStats(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

// SendStats sends the syscall statistics to statsd
func (sm *SyscallMonitor) SendStats(statsdClient *statsd.Client) error {
	collector := &SyscallStatsdCollector{statsdClient: statsdClient}
	return sm.CollectStats(collector)
}

// CollectStats fetches the syscall statistics from the eBPF maps
func (sm *SyscallMonitor) CollectStats(collector SyscallStatsCollector) error {
	var (
		zeroKey        [24]byte
		prevKey        [24]byte
		processSyscall ProcessSyscall
		buffer         = sm.buffers[1-sm.activeKernelBuffer]
	)

	for {
		more, nextKey, value, err := buffer.GetNext(prevKey[:])
		if err != nil {
			return err
		}

		if string(zeroKey[:]) != string(prevKey[:]) {
			if err = buffer.Delete(prevKey[:]); err != nil {
				log.Debug(err)
			}
		}

		if !more {
			break
		}

		copy(prevKey[:], nextKey[:])
		if err := processSyscall.UnmarshalBinary(nextKey); err != nil {
			return err
		}

		count := byteOrder.Uint64(value[0:8])

		if err := collector.Count(processSyscall.Process, Syscall(processSyscall.ID), count); err != nil {
			return err
		}
	}

	sm.activeKernelBuffer = 1 - sm.activeKernelBuffer
	return sm.bufferSelector.Set(ebpf.ZeroUint32TableItem, sm.activeKernelBuffer)
}

// NewSyscallMonitor instantiates a new syscall monitor
func NewSyscallMonitor(module *ebpf.Module, bufferSelector, frontBuffer, backBuffer *ebpf.Table) (*SyscallMonitor, error) {
	if err := module.RegisterTracepoint("tracepoint/raw_syscalls/sys_enter"); err != nil {
		return nil, err
	}

	return &SyscallMonitor{
		bufferSelector: bufferSelector,
		buffers:        [2]*ebpf.Table{frontBuffer, backBuffer},
	}, nil
}
