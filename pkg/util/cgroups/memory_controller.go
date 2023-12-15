// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

import (
	"fmt"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	cgroupsv1 "github.com/containerd/cgroups/v3/cgroup1"
)

const maxEpollEvents = 4

// MemoryController describes a cgroup based memory controller
type MemoryController struct {
	efd          int
	memoryEvents map[int]func()
}

// MemoryMonitor creates a cgroup memory event
type MemoryMonitor func(cgroup cgroupsv1.Cgroup) (cgroupsv1.MemoryEvent, func(), error)

// MemoryPercentageThresholdMonitor monitors memory usage above a specified percentage threshold
func MemoryPercentageThresholdMonitor(cb func(), percentage uint64, swap bool) MemoryMonitor {
	return func(cgroup cgroupsv1.Cgroup) (cgroupsv1.MemoryEvent, func(), error) {
		metrics, err := cgroup.Stat(cgroupsv1.IgnoreNotExist)
		if err != nil {
			return nil, nil, fmt.Errorf("can't get cgroup metrics: %w", err)
		}

		if metrics.Memory == nil || metrics.Memory.Usage == nil {
			return nil, nil, fmt.Errorf("can't get cgroup memory metrics: %w", err)
		}

		return cgroupsv1.MemoryThresholdEvent(metrics.Memory.Usage.Limit*percentage/100, swap), cb, nil
	}
}

// MemoryThresholdMonitor monitors memory usage above a specified threshold
func MemoryThresholdMonitor(cb func(), limit uint64, swap bool) MemoryMonitor {
	return func(cgroup cgroupsv1.Cgroup) (cgroupsv1.MemoryEvent, func(), error) {
		return cgroupsv1.MemoryThresholdEvent(limit, swap), cb, nil
	}
}

// MemoryPressureMonitor monitors memory pressure levels
func MemoryPressureMonitor(cb func(), level string) MemoryMonitor {
	return func(cgroup cgroupsv1.Cgroup) (cgroupsv1.MemoryEvent, func(), error) {
		return cgroupsv1.MemoryPressureEvent(cgroupsv1.MemoryPressureLevel(level), cgroupsv1.LocalMode), cb, nil
	}
}

type hostSubsystem struct {
	cgroupsv1.Subsystem
}

func hostHierarchy(hierarchy cgroupsv1.Hierarchy) cgroupsv1.Hierarchy {
	return func() ([]cgroupsv1.Subsystem, error) {
		subsystems, err := hierarchy()
		if err != nil {
			return nil, err
		}

		for i, subsystem := range subsystems {
			subsystems[i] = &hostSubsystem{
				Subsystem: subsystem,
			}
		}

		return subsystems, nil
	}
}

// NewMemoryController creates a new systemd cgroup based memory controller
func NewMemoryController(kind string, containerized bool, monitors ...MemoryMonitor) (*MemoryController, error) {
	path := cgroupsv1.NestedPath("")

	var cgroupHierarchy cgroupsv1.Hierarchy
	switch kind {
	case "systemd":
		cgroupHierarchy = cgroupsv1.Systemd
	case "v1":
		cgroupHierarchy = cgroupsv1.Default
	default:
		return nil, fmt.Errorf("unsupported cgroup hierarchy '%s'", kind)
	}

	if containerized {
		cgroupHierarchy = hostHierarchy(cgroupHierarchy)
	}

	cgroup, err := cgroupsv1.Load(path, cgroupsv1.WithHiearchy(cgroupHierarchy))
	if err != nil {
		return nil, fmt.Errorf("can't open memory cgroup: %w", err)
	}

	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, err
	}

	mc := &MemoryController{
		efd:          epfd,
		memoryEvents: make(map[int]func()),
	}

	for _, monitor := range monitors {
		memoryEvent, cb, err := monitor(cgroup)
		if err != nil {
			mc.Stop()
			return nil, err
		}

		efd, err := cgroup.RegisterMemoryEvent(memoryEvent)
		if err != nil {
			mc.Stop()
			return nil, fmt.Errorf("can't register memory event: %w", err)
		}

		var event syscall.EpollEvent
		event.Events = syscall.EPOLLIN
		event.Fd = int32(efd)

		if err := syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, int(efd), &event); err != nil {
			mc.Stop()
			return nil, fmt.Errorf("can't add file descriptor to epoll: %w", err)
		}

		mc.memoryEvents[int(efd)] = cb
	}

	return mc, nil
}

// Start listening for events
func (mc *MemoryController) Start() {
	go func() {
		var buf [256]byte
		var events [maxEpollEvents]syscall.EpollEvent

	EPOLLWAIT:
		for {
			nevents, err := syscall.EpollWait(mc.efd, events[:], -1)
			if err != nil {
				log.Warnf("Error while waiting for memory controller events: %v", err)
				break
			}

			for ev := 0; ev < nevents; ev++ {
				fd := int(events[ev].Fd)

				if _, err := syscall.Read(fd, buf[:]); err != nil {
					log.Warnf("Error while reading memory controller event: %v", err)
					continue EPOLLWAIT
				}

				mc.memoryEvents[fd]()
			}
		}
	}()
}

// Stop the memory controller
func (mc *MemoryController) Stop() {
	for fd := range mc.memoryEvents {
		syscall.Close(fd)
	}

	if mc.efd != 0 {
		syscall.Close(mc.efd)
	}
}
