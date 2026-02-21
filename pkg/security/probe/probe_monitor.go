// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/probe/eventstream"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/approver"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/cgroups"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/discarder"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/dns"
	"github.com/DataDog/datadog-agent/pkg/security/probe/monitors/syscalls"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// EBPFMonitors regroups all the work we want to do to monitor the probes we pushed in the kernel
type EBPFMonitors struct {
	ebpfProbe *EBPFProbe

	eventStreamMonitor *eventstream.Monitor
	discarderMonitor   *discarder.Monitor
	cgroupsMonitor     *cgroups.Monitor
	approverMonitor    *approver.Monitor
	syscallsMonitor    *syscalls.Monitor
	dnsMonitor         *dns.Monitor
}

// NewEBPFMonitors returns a new instance of a ProbeMonitor
func NewEBPFMonitors(p *EBPFProbe) *EBPFMonitors {
	return &EBPFMonitors{
		ebpfProbe: p,
	}
}

// Init initializes the monitor
func (m *EBPFMonitors) Init() error {
	var err error
	p := m.ebpfProbe

	// instantiate a new event statistics monitor
	m.eventStreamMonitor, err = eventstream.NewEventStreamMonitor(p.config.Probe, p.Erpc, p.Manager, p.statsdClient, p.onEventLost, p.useRingBuffers)
	if err != nil {
		return fmt.Errorf("couldn't create the events statistics monitor: %w", err)
	}

	m.discarderMonitor, err = discarder.NewDiscarderMonitor(p.Manager, p.statsdClient)
	if err != nil {
		return fmt.Errorf("couldn't create the discarder monitor: %w", err)
	}
	m.approverMonitor, err = approver.NewApproverMonitor(p.Manager, p.statsdClient)
	if err != nil {
		return fmt.Errorf("couldn't create the approver monitor: %w", err)
	}

	if p.opts.SyscallsMonitorEnabled {
		m.syscallsMonitor, err = syscalls.NewSyscallsMonitor(p.Manager, p.statsdClient)
		if err != nil {
			return fmt.Errorf("couldn't create the approver monitor: %w", err)
		}
	}

	m.cgroupsMonitor = cgroups.NewCgroupsMonitor(p.statsdClient, p.Resolvers.CGroupResolver)

	if p.config.Probe.DNSResolutionEnabled {
		m.dnsMonitor, err = dns.NewDNSMonitor(p.Manager, p.statsdClient)
		if err != nil {
			return fmt.Errorf("couldn't create the DNS monitor: %w", err)
		}
	}

	return nil
}

// GetEventStreamMonitor returns the perf buffer monitor
func (m *EBPFMonitors) GetEventStreamMonitor() *eventstream.Monitor {
	return m.eventStreamMonitor
}

// SendStats sends statistics about the probe to Datadog
func (m *EBPFMonitors) SendStats() error {
	if resolvers := m.ebpfProbe.Resolvers; resolvers != nil {
		if err := resolvers.ProcessResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send process_resolver stats: %w", err)
		}

		if err := resolvers.DentryResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send process_resolver stats: %w", err)
		}

		if err := resolvers.NamespaceResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send namespace_resolver stats: %w", err)
		}

		if err := resolvers.MountResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send mount_resolver stats: %w", err)
		}

		if err := resolvers.CGroupResolver.SendStats(); err != nil {
			return fmt.Errorf("failed to send cgroup_resolver stats: %w", err)
		}

		if resolvers.SBOMResolver != nil {
			if err := resolvers.SBOMResolver.SendStats(); err != nil {
				return fmt.Errorf("failed to send sbom_resolver stats: %w", err)
			}
		}

		if resolvers.HashResolver != nil {
			if err := resolvers.HashResolver.SendStats(); err != nil {
				return fmt.Errorf("failed to send hash_resolver stats: %w", err)
			}
		}

		if m.ebpfProbe.config.Probe.DNSResolutionEnabled {
			if err := m.dnsMonitor.SendStats(); err != nil {
				return fmt.Errorf("failed to send dns monitor stats: %w", err)
			}

			if err := resolvers.DNSResolver.SendStats(); err != nil {
				return fmt.Errorf("failed to send process_resolver stats: %w", err)
			}
		}

		if resolvers.FileMetadataResolver != nil {
			if err := resolvers.FileMetadataResolver.SendStats(); err != nil {
				return fmt.Errorf("failed to send file_resolver stats: %w", err)
			}
		}
	}

	if err := m.eventStreamMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send events stats: %w", err)
	}

	if err := m.discarderMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send discarder stats: %w", err)
	}

	if err := m.cgroupsMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send cgroups stats: %w", err)
	}

	if err := m.approverMonitor.SendStats(); err != nil {
		return fmt.Errorf("failed to send evaluation set stats: %w", err)
	}

	if m.ebpfProbe.opts.SyscallsMonitorEnabled {
		if err := m.syscallsMonitor.SendStats(); err != nil {
			return fmt.Errorf("failed to send evaluation set stats: %w", err)
		}
	}

	return nil
}

// ProcessEvent processes an event through the various monitors and controllers of the probe
func (m *EBPFMonitors) ProcessEvent(event *model.Event, scrubber *utils.Scrubber) {
	if !m.ebpfProbe.config.RuntimeSecurity.InternalMonitoringEnabled {
		return
	}

	// handle event errors
	if event.Error == nil {
		return
	}

	var notCritical *path.ErrPathResolutionNotCritical
	if errors.As(event.Error, &notCritical) {
		return
	}

	opts := m.ebpfProbe.evalOpts()

	if errors.Is(event.Error, model.ErrFailedDNSPacketDecoding) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewFailedDNSEvent(m.ebpfProbe.GetAgentContainerContext(), events.FailedDNSRuleID, events.AbnormalPathRuleDesc, event, opts),
		)
	}

	var pathErr *path.ErrPathResolution
	if errors.As(event.Error, &pathErr) {
		// Print detailed information about the abnormal path detection
		fmt.Printf("\n========== ABNORMAL PATH DETECTED ==========\n")
		fmt.Printf("Event Type: %s\n", event.GetType())
		fmt.Printf("Error: %v\n", pathErr.Err)
		fmt.Printf("Timestamp: %v\n", event.ResolveEventTime())

		// Print process context if available
		if event.ProcessContext != nil {
			proc := event.ProcessContext.Process
			fmt.Printf("\n--- Process Information ---\n")
			fmt.Printf("PID: %d\n", proc.Pid)
			fmt.Printf("Comm: %s\n", proc.Comm)
			fmt.Printf("Exe: %s\n", proc.FileEvent.PathnameStr)
			fmt.Printf("UID: %d, GID: %d\n", proc.UID, proc.GID)

			// Container info
			if proc.ContainerContext.ContainerID != "" {
				fmt.Printf("Container ID: %s\n", proc.ContainerContext.ContainerID)
			}

			// Parent process if available
			if event.ProcessContext.HasParent() && event.ProcessContext.Parent != nil {
				fmt.Printf("Parent PID: %d, Parent Comm: %s\n",
					event.ProcessContext.Parent.Pid,
					event.ProcessContext.Parent.Comm)
			}
		}

		// Print file-specific information based on event type
		fmt.Printf("\n--- File/Path Information ---\n")
		switch event.GetEventType() {
		case model.FileOpenEventType:
			fmt.Printf("Open File Path: %s (attempted)\n", event.Open.File.PathnameStr)
			fmt.Printf("Open Flags: 0x%x, Mode: 0%o\n", event.Open.Flags, event.Open.Mode)
			fmt.Printf("Inode: %d, MountID: %d\n", event.Open.File.Inode, event.Open.File.MountID)
			if event.Open.SyscallPath != "" {
				fmt.Printf("Syscall Path Arg: %s\n", event.Open.SyscallPath)
			}
		case model.FileChmodEventType:
			fmt.Printf("Chmod File Path: %s (attempted)\n", event.Chmod.File.PathnameStr)
			fmt.Printf("Inode: %d, MountID: %d\n", event.Chmod.File.Inode, event.Chmod.File.MountID)
		case model.FileChownEventType:
			fmt.Printf("Chown File Path: %s (attempted)\n", event.Chown.File.PathnameStr)
			fmt.Printf("Inode: %d, MountID: %d\n", event.Chown.File.Inode, event.Chown.File.MountID)
		case model.FileUnlinkEventType:
			fmt.Printf("Unlink File Path: %s (attempted)\n", event.Unlink.File.PathnameStr)
			fmt.Printf("Inode: %d, MountID: %d\n", event.Unlink.File.Inode, event.Unlink.File.MountID)
		case model.FileRenameEventType:
			fmt.Printf("Rename Old Path: %s (attempted)\n", event.Rename.Old.PathnameStr)
			fmt.Printf("Rename New Path: %s (attempted)\n", event.Rename.New.PathnameStr)
			fmt.Printf("Old Inode: %d, New Inode: %d\n", event.Rename.Old.Inode, event.Rename.New.Inode)
		case model.FileMkdirEventType:
			fmt.Printf("Mkdir Path: %s (attempted)\n", event.Mkdir.File.PathnameStr)
			fmt.Printf("Inode: %d, MountID: %d\n", event.Mkdir.File.Inode, event.Mkdir.File.MountID)
		case model.FileRmdirEventType:
			fmt.Printf("Rmdir Path: %s (attempted)\n", event.Rmdir.File.PathnameStr)
			fmt.Printf("Inode: %d, MountID: %d\n", event.Rmdir.File.Inode, event.Rmdir.File.MountID)
		case model.FileLinkEventType:
			fmt.Printf("Link Source: %s, Target: %s (attempted)\n", event.Link.Source.PathnameStr, event.Link.Target.PathnameStr)
		case model.ExecEventType:
			if event.Exec.Process != nil {
				fmt.Printf("Exec File Path: %s (attempted)\n", event.Exec.Process.FileEvent.PathnameStr)
				fmt.Printf("Inode: %d, MountID: %d\n", event.Exec.Process.FileEvent.Inode, event.Exec.Process.FileEvent.MountID)
			}
		default:
			fmt.Printf("Event-specific file info not displayed for type: %s\n", event.GetType())
		}

		fmt.Printf("============================================\n\n")

		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(m.ebpfProbe.GetAgentContainerContext(), events.AbnormalPathRuleID, events.AbnormalPathRuleDesc, event, scrubber, pathErr.Err, opts),
		)
	}

	if errors.Is(event.Error, model.ErrNoProcessContext) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(m.ebpfProbe.GetAgentContainerContext(), events.NoProcessContextErrorRuleID, events.NoProcessContextErrorRuleDesc, event, scrubber, event.Error, opts),
		)
	}

	var brokenLineageErr *model.ErrProcessBrokenLineage
	if errors.As(event.Error, &brokenLineageErr) {
		m.ebpfProbe.probe.DispatchCustomEvent(
			NewAbnormalEvent(m.ebpfProbe.GetAgentContainerContext(), events.BrokenProcessLineageErrorRuleID, events.BrokenProcessLineageErrorRuleDesc, event, scrubber, event.Error, opts),
		)
	}
}
