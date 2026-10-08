// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"net"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// snapshotExistingProcesses walks the process cache once at startup and seeds the open files and
// bound sockets of every containerized process that is already running, so behavior that predates
// tracing is captured. Processes that start later are seen live over ebpf and are not snapshotted.
func (m *ManagerV3) snapshotExistingProcesses() {
	if m.newEvent == nil {
		return
	}
	m.resolvers.ProcessResolver.Walk(func(entry *model.ProcessCacheEntry) {
		if entry.Process.ContainerContext.IsNull() {
			return
		}
		m.snapshotProcess(entry)
	})
}

func (m *ManagerV3) snapshotProcess(entry *model.ProcessCacheEntry) {
	workloadID := workloadIDFromEntry(entry)
	if workloadID == nil {
		return
	}
	tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
	if err != nil || utils.GetTagValue("image_tag", tags) == "" {
		return
	}

	p, err := process.NewProcess(int32(entry.Process.Pid))
	if err != nil {
		return
	}

	if slices.Contains(m.config.RuntimeSecurity.SecurityProfileV2EventTypes, model.FileOpenEventType) {
		m.snapshotFiles(entry, tags, p)
	}
	if slices.Contains(m.config.RuntimeSecurity.SecurityProfileV2EventTypes, model.BindEventType) {
		m.snapshotSockets(entry, tags, p)
	}
}

func (m *ManagerV3) snapshotFiles(entry *model.ProcessCacheEntry, tags []string, p *process.Process) {
	fileFDs, err := p.OpenFiles()
	if err != nil {
		seclog.Warnf("snapshot: error listing files (pid: %v): %s", entry.Process.Pid, err)
		return
	}

	for _, fd := range fileFDs {
		if !strings.HasPrefix(fd.Path, "/") {
			continue
		}

		resolved, err := filepath.EvalSymlinks(fd.Path)
		if err != nil || len(resolved) == 0 {
			resolved = fd.Path
		}

		evt := m.snapshotEvent(entry, tags)
		evt.Type = uint32(model.FileOpenEventType)
		evt.Open.File.SetPathnameStr(resolved)
		evt.Open.File.SetBasenameStr(path.Base(resolved))
		m.emitIfNovel(evt, true)
	}
}

func (m *ManagerV3) snapshotSockets(entry *model.ProcessCacheEntry, tags []string, p *process.Process) {
	boundSockets, err := procfs.NewBoundSocketSnapshotter().GetBoundSockets(p)
	if err != nil {
		seclog.Warnf("snapshot: error listing sockets (pid: %v): %s", entry.Process.Pid, err)
		return
	}

	for _, socket := range boundSockets {
		evt := m.snapshotEvent(entry, tags)
		evt.Type = uint32(model.BindEventType)
		evt.Bind.AddrFamily = socket.Family
		evt.Bind.Addr.IPNet.IP = socket.IP
		evt.Bind.Protocol = socket.Protocol
		if socket.Family == unix.AF_INET {
			evt.Bind.Addr.IPNet.Mask = net.CIDRMask(32, 32)
		} else {
			evt.Bind.Addr.IPNet.Mask = net.CIDRMask(128, 128)
		}
		evt.Bind.Addr.Port = socket.Port
		m.emitIfNovel(evt, true)
	}
}

// snapshotEvent builds a fresh event bound to the given process so emitIfNovel can walk its lineage
// and resolve its workload. Tags are stamped on the process context the same way the live path does.
func (m *ManagerV3) snapshotEvent(entry *model.ProcessCacheEntry, tags []string) *model.Event {
	evt := m.newEvent()
	evt.ProcessCacheEntry = entry
	evt.ProcessContext = &entry.ProcessContext
	evt.ProcessContext.Process.ContainerContext.Tags = tags
	return evt
}

func workloadIDFromEntry(entry *model.ProcessCacheEntry) containerutils.WorkloadID {
	if !entry.Process.ContainerContext.IsNull() {
		return entry.Process.ContainerContext.ContainerID
	}
	if entry.Process.CGroup.IsResolved() {
		return entry.Process.CGroup.CGroupID
	}
	return nil
}
