package probe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
	"github.com/DataDog/datadog-agent/pkg/ebpf/probe/types"
	"github.com/DataDog/datadog-go/statsd"
)

const SyscallMetric = MetricPrefix + ".syscalls"

type ProcessSyscall struct {
	Process string
	Pid     uint32
	Id      uint32
}

func (p *ProcessSyscall) UnmarshalBinary(data []byte) error {
	var comm [16]byte
	if err := binary.Read(bytes.NewBuffer(data[0:16]), byteOrder, &comm); err != nil {
		return err
	}
	p.Process = string(bytes.Trim(comm[:], "\x00"))
	p.Pid = byteOrder.Uint32(data[16:20])
	p.Id = byteOrder.Uint32(data[20:24])
	return nil
}

type SyscallStatsCollector interface {
	Count(process string, syscallId Syscall, count uint64) error
}

type SyscallStats map[Syscall]map[string]uint64

func (s *SyscallStats) Count(process string, syscallId Syscall, count uint64) error {
	if (*s)[syscallId] == nil {
		(*s)[syscallId] = make(map[string]uint64)
	}
	(*s)[syscallId][process] = count
	return nil
}

type SyscallStatsdCollector struct {
	statsdClient *statsd.Client
}

func (s *SyscallStatsdCollector) Count(process string, syscallId Syscall, count uint64) error {
	syscall := strings.ToLower(strings.TrimPrefix(Syscall(syscallId).String(), "SYS_"))
	tags := []string{
		fmt.Sprintf("process:%s", process),
		fmt.Sprintf("syscall:%s", syscall),
	}

	return s.statsdClient.Count(SyscallMetric, int64(count), tags, 1.0)
}

type SyscallMonitor struct {
	bufferSelector     eprobe.Table
	buffers            [2]eprobe.Table
	activeKernelBuffer Uint32Key
}

type Uint32Key uint32

func (i Uint32Key) Bytes() []byte {
	var buffer [4]byte
	byteOrder.PutUint32(buffer[:], uint32(i))
	return buffer[:]
}

func (sm *SyscallMonitor) GetStats() (*SyscallStats, error) {
	stats := SyscallStats(make(map[Syscall]map[string]uint64))
	if err := sm.CollectStats(&stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (sm *SyscallMonitor) SendStats(statsdClient *statsd.Client) error {
	collector := &SyscallStatsdCollector{statsdClient: statsdClient}
	return sm.CollectStats(collector)
}

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
			buffer.Delete(prevKey[:])
		}

		if !more {
			break
		}

		copy(prevKey[:], nextKey[:])
		if err := processSyscall.UnmarshalBinary(nextKey); err != nil {
			return err
		}

		count := byteOrder.Uint64(value[0:8])

		if err := collector.Count(processSyscall.Process, Syscall(processSyscall.Id), count); err != nil {
			return err
		}
	}

	sm.activeKernelBuffer = 1 - sm.activeKernelBuffer
	return sm.bufferSelector.Set(zeroInt32, sm.activeKernelBuffer.Bytes())
}

func NewSyscallMonitor(module eprobe.Module, bufferSelector, frontBuffer, backBuffer eprobe.Table) (*SyscallMonitor, error) {
	if err := module.RegisterTracepoint(&types.Tracepoint{
		Name: "tracepoint/raw_syscalls/sys_enter",
	}); err != nil {
		return nil, err
	}

	return &SyscallMonitor{
		bufferSelector: bufferSelector,
		buffers:        [2]eprobe.Table{frontBuffer, backBuffer},
	}, nil
}
