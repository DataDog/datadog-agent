// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"C"
	"bytes"
	"fmt"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)
import "github.com/DataDog/datadog-agent/pkg/security/metrics"

// ProcessSyscall represents a syscall made by a process
type ProcessSyscall struct {
	Process string
	Pid     uint32
	ID      uint32
}

// UnmarshalBinary unmarshals a binary representation of a ProcessSyscall
func (p *ProcessSyscall) UnmarshalBinary(data []byte) error {
	var comm [16]byte
	model.SliceToArray(data[0:16], unsafe.Pointer(&comm))

	p.Process = string(bytes.Trim(comm[:], "\x00"))
	p.Pid = model.ByteOrder.Uint32(data[16:20])
	p.ID = model.ByteOrder.Uint32(data[20:24])
	return nil
}

// IsNull returns true if a ProcessSyscall instance is empty
func (p *ProcessSyscall) IsNull() bool {
	return p.Process == "" && p.Pid == 0 && p.ID == 0
}

// ProcessPath contains a process path as its binary representation
type ProcessPath struct {
	PathRaw [256]byte
	Path    string
}

// IsEmpty returns true if the current instance of ProcessPath is empty
func (p *ProcessPath) IsEmpty() bool {
	return p.Path[0] == '\x00'
}

// UnmarshalBinary unmarshals a binary representation of a ProcessSyscall
func (p *ProcessPath) UnmarshalBinary(data []byte) error {
	if len(data) == 0 {
		return errors.New("path empty")
	}
	model.SliceToArray(data[0:256], unsafe.Pointer(&p.PathRaw))
	p.Path = C.GoString((*C.char)(unsafe.Pointer(&p.PathRaw)))
	return nil
}

// SyscallStatsCollector is the interface implemented by an object that collect syscall statistics
type SyscallStatsCollector interface {
	CountSyscall(process string, syscallID Syscall, count uint64) error
	CountExec(process string, count uint64) error
	CountConcurrentSyscalls(count int64) error
}

// SyscallStats collects syscall statistics and store them in memory
type SyscallStats map[Syscall]map[string]uint64

// CountSyscall counts the number of calls of a syscall by a process
func (s *SyscallStats) CountSyscall(process string, syscallID Syscall, count uint64) error {
	if (*s)[syscallID] == nil {
		(*s)[syscallID] = make(map[string]uint64)
	}
	(*s)[syscallID][process] = count
	return nil
}

// CountExec counts the number times a process was executed
func (s *SyscallStats) CountExec(process string, count uint64) error {
	return nil
}

// CountConcurrentSyscalls counts the number of syscalls that are currently being executed
func (s *SyscallStats) CountConcurrentSyscalls(count int64) error {
	return nil
}

// SyscallStatsdCollector collects syscall statistics and sends them to statsd
type SyscallStatsdCollector struct {
	statsdClient *statsd.Client
}

// CountSyscall counts the number of calls of a syscall by a process
func (s *SyscallStatsdCollector) CountSyscall(process string, syscallID Syscall, count uint64) error {
	syscall := strings.ToLower(strings.TrimPrefix(syscallID.String(), "Sys"))
	tags := []string{
		fmt.Sprintf("process:%s", process),
		fmt.Sprintf("syscall:%s", syscall),
	}

	return s.statsdClient.Count(metrics.MetricSyscalls, int64(count), tags, 1.0)
}

// CountExec counts the number times a process was executed
func (s *SyscallStatsdCollector) CountExec(process string, count uint64) error {
	tags := []string{
		fmt.Sprintf("process:%s", process),
	}

	return s.statsdClient.Count(metrics.MetricExec, int64(count), tags, 1.0)
}

// CountConcurrentSyscalls counts the number of syscalls that are currently being executed
func (s *SyscallStatsdCollector) CountConcurrentSyscalls(count int64) error {
	if count > 0 {
		return s.statsdClient.Count(metrics.MetricConcurrentSyscall, count, []string{}, 1.0)
	}
	return nil
}

// SyscallMonitor monitors syscalls using eBPF maps filled using kernel tracepoints
type SyscallMonitor struct {
	bufferSelector     *lib.Map
	buffers            [2]*lib.Map
	execBuffers        [2]*lib.Map
	activeKernelBuffer uint32
	concurrentSyscalls *lib.Map
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
		value             uint64
		processSyscall    ProcessSyscall
		processSyscallRaw []byte
		processPath       ProcessPath
		buffer            = sm.buffers[1-sm.activeKernelBuffer]
		execBuffer        = sm.execBuffers[1-sm.activeKernelBuffer]
	)

	// syscall counter
	mapIterator := buffer.Iterate()
	for mapIterator.Next(&processSyscallRaw, &value) {
		if err := processSyscall.UnmarshalBinary(processSyscallRaw); err != nil {
			return err
		}

		if !processSyscall.IsNull() {
			if err := buffer.Delete(processSyscallRaw); err != nil {
				log.Debug(err)
			}
		}

		if err := collector.CountSyscall(processSyscall.Process, Syscall(processSyscall.ID), value); err != nil {
			return err
		}
	}
	if mapIterator.Err() != nil {
		log.Debugf("couldn't iterate over %s: %v", buffer.String(), mapIterator.Err())
	}

	// exec counter
	mapIterator = execBuffer.Iterate()
	for mapIterator.Next(&processPath, &value) {
		if !processPath.IsEmpty() {
			if err := execBuffer.Delete(&processPath.PathRaw); err != nil {
				log.Debug(err)
			}

			if err := collector.CountExec(processPath.Path, value); err != nil {
				return err
			}
		}

	}
	if mapIterator.Err() != nil {
		log.Debugf("couldn't iterate over %s: %v", execBuffer.String(), mapIterator.Err())
	}

	// concurrent syscalls counter
	var concurrentSyscallKey uint32
	var concurrentCount int64
	if err := sm.concurrentSyscalls.Lookup(concurrentSyscallKey, &concurrentCount); err != nil {
		return err
	}
	if err := collector.CountConcurrentSyscalls(concurrentCount); err != nil {
		return err
	}

	sm.activeKernelBuffer = 1 - sm.activeKernelBuffer
	return sm.bufferSelector.Put(ebpf.BufferSelectorSyscallMonitorKey, sm.activeKernelBuffer)
}

// NewSyscallMonitor instantiates a new syscall monitor
func NewSyscallMonitor(manager *manager.Manager) (*SyscallMonitor, error) {
	// select eBPF maps
	bufferSelector, ok, err := manager.GetMap("buffer_selector")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map buffer_selector not found")
	}

	frontBuffer, ok, err := manager.GetMap("noisy_processes_fb")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map noisy_processes_fb not found")
	}

	backBuffer, ok, err := manager.GetMap("noisy_processes_bb")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map noisy_processes_bb not found")
	}

	execFrontBuffer, ok, err := manager.GetMap("exec_count_fb")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map exec_count_one not found")
	}

	execBackBuffer, ok, err := manager.GetMap("exec_count_bb")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map exec_count_two not found")
	}

	concurrentSyscalls, ok, err := manager.GetMap("concurrent_syscalls")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map concurrent_syscalls not found")
	}

	return &SyscallMonitor{
		bufferSelector:     bufferSelector,
		buffers:            [2]*lib.Map{frontBuffer, backBuffer},
		execBuffers:        [2]*lib.Map{execFrontBuffer, execBackBuffer},
		concurrentSyscalls: concurrentSyscalls,
	}, nil
}
