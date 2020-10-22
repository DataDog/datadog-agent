// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	if err := binary.Read(bytes.NewBuffer(data[0:16]), ebpf.ByteOrder, &comm); err != nil {
		return err
	}
	p.Process = string(bytes.Trim(comm[:], "\x00"))
	p.Pid = ebpf.ByteOrder.Uint32(data[16:20])
	p.ID = ebpf.ByteOrder.Uint32(data[20:24])
	return nil
}

// IsNull returns true if a ProcessSyscall instance is empty
func (p *ProcessSyscall) IsNull() bool {
	return p.Process == "" && p.Pid == 0 && p.ID == 0
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
	bufferSelector     *lib.Map
	buffers            [2]*lib.Map
	activeKernelBuffer uint32
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
		buffer            = sm.buffers[1-sm.activeKernelBuffer]
	)

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

		if err := collector.Count(processSyscall.Process, Syscall(processSyscall.ID), value); err != nil {
			return err
		}
	}

	sm.activeKernelBuffer = 1 - sm.activeKernelBuffer
	return sm.bufferSelector.Put(ebpf.ZeroUint32MapItem, sm.activeKernelBuffer)
}

// NewSyscallMonitor instantiates a new syscall monitor
func NewSyscallMonitor(manager *manager.Manager) (*SyscallMonitor, error) {
	// select eBPF maps
	bufferSelector, ok, err := manager.GetMap("noisy_processes_buffer")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("map noisy_processes_buffer not found")
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

	return &SyscallMonitor{
		bufferSelector: bufferSelector,
		buffers:        [2]*lib.Map{frontBuffer, backBuffer},
	}, nil
}
