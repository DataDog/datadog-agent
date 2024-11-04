// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"net"
	"time"
)

// GetBindAddrFamily returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrFamily() uint16 {
	if ev.GetEventType().String() != "bind" {
		return uint16(0)
	}
	return ev.Bind.AddrFamily
}

// GetBindAddrIp returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrIp() net.IPNet {
	if ev.GetEventType().String() != "bind" {
		return net.IPNet{}
	}
	return ev.Bind.Addr.IPNet
}

// GetBindAddrPort returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrPort() uint16 {
	if ev.GetEventType().String() != "bind" {
		return uint16(0)
	}
	return ev.Bind.Addr.Port
}

// GetBindRetval returns the value of the field, resolving if necessary
func (ev *Event) GetBindRetval() int64 {
	if ev.GetEventType().String() != "bind" {
		return int64(0)
	}
	return ev.Bind.SyscallEvent.Retval
}

// GetBpfCmd returns the value of the field, resolving if necessary
func (ev *Event) GetBpfCmd() uint32 {
	if ev.GetEventType().String() != "bpf" {
		return uint32(0)
	}
	return ev.BPF.Cmd
}

// GetBpfMapName returns the value of the field, resolving if necessary
func (ev *Event) GetBpfMapName() string {
	if ev.GetEventType().String() != "bpf" {
		return ""
	}
	return ev.BPF.Map.Name
}

// GetBpfMapType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfMapType() uint32 {
	if ev.GetEventType().String() != "bpf" {
		return uint32(0)
	}
	return ev.BPF.Map.Type
}

// GetBpfProgAttachType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgAttachType() uint32 {
	if ev.GetEventType().String() != "bpf" {
		return uint32(0)
	}
	return ev.BPF.Program.AttachType
}

// GetBpfProgHelpers returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgHelpers() []uint32 {
	if ev.GetEventType().String() != "bpf" {
		return []uint32{}
	}
	return ev.BPF.Program.Helpers
}

// GetBpfProgName returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgName() string {
	if ev.GetEventType().String() != "bpf" {
		return ""
	}
	return ev.BPF.Program.Name
}

// GetBpfProgTag returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgTag() string {
	if ev.GetEventType().String() != "bpf" {
		return ""
	}
	return ev.BPF.Program.Tag
}

// GetBpfProgType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgType() uint32 {
	if ev.GetEventType().String() != "bpf" {
		return uint32(0)
	}
	return ev.BPF.Program.Type
}

// GetBpfRetval returns the value of the field, resolving if necessary
func (ev *Event) GetBpfRetval() int64 {
	if ev.GetEventType().String() != "bpf" {
		return int64(0)
	}
	return ev.BPF.SyscallEvent.Retval
}

// GetCapsetCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetCapsetCapEffective() uint64 {
	if ev.GetEventType().String() != "capset" {
		return uint64(0)
	}
	return ev.Capset.CapEffective
}

// GetCapsetCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetCapsetCapPermitted() uint64 {
	if ev.GetEventType().String() != "capset" {
		return uint64(0)
	}
	return ev.Capset.CapPermitted
}

// GetCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetCgroupFileInode() uint64 {
	return ev.CGroupContext.CGroupFile.Inode
}

// GetCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetCgroupFileMountId() uint32 {
	return ev.CGroupContext.CGroupFile.MountID
}

// GetCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetCgroupId() string {
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.CGroupContext)
}

// GetCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetCgroupManager() string {
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.CGroupContext)
}

// GetChdirFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileChangeTime() uint64 {
	if ev.GetEventType().String() != "chdir" {
		return uint64(0)
	}
	return ev.Chdir.File.FileFields.CTime
}

// GetChdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileFilesystem() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chdir.File)
}

// GetChdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileGid() uint32 {
	if ev.GetEventType().String() != "chdir" {
		return uint32(0)
	}
	return ev.Chdir.File.FileFields.GID
}

// GetChdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileGroup() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chdir.File.FileFields)
}

// GetChdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileHashes() []string {
	if ev.GetEventType().String() != "chdir" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chdir.File)
}

// GetChdirFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileInUpperLayer() bool {
	if ev.GetEventType().String() != "chdir" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chdir.File.FileFields)
}

// GetChdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileInode() uint64 {
	if ev.GetEventType().String() != "chdir" {
		return uint64(0)
	}
	return ev.Chdir.File.FileFields.PathKey.Inode
}

// GetChdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileMode() uint16 {
	if ev.GetEventType().String() != "chdir" {
		return uint16(0)
	}
	return ev.Chdir.File.FileFields.Mode
}

// GetChdirFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileModificationTime() uint64 {
	if ev.GetEventType().String() != "chdir" {
		return uint64(0)
	}
	return ev.Chdir.File.FileFields.MTime
}

// GetChdirFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileMountId() uint32 {
	if ev.GetEventType().String() != "chdir" {
		return uint32(0)
	}
	return ev.Chdir.File.FileFields.PathKey.MountID
}

// GetChdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileName() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chdir.File)
}

// GetChdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileNameLength() int {
	if ev.GetEventType().String() != "chdir" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chdir.File))
}

// GetChdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFilePackageName() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chdir.File)
}

// GetChdirFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chdir.File)
}

// GetChdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFilePackageVersion() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chdir.File)
}

// GetChdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFilePath() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chdir.File)
}

// GetChdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFilePathLength() int {
	if ev.GetEventType().String() != "chdir" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chdir.File))
}

// GetChdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileRights() int {
	if ev.GetEventType().String() != "chdir" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Chdir.File.FileFields)
}

// GetChdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileUid() uint32 {
	if ev.GetEventType().String() != "chdir" {
		return uint32(0)
	}
	return ev.Chdir.File.FileFields.UID
}

// GetChdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChdirFileUser() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chdir.File.FileFields)
}

// GetChdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChdirRetval() int64 {
	if ev.GetEventType().String() != "chdir" {
		return int64(0)
	}
	return ev.Chdir.SyscallEvent.Retval
}

// GetChdirSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallInt1() int {
	if ev.GetEventType().String() != "chdir" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Chdir.SyscallContext)
}

// GetChdirSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallInt2() int {
	if ev.GetEventType().String() != "chdir" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chdir.SyscallContext)
}

// GetChdirSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallInt3() int {
	if ev.GetEventType().String() != "chdir" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chdir.SyscallContext)
}

// GetChdirSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallPath() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chdir.SyscallContext)
}

// GetChdirSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallStr1() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chdir.SyscallContext)
}

// GetChdirSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallStr2() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Chdir.SyscallContext)
}

// GetChdirSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetChdirSyscallStr3() string {
	if ev.GetEventType().String() != "chdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Chdir.SyscallContext)
}

// GetChmodFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileChangeTime() uint64 {
	if ev.GetEventType().String() != "chmod" {
		return uint64(0)
	}
	return ev.Chmod.File.FileFields.CTime
}

// GetChmodFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileDestinationMode() uint32 {
	if ev.GetEventType().String() != "chmod" {
		return uint32(0)
	}
	return ev.Chmod.Mode
}

// GetChmodFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileDestinationRights() uint32 {
	if ev.GetEventType().String() != "chmod" {
		return uint32(0)
	}
	return ev.Chmod.Mode
}

// GetChmodFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileFilesystem() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File)
}

// GetChmodFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileGid() uint32 {
	if ev.GetEventType().String() != "chmod" {
		return uint32(0)
	}
	return ev.Chmod.File.FileFields.GID
}

// GetChmodFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileGroup() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileHashes() []string {
	if ev.GetEventType().String() != "chmod" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File)
}

// GetChmodFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileInUpperLayer() bool {
	if ev.GetEventType().String() != "chmod" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileInode() uint64 {
	if ev.GetEventType().String() != "chmod" {
		return uint64(0)
	}
	return ev.Chmod.File.FileFields.PathKey.Inode
}

// GetChmodFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileMode() uint16 {
	if ev.GetEventType().String() != "chmod" {
		return uint16(0)
	}
	return ev.Chmod.File.FileFields.Mode
}

// GetChmodFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileModificationTime() uint64 {
	if ev.GetEventType().String() != "chmod" {
		return uint64(0)
	}
	return ev.Chmod.File.FileFields.MTime
}

// GetChmodFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileMountId() uint32 {
	if ev.GetEventType().String() != "chmod" {
		return uint32(0)
	}
	return ev.Chmod.File.FileFields.PathKey.MountID
}

// GetChmodFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileName() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File)
}

// GetChmodFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileNameLength() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File))
}

// GetChmodFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageName() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File)
}

// GetChmodFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File)
}

// GetChmodFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageVersion() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File)
}

// GetChmodFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePath() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File)
}

// GetChmodFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePathLength() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File))
}

// GetChmodFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileRights() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileUid() uint32 {
	if ev.GetEventType().String() != "chmod" {
		return uint32(0)
	}
	return ev.Chmod.File.FileFields.UID
}

// GetChmodFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileUser() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields)
}

// GetChmodRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChmodRetval() int64 {
	if ev.GetEventType().String() != "chmod" {
		return int64(0)
	}
	return ev.Chmod.SyscallEvent.Retval
}

// GetChmodSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallInt1() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallInt2() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallInt3() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallMode() int {
	if ev.GetEventType().String() != "chmod" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallPath() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallStr1() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallStr2() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Chmod.SyscallContext)
}

// GetChmodSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetChmodSyscallStr3() string {
	if ev.GetEventType().String() != "chmod" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Chmod.SyscallContext)
}

// GetChownFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileChangeTime() uint64 {
	if ev.GetEventType().String() != "chown" {
		return uint64(0)
	}
	return ev.Chown.File.FileFields.CTime
}

// GetChownFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationGid() int64 {
	if ev.GetEventType().String() != "chown" {
		return int64(0)
	}
	return ev.Chown.GID
}

// GetChownFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationGroup() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown)
}

// GetChownFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationUid() int64 {
	if ev.GetEventType().String() != "chown" {
		return int64(0)
	}
	return ev.Chown.UID
}

// GetChownFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationUser() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown)
}

// GetChownFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileFilesystem() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File)
}

// GetChownFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileGid() uint32 {
	if ev.GetEventType().String() != "chown" {
		return uint32(0)
	}
	return ev.Chown.File.FileFields.GID
}

// GetChownFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileGroup() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields)
}

// GetChownFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileHashes() []string {
	if ev.GetEventType().String() != "chown" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File)
}

// GetChownFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileInUpperLayer() bool {
	if ev.GetEventType().String() != "chown" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chown.File.FileFields)
}

// GetChownFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileInode() uint64 {
	if ev.GetEventType().String() != "chown" {
		return uint64(0)
	}
	return ev.Chown.File.FileFields.PathKey.Inode
}

// GetChownFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileMode() uint16 {
	if ev.GetEventType().String() != "chown" {
		return uint16(0)
	}
	return ev.Chown.File.FileFields.Mode
}

// GetChownFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileModificationTime() uint64 {
	if ev.GetEventType().String() != "chown" {
		return uint64(0)
	}
	return ev.Chown.File.FileFields.MTime
}

// GetChownFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileMountId() uint32 {
	if ev.GetEventType().String() != "chown" {
		return uint32(0)
	}
	return ev.Chown.File.FileFields.PathKey.MountID
}

// GetChownFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileName() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File)
}

// GetChownFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileNameLength() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File))
}

// GetChownFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageName() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File)
}

// GetChownFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File)
}

// GetChownFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageVersion() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File)
}

// GetChownFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePath() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File)
}

// GetChownFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePathLength() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File))
}

// GetChownFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileRights() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Chown.File.FileFields)
}

// GetChownFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileUid() uint32 {
	if ev.GetEventType().String() != "chown" {
		return uint32(0)
	}
	return ev.Chown.File.FileFields.UID
}

// GetChownFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileUser() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields)
}

// GetChownRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChownRetval() int64 {
	if ev.GetEventType().String() != "chown" {
		return int64(0)
	}
	return ev.Chown.SyscallEvent.Retval
}

// GetChownSyscallGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallGid() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallInt1() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallInt2() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallInt3() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallPath() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallStr1() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallStr2() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallStr3() string {
	if ev.GetEventType().String() != "chown" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Chown.SyscallContext)
}

// GetChownSyscallUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownSyscallUid() int {
	if ev.GetEventType().String() != "chown" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chown.SyscallContext)
}

// GetConnectAddrFamily returns the value of the field, resolving if necessary
func (ev *Event) GetConnectAddrFamily() uint16 {
	if ev.GetEventType().String() != "connect" {
		return uint16(0)
	}
	return ev.Connect.AddrFamily
}

// GetConnectAddrIp returns the value of the field, resolving if necessary
func (ev *Event) GetConnectAddrIp() net.IPNet {
	if ev.GetEventType().String() != "connect" {
		return net.IPNet{}
	}
	return ev.Connect.Addr.IPNet
}

// GetConnectAddrPort returns the value of the field, resolving if necessary
func (ev *Event) GetConnectAddrPort() uint16 {
	if ev.GetEventType().String() != "connect" {
		return uint16(0)
	}
	return ev.Connect.Addr.Port
}

// GetConnectRetval returns the value of the field, resolving if necessary
func (ev *Event) GetConnectRetval() int64 {
	if ev.GetEventType().String() != "connect" {
		return int64(0)
	}
	return ev.Connect.SyscallEvent.Retval
}

// GetConnectServerAddrFamily returns the value of the field, resolving if necessary
func (ev *Event) GetConnectServerAddrFamily() uint16 {
	if ev.GetEventType().String() != "connect" {
		return uint16(0)
	}
	return ev.Connect.AddrFamily
}

// GetConnectServerAddrIp returns the value of the field, resolving if necessary
func (ev *Event) GetConnectServerAddrIp() net.IPNet {
	if ev.GetEventType().String() != "connect" {
		return net.IPNet{}
	}
	return ev.Connect.Addr.IPNet
}

// GetConnectServerAddrPort returns the value of the field, resolving if necessary
func (ev *Event) GetConnectServerAddrPort() uint16 {
	if ev.GetEventType().String() != "connect" {
		return uint16(0)
	}
	return ev.Connect.Addr.Port
}

// GetContainerCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetContainerCreatedAt() int {
	if ev.BaseEvent.ContainerContext == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext)
}

// GetContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetContainerId() string {
	if ev.BaseEvent.ContainerContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
}

// GetContainerRuntime returns the value of the field, resolving if necessary
func (ev *Event) GetContainerRuntime() string {
	if ev.BaseEvent.ContainerContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveContainerRuntime(ev, ev.BaseEvent.ContainerContext)
}

// GetContainerTags returns the value of the field, resolving if necessary
func (ev *Event) GetContainerTags() []string {
	if ev.BaseEvent.ContainerContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
}

// GetDnsId returns the value of the field, resolving if necessary
func (ev *Event) GetDnsId() uint16 {
	if ev.GetEventType().String() != "dns" {
		return uint16(0)
	}
	return ev.DNS.ID
}

// GetDnsQuestionClass returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionClass() uint16 {
	if ev.GetEventType().String() != "dns" {
		return uint16(0)
	}
	return ev.DNS.Class
}

// GetDnsQuestionCount returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionCount() uint16 {
	if ev.GetEventType().String() != "dns" {
		return uint16(0)
	}
	return ev.DNS.Count
}

// GetDnsQuestionLength returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionLength() uint16 {
	if ev.GetEventType().String() != "dns" {
		return uint16(0)
	}
	return ev.DNS.Size
}

// GetDnsQuestionName returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionName() string {
	if ev.GetEventType().String() != "dns" {
		return ""
	}
	return ev.DNS.Name
}

// GetDnsQuestionNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionNameLength() int {
	if ev.GetEventType().String() != "dns" {
		return 0
	}
	return len(ev.DNS.Name)
}

// GetDnsQuestionType returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionType() uint16 {
	if ev.GetEventType().String() != "dns" {
		return uint16(0)
	}
	return ev.DNS.Type
}

// GetEventAsync returns the value of the field, resolving if necessary
func (ev *Event) GetEventAsync() bool {
	return ev.FieldHandlers.ResolveAsync(ev)
}

// GetEventHostname returns the value of the field, resolving if necessary
func (ev *Event) GetEventHostname() string {
	return ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent)
}

// GetEventOrigin returns the value of the field, resolving if necessary
func (ev *Event) GetEventOrigin() string {
	return ev.BaseEvent.Origin
}

// GetEventOs returns the value of the field, resolving if necessary
func (ev *Event) GetEventOs() string {
	return ev.BaseEvent.Os
}

// GetEventService returns the value of the field, resolving if necessary
func (ev *Event) GetEventService() string {
	return ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
}

// GetEventTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetEventTimestamp() int {
	return ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)
}

// GetExecArgs returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgs() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process)
}

// GetExecArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsFlags() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process)
}

// GetExecArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsOptions() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process)
}

// GetExecArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsScrubbed() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, ev.Exec.Process)
}

// GetExecArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsTruncated() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process)
}

// GetExecArgv returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgv() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process)
}

// GetExecArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgv0() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process)
}

// GetExecArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgvScrubbed() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.Exec.Process)
}

// GetExecAuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecAuid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.AUID
}

// GetExecCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetExecCapEffective() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	return ev.Exec.Process.Credentials.CapEffective
}

// GetExecCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetExecCapPermitted() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	return ev.Exec.Process.Credentials.CapPermitted
}

// GetExecCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecCgroupFileInode() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	return ev.Exec.Process.CGroup.CGroupFile.Inode
}

// GetExecCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExecCgroupFileMountId() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.CGroup.CGroupFile.MountID
}

// GetExecCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetExecCgroupId() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.Exec.Process.CGroup)
}

// GetExecCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetExecCgroupManager() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Exec.Process.CGroup)
}

// GetExecCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetExecCmdargv() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, ev.Exec.Process)
}

// GetExecComm returns the value of the field, resolving if necessary
func (ev *Event) GetExecComm() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Comm
}

// GetExecContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExecContainerId() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, ev.Exec.Process)
}

// GetExecCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetExecCreatedAt() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)
}

// GetExecEgid returns the value of the field, resolving if necessary
func (ev *Event) GetExecEgid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.EGID
}

// GetExecEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecEgroup() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Credentials.EGroup
}

// GetExecEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvp() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
}

// GetExecEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvs() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
}

// GetExecEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvsTruncated() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process)
}

// GetExecEuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecEuid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.EUID
}

// GetExecEuser returns the value of the field, resolving if necessary
func (ev *Event) GetExecEuser() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Credentials.EUser
}

// GetExecExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecExecTime() time.Time {
	if ev.GetEventType().String() != "exec" {
		return time.Time{}
	}
	if ev.Exec.Process == nil {
		return time.Time{}
	}
	return ev.Exec.Process.ExecTime
}

// GetExecExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecExitTime() time.Time {
	if ev.GetEventType().String() != "exec" {
		return time.Time{}
	}
	if ev.Exec.Process == nil {
		return time.Time{}
	}
	return ev.Exec.Process.ExitTime
}

// GetExecFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileChangeTime() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.CTime
}

// GetExecFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileFilesystem() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileGid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.GID
}

// GetExecFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileGroup() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileHashes() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	if !ev.Exec.Process.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileInUpperLayer() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	if !ev.Exec.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileInode() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.PathKey.Inode
}

// GetExecFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileMode() uint16 {
	if ev.GetEventType().String() != "exec" {
		return uint16(0)
	}
	if ev.Exec.Process == nil {
		return uint16(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.Mode
}

// GetExecFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileModificationTime() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.MTime
}

// GetExecFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileMountId() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.PathKey.MountID
}

// GetExecFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileName() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileNameLength() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
}

// GetExecFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageName() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageVersion() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePath() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePathLength() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
}

// GetExecFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileRights() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	if !ev.Exec.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileUid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.UID
}

// GetExecFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileUser() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecForkTime() time.Time {
	if ev.GetEventType().String() != "exec" {
		return time.Time{}
	}
	if ev.Exec.Process == nil {
		return time.Time{}
	}
	return ev.Exec.Process.ForkTime
}

// GetExecFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsgid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.FSGID
}

// GetExecFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsgroup() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Credentials.FSGroup
}

// GetExecFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsuid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.FSUID
}

// GetExecFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsuser() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Credentials.FSUser
}

// GetExecGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecGid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.GID
}

// GetExecGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecGroup() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Credentials.Group
}

// GetExecInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileChangeTime() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetExecInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileFilesystem() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileGid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetExecInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileGroup() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	if !ev.Exec.Process.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileInUpperLayer() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	if !ev.Exec.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileInode() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetExecInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileMode() uint16 {
	if ev.GetEventType().String() != "exec" {
		return uint16(0)
	}
	if ev.Exec.Process == nil {
		return uint16(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetExecInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileModificationTime() uint64 {
	if ev.GetEventType().String() != "exec" {
		return uint64(0)
	}
	if ev.Exec.Process == nil {
		return uint64(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetExecInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileMountId() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetExecInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileName() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileNameLength() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
}

// GetExecInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageName() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageVersion() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePath() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePathLength() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
}

// GetExecInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileRights() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	if ev.Exec.Process == nil {
		return 0
	}
	if !ev.Exec.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileUid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetExecInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileUser() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetExecIsExec() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	return ev.Exec.Process.IsExec
}

// GetExecIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetExecIsKworker() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	return ev.Exec.Process.PIDContext.IsKworker
}

// GetExecIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetExecIsThread() bool {
	if ev.GetEventType().String() != "exec" {
		return false
	}
	if ev.Exec.Process == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exec.Process)
}

// GetExecPid returns the value of the field, resolving if necessary
func (ev *Event) GetExecPid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.PIDContext.Pid
}

// GetExecPpid returns the value of the field, resolving if necessary
func (ev *Event) GetExecPpid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.PPid
}

// GetExecSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallInt1() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Exec.SyscallContext)
}

// GetExecSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallInt2() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Exec.SyscallContext)
}

// GetExecSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallInt3() int {
	if ev.GetEventType().String() != "exec" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Exec.SyscallContext)
}

// GetExecSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallPath() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Exec.SyscallContext)
}

// GetExecSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallStr1() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Exec.SyscallContext)
}

// GetExecSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallStr2() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Exec.SyscallContext)
}

// GetExecSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetExecSyscallStr3() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Exec.SyscallContext)
}

// GetExecTid returns the value of the field, resolving if necessary
func (ev *Event) GetExecTid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.PIDContext.Tid
}

// GetExecTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetExecTtyName() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.TTYName
}

// GetExecUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecUid() uint32 {
	if ev.GetEventType().String() != "exec" {
		return uint32(0)
	}
	if ev.Exec.Process == nil {
		return uint32(0)
	}
	return ev.Exec.Process.Credentials.UID
}

// GetExecUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecUser() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.Credentials.User
}

// GetExecUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetExecUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "exec" {
		return []string{}
	}
	if ev.Exec.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exec.Process.UserSession)
}

// GetExecUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecUserSessionK8sUid() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exec.Process.UserSession)
}

// GetExecUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetExecUserSessionK8sUsername() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exec.Process.UserSession)
}

// GetExitArgs returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgs() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
}

// GetExitArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsFlags() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process)
}

// GetExitArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsOptions() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process)
}

// GetExitArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsScrubbed() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, ev.Exit.Process)
}

// GetExitArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsTruncated() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
}

// GetExitArgv returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgv() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process)
}

// GetExitArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgv0() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
}

// GetExitArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgvScrubbed() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.Exit.Process)
}

// GetExitAuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitAuid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.AUID
}

// GetExitCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetExitCapEffective() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	return ev.Exit.Process.Credentials.CapEffective
}

// GetExitCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetExitCapPermitted() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	return ev.Exit.Process.Credentials.CapPermitted
}

// GetExitCause returns the value of the field, resolving if necessary
func (ev *Event) GetExitCause() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	return ev.Exit.Cause
}

// GetExitCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCgroupFileInode() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	return ev.Exit.Process.CGroup.CGroupFile.Inode
}

// GetExitCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExitCgroupFileMountId() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.CGroup.CGroupFile.MountID
}

// GetExitCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetExitCgroupId() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.Exit.Process.CGroup)
}

// GetExitCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetExitCgroupManager() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Exit.Process.CGroup)
}

// GetExitCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetExitCmdargv() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, ev.Exit.Process)
}

// GetExitCode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCode() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	return ev.Exit.Code
}

// GetExitComm returns the value of the field, resolving if necessary
func (ev *Event) GetExitComm() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Comm
}

// GetExitContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExitContainerId() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, ev.Exit.Process)
}

// GetExitCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetExitCreatedAt() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
}

// GetExitEgid returns the value of the field, resolving if necessary
func (ev *Event) GetExitEgid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.EGID
}

// GetExitEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitEgroup() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Credentials.EGroup
}

// GetExitEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvp() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
}

// GetExitEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvs() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
}

// GetExitEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvsTruncated() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
}

// GetExitEuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitEuid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.EUID
}

// GetExitEuser returns the value of the field, resolving if necessary
func (ev *Event) GetExitEuser() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Credentials.EUser
}

// GetExitExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitExecTime() time.Time {
	if ev.GetEventType().String() != "exit" {
		return time.Time{}
	}
	if ev.Exit.Process == nil {
		return time.Time{}
	}
	return ev.Exit.Process.ExecTime
}

// GetExitExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitExitTime() time.Time {
	if ev.GetEventType().String() != "exit" {
		return time.Time{}
	}
	if ev.Exit.Process == nil {
		return time.Time{}
	}
	return ev.Exit.Process.ExitTime
}

// GetExitFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileChangeTime() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.CTime
}

// GetExitFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileFilesystem() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileGid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.GID
}

// GetExitFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileGroup() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileHashes() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	if !ev.Exit.Process.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileInUpperLayer() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	if !ev.Exit.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileInode() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.PathKey.Inode
}

// GetExitFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileMode() uint16 {
	if ev.GetEventType().String() != "exit" {
		return uint16(0)
	}
	if ev.Exit.Process == nil {
		return uint16(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.Mode
}

// GetExitFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileModificationTime() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.MTime
}

// GetExitFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileMountId() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.PathKey.MountID
}

// GetExitFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileName() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileNameLength() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
}

// GetExitFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageName() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageVersion() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePath() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePathLength() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
}

// GetExitFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileRights() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	if !ev.Exit.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileUid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.UID
}

// GetExitFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileUser() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitForkTime() time.Time {
	if ev.GetEventType().String() != "exit" {
		return time.Time{}
	}
	if ev.Exit.Process == nil {
		return time.Time{}
	}
	return ev.Exit.Process.ForkTime
}

// GetExitFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsgid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.FSGID
}

// GetExitFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsgroup() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Credentials.FSGroup
}

// GetExitFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsuid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.FSUID
}

// GetExitFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsuser() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Credentials.FSUser
}

// GetExitGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitGid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.GID
}

// GetExitGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitGroup() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Credentials.Group
}

// GetExitInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileChangeTime() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetExitInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileFilesystem() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileGid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetExitInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileGroup() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	if !ev.Exit.Process.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileInUpperLayer() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	if !ev.Exit.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileInode() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetExitInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileMode() uint16 {
	if ev.GetEventType().String() != "exit" {
		return uint16(0)
	}
	if ev.Exit.Process == nil {
		return uint16(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetExitInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileModificationTime() uint64 {
	if ev.GetEventType().String() != "exit" {
		return uint64(0)
	}
	if ev.Exit.Process == nil {
		return uint64(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetExitInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileMountId() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetExitInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileName() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileNameLength() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
}

// GetExitInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageName() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageVersion() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePath() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePathLength() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
}

// GetExitInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileRights() int {
	if ev.GetEventType().String() != "exit" {
		return 0
	}
	if ev.Exit.Process == nil {
		return 0
	}
	if !ev.Exit.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileUid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetExitInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileUser() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetExitIsExec() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	return ev.Exit.Process.IsExec
}

// GetExitIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetExitIsKworker() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	return ev.Exit.Process.PIDContext.IsKworker
}

// GetExitIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetExitIsThread() bool {
	if ev.GetEventType().String() != "exit" {
		return false
	}
	if ev.Exit.Process == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exit.Process)
}

// GetExitPid returns the value of the field, resolving if necessary
func (ev *Event) GetExitPid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.PIDContext.Pid
}

// GetExitPpid returns the value of the field, resolving if necessary
func (ev *Event) GetExitPpid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.PPid
}

// GetExitTid returns the value of the field, resolving if necessary
func (ev *Event) GetExitTid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.PIDContext.Tid
}

// GetExitTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetExitTtyName() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.TTYName
}

// GetExitUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitUid() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	if ev.Exit.Process == nil {
		return uint32(0)
	}
	return ev.Exit.Process.Credentials.UID
}

// GetExitUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitUser() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.Credentials.User
}

// GetExitUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetExitUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "exit" {
		return []string{}
	}
	if ev.Exit.Process == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exit.Process.UserSession)
}

// GetExitUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitUserSessionK8sUid() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exit.Process.UserSession)
}

// GetExitUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetExitUserSessionK8sUsername() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exit.Process.UserSession)
}

// GetImdsAwsIsImdsV2 returns the value of the field, resolving if necessary
func (ev *Event) GetImdsAwsIsImdsV2() bool {
	if ev.GetEventType().String() != "imds" {
		return false
	}
	return ev.IMDS.AWS.IsIMDSv2
}

// GetImdsAwsSecurityCredentialsType returns the value of the field, resolving if necessary
func (ev *Event) GetImdsAwsSecurityCredentialsType() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.AWS.SecurityCredentials.Type
}

// GetImdsCloudProvider returns the value of the field, resolving if necessary
func (ev *Event) GetImdsCloudProvider() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.CloudProvider
}

// GetImdsHost returns the value of the field, resolving if necessary
func (ev *Event) GetImdsHost() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.Host
}

// GetImdsServer returns the value of the field, resolving if necessary
func (ev *Event) GetImdsServer() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.Server
}

// GetImdsType returns the value of the field, resolving if necessary
func (ev *Event) GetImdsType() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.Type
}

// GetImdsUrl returns the value of the field, resolving if necessary
func (ev *Event) GetImdsUrl() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.URL
}

// GetImdsUserAgent returns the value of the field, resolving if necessary
func (ev *Event) GetImdsUserAgent() string {
	if ev.GetEventType().String() != "imds" {
		return ""
	}
	return ev.IMDS.UserAgent
}

// GetLinkFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileChangeTime() uint64 {
	if ev.GetEventType().String() != "link" {
		return uint64(0)
	}
	return ev.Link.Source.FileFields.CTime
}

// GetLinkFileDestinationChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationChangeTime() uint64 {
	if ev.GetEventType().String() != "link" {
		return uint64(0)
	}
	return ev.Link.Target.FileFields.CTime
}

// GetLinkFileDestinationFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationFilesystem() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target)
}

// GetLinkFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationGid() uint32 {
	if ev.GetEventType().String() != "link" {
		return uint32(0)
	}
	return ev.Link.Target.FileFields.GID
}

// GetLinkFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationGroup() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationHashes() []string {
	if ev.GetEventType().String() != "link" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target)
}

// GetLinkFileDestinationInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationInUpperLayer() bool {
	if ev.GetEventType().String() != "link" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationInode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationInode() uint64 {
	if ev.GetEventType().String() != "link" {
		return uint64(0)
	}
	return ev.Link.Target.FileFields.PathKey.Inode
}

// GetLinkFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationMode() uint16 {
	if ev.GetEventType().String() != "link" {
		return uint16(0)
	}
	return ev.Link.Target.FileFields.Mode
}

// GetLinkFileDestinationModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationModificationTime() uint64 {
	if ev.GetEventType().String() != "link" {
		return uint64(0)
	}
	return ev.Link.Target.FileFields.MTime
}

// GetLinkFileDestinationMountId returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationMountId() uint32 {
	if ev.GetEventType().String() != "link" {
		return uint32(0)
	}
	return ev.Link.Target.FileFields.PathKey.MountID
}

// GetLinkFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationName() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target)
}

// GetLinkFileDestinationNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationNameLength() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target))
}

// GetLinkFileDestinationPackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageName() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageSourceVersion() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageVersion() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPath() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPathLength() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target))
}

// GetLinkFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationRights() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationUid() uint32 {
	if ev.GetEventType().String() != "link" {
		return uint32(0)
	}
	return ev.Link.Target.FileFields.UID
}

// GetLinkFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationUser() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileFilesystem() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source)
}

// GetLinkFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileGid() uint32 {
	if ev.GetEventType().String() != "link" {
		return uint32(0)
	}
	return ev.Link.Source.FileFields.GID
}

// GetLinkFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileGroup() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileHashes() []string {
	if ev.GetEventType().String() != "link" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source)
}

// GetLinkFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileInUpperLayer() bool {
	if ev.GetEventType().String() != "link" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileInode() uint64 {
	if ev.GetEventType().String() != "link" {
		return uint64(0)
	}
	return ev.Link.Source.FileFields.PathKey.Inode
}

// GetLinkFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileMode() uint16 {
	if ev.GetEventType().String() != "link" {
		return uint16(0)
	}
	return ev.Link.Source.FileFields.Mode
}

// GetLinkFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileModificationTime() uint64 {
	if ev.GetEventType().String() != "link" {
		return uint64(0)
	}
	return ev.Link.Source.FileFields.MTime
}

// GetLinkFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileMountId() uint32 {
	if ev.GetEventType().String() != "link" {
		return uint32(0)
	}
	return ev.Link.Source.FileFields.PathKey.MountID
}

// GetLinkFileName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileName() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source)
}

// GetLinkFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileNameLength() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source))
}

// GetLinkFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageName() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source)
}

// GetLinkFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source)
}

// GetLinkFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageVersion() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source)
}

// GetLinkFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePath() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source)
}

// GetLinkFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePathLength() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source))
}

// GetLinkFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileRights() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileUid() uint32 {
	if ev.GetEventType().String() != "link" {
		return uint32(0)
	}
	return ev.Link.Source.FileFields.UID
}

// GetLinkFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileUser() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields)
}

// GetLinkRetval returns the value of the field, resolving if necessary
func (ev *Event) GetLinkRetval() int64 {
	if ev.GetEventType().String() != "link" {
		return int64(0)
	}
	return ev.Link.SyscallEvent.Retval
}

// GetLinkSyscallDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallDestinationPath() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallInt1() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallInt2() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallInt3() int {
	if ev.GetEventType().String() != "link" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallPath() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallStr1() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallStr2() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Link.SyscallContext)
}

// GetLinkSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetLinkSyscallStr3() string {
	if ev.GetEventType().String() != "link" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Link.SyscallContext)
}

// GetLoadModuleArgs returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleArgs() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule)
}

// GetLoadModuleArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleArgsTruncated() bool {
	if ev.GetEventType().String() != "load_module" {
		return false
	}
	return ev.LoadModule.ArgsTruncated
}

// GetLoadModuleArgv returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleArgv() []string {
	if ev.GetEventType().String() != "load_module" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule)
}

// GetLoadModuleFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileChangeTime() uint64 {
	if ev.GetEventType().String() != "load_module" {
		return uint64(0)
	}
	return ev.LoadModule.File.FileFields.CTime
}

// GetLoadModuleFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileFilesystem() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File)
}

// GetLoadModuleFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileGid() uint32 {
	if ev.GetEventType().String() != "load_module" {
		return uint32(0)
	}
	return ev.LoadModule.File.FileFields.GID
}

// GetLoadModuleFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileGroup() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileHashes() []string {
	if ev.GetEventType().String() != "load_module" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File)
}

// GetLoadModuleFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileInUpperLayer() bool {
	if ev.GetEventType().String() != "load_module" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileInode() uint64 {
	if ev.GetEventType().String() != "load_module" {
		return uint64(0)
	}
	return ev.LoadModule.File.FileFields.PathKey.Inode
}

// GetLoadModuleFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileMode() uint16 {
	if ev.GetEventType().String() != "load_module" {
		return uint16(0)
	}
	return ev.LoadModule.File.FileFields.Mode
}

// GetLoadModuleFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileModificationTime() uint64 {
	if ev.GetEventType().String() != "load_module" {
		return uint64(0)
	}
	return ev.LoadModule.File.FileFields.MTime
}

// GetLoadModuleFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileMountId() uint32 {
	if ev.GetEventType().String() != "load_module" {
		return uint32(0)
	}
	return ev.LoadModule.File.FileFields.PathKey.MountID
}

// GetLoadModuleFileName returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileName() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File)
}

// GetLoadModuleFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileNameLength() int {
	if ev.GetEventType().String() != "load_module" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File))
}

// GetLoadModuleFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePackageName() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePackageVersion() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePath() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePathLength() int {
	if ev.GetEventType().String() != "load_module" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File))
}

// GetLoadModuleFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileRights() int {
	if ev.GetEventType().String() != "load_module" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileUid() uint32 {
	if ev.GetEventType().String() != "load_module" {
		return uint32(0)
	}
	return ev.LoadModule.File.FileFields.UID
}

// GetLoadModuleFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileUser() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleLoadedFromMemory returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleLoadedFromMemory() bool {
	if ev.GetEventType().String() != "load_module" {
		return false
	}
	return ev.LoadModule.LoadedFromMemory
}

// GetLoadModuleName returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleName() string {
	if ev.GetEventType().String() != "load_module" {
		return ""
	}
	return ev.LoadModule.Name
}

// GetLoadModuleRetval returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleRetval() int64 {
	if ev.GetEventType().String() != "load_module" {
		return int64(0)
	}
	return ev.LoadModule.SyscallEvent.Retval
}

// GetMkdirFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileChangeTime() uint64 {
	if ev.GetEventType().String() != "mkdir" {
		return uint64(0)
	}
	return ev.Mkdir.File.FileFields.CTime
}

// GetMkdirFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileDestinationMode() uint32 {
	if ev.GetEventType().String() != "mkdir" {
		return uint32(0)
	}
	return ev.Mkdir.Mode
}

// GetMkdirFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileDestinationRights() uint32 {
	if ev.GetEventType().String() != "mkdir" {
		return uint32(0)
	}
	return ev.Mkdir.Mode
}

// GetMkdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileFilesystem() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File)
}

// GetMkdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileGid() uint32 {
	if ev.GetEventType().String() != "mkdir" {
		return uint32(0)
	}
	return ev.Mkdir.File.FileFields.GID
}

// GetMkdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileGroup() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileHashes() []string {
	if ev.GetEventType().String() != "mkdir" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File)
}

// GetMkdirFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileInUpperLayer() bool {
	if ev.GetEventType().String() != "mkdir" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileInode() uint64 {
	if ev.GetEventType().String() != "mkdir" {
		return uint64(0)
	}
	return ev.Mkdir.File.FileFields.PathKey.Inode
}

// GetMkdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileMode() uint16 {
	if ev.GetEventType().String() != "mkdir" {
		return uint16(0)
	}
	return ev.Mkdir.File.FileFields.Mode
}

// GetMkdirFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileModificationTime() uint64 {
	if ev.GetEventType().String() != "mkdir" {
		return uint64(0)
	}
	return ev.Mkdir.File.FileFields.MTime
}

// GetMkdirFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileMountId() uint32 {
	if ev.GetEventType().String() != "mkdir" {
		return uint32(0)
	}
	return ev.Mkdir.File.FileFields.PathKey.MountID
}

// GetMkdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileName() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File)
}

// GetMkdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileNameLength() int {
	if ev.GetEventType().String() != "mkdir" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File))
}

// GetMkdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageName() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File)
}

// GetMkdirFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File)
}

// GetMkdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageVersion() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File)
}

// GetMkdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePath() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File)
}

// GetMkdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePathLength() int {
	if ev.GetEventType().String() != "mkdir" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File))
}

// GetMkdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileRights() int {
	if ev.GetEventType().String() != "mkdir" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileUid() uint32 {
	if ev.GetEventType().String() != "mkdir" {
		return uint32(0)
	}
	return ev.Mkdir.File.FileFields.UID
}

// GetMkdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileUser() string {
	if ev.GetEventType().String() != "mkdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirRetval() int64 {
	if ev.GetEventType().String() != "mkdir" {
		return int64(0)
	}
	return ev.Mkdir.SyscallEvent.Retval
}

// GetMmapFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileChangeTime() uint64 {
	if ev.GetEventType().String() != "mmap" {
		return uint64(0)
	}
	return ev.MMap.File.FileFields.CTime
}

// GetMmapFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileFilesystem() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File)
}

// GetMmapFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileGid() uint32 {
	if ev.GetEventType().String() != "mmap" {
		return uint32(0)
	}
	return ev.MMap.File.FileFields.GID
}

// GetMmapFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileGroup() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileHashes() []string {
	if ev.GetEventType().String() != "mmap" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File)
}

// GetMmapFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileInUpperLayer() bool {
	if ev.GetEventType().String() != "mmap" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileInode() uint64 {
	if ev.GetEventType().String() != "mmap" {
		return uint64(0)
	}
	return ev.MMap.File.FileFields.PathKey.Inode
}

// GetMmapFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileMode() uint16 {
	if ev.GetEventType().String() != "mmap" {
		return uint16(0)
	}
	return ev.MMap.File.FileFields.Mode
}

// GetMmapFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileModificationTime() uint64 {
	if ev.GetEventType().String() != "mmap" {
		return uint64(0)
	}
	return ev.MMap.File.FileFields.MTime
}

// GetMmapFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileMountId() uint32 {
	if ev.GetEventType().String() != "mmap" {
		return uint32(0)
	}
	return ev.MMap.File.FileFields.PathKey.MountID
}

// GetMmapFileName returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileName() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File)
}

// GetMmapFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileNameLength() int {
	if ev.GetEventType().String() != "mmap" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File))
}

// GetMmapFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageName() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File)
}

// GetMmapFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File)
}

// GetMmapFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageVersion() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File)
}

// GetMmapFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePath() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File)
}

// GetMmapFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePathLength() int {
	if ev.GetEventType().String() != "mmap" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File))
}

// GetMmapFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileRights() int {
	if ev.GetEventType().String() != "mmap" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileUid() uint32 {
	if ev.GetEventType().String() != "mmap" {
		return uint32(0)
	}
	return ev.MMap.File.FileFields.UID
}

// GetMmapFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileUser() string {
	if ev.GetEventType().String() != "mmap" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields)
}

// GetMmapFlags returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFlags() uint64 {
	if ev.GetEventType().String() != "mmap" {
		return uint64(0)
	}
	return ev.MMap.Flags
}

// GetMmapProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMmapProtection() uint64 {
	if ev.GetEventType().String() != "mmap" {
		return uint64(0)
	}
	return ev.MMap.Protection
}

// GetMmapRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMmapRetval() int64 {
	if ev.GetEventType().String() != "mmap" {
		return int64(0)
	}
	return ev.MMap.SyscallEvent.Retval
}

// GetMountFsType returns the value of the field, resolving if necessary
func (ev *Event) GetMountFsType() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.Mount.Mount.FSType
}

// GetMountMountpointPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountMountpointPath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount)
}

// GetMountRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMountRetval() int64 {
	if ev.GetEventType().String() != "mount" {
		return int64(0)
	}
	return ev.Mount.SyscallEvent.Retval
}

// GetMountRootPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountRootPath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveMountRootPath(ev, &ev.Mount)
}

// GetMountSourcePath returns the value of the field, resolving if necessary
func (ev *Event) GetMountSourcePath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount)
}

// GetMountSyscallFsType returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallFsType() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallInt1() int {
	if ev.GetEventType().String() != "mount" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallInt2() int {
	if ev.GetEventType().String() != "mount" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallInt3() int {
	if ev.GetEventType().String() != "mount" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallMountpointPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallMountpointPath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallSourcePath returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallSourcePath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallStr1() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallStr2() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Mount.SyscallContext)
}

// GetMountSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetMountSyscallStr3() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Mount.SyscallContext)
}

// GetMprotectReqProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectReqProtection() int {
	if ev.GetEventType().String() != "mprotect" {
		return 0
	}
	return ev.MProtect.ReqProtection
}

// GetMprotectRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectRetval() int64 {
	if ev.GetEventType().String() != "mprotect" {
		return int64(0)
	}
	return ev.MProtect.SyscallEvent.Retval
}

// GetMprotectVmProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectVmProtection() int {
	if ev.GetEventType().String() != "mprotect" {
		return 0
	}
	return ev.MProtect.VMProtection
}

// GetNetworkDestinationIp returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDestinationIp() net.IPNet {
	return ev.NetworkContext.Destination.IPNet
}

// GetNetworkDestinationPort returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDestinationPort() uint16 {
	return ev.NetworkContext.Destination.Port
}

// GetNetworkDeviceIfname returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDeviceIfname() string {
	return ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkContext.Device)
}

// GetNetworkL3Protocol returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkL3Protocol() uint16 {
	return ev.NetworkContext.L3Protocol
}

// GetNetworkL4Protocol returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkL4Protocol() uint16 {
	return ev.NetworkContext.L4Protocol
}

// GetNetworkSize returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSize() uint32 {
	return ev.NetworkContext.Size
}

// GetNetworkSourceIp returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSourceIp() net.IPNet {
	return ev.NetworkContext.Source.IPNet
}

// GetNetworkSourcePort returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSourcePort() uint16 {
	return ev.NetworkContext.Source.Port
}

// GetOndemandArg1Str returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg1Str() string {
	if ev.GetEventType().String() != "ondemand" {
		return ""
	}
	return ev.FieldHandlers.ResolveOnDemandArg1Str(ev, &ev.OnDemand)
}

// GetOndemandArg1Uint returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg1Uint() int {
	if ev.GetEventType().String() != "ondemand" {
		return 0
	}
	return ev.FieldHandlers.ResolveOnDemandArg1Uint(ev, &ev.OnDemand)
}

// GetOndemandArg2Str returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg2Str() string {
	if ev.GetEventType().String() != "ondemand" {
		return ""
	}
	return ev.FieldHandlers.ResolveOnDemandArg2Str(ev, &ev.OnDemand)
}

// GetOndemandArg2Uint returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg2Uint() int {
	if ev.GetEventType().String() != "ondemand" {
		return 0
	}
	return ev.FieldHandlers.ResolveOnDemandArg2Uint(ev, &ev.OnDemand)
}

// GetOndemandArg3Str returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg3Str() string {
	if ev.GetEventType().String() != "ondemand" {
		return ""
	}
	return ev.FieldHandlers.ResolveOnDemandArg3Str(ev, &ev.OnDemand)
}

// GetOndemandArg3Uint returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg3Uint() int {
	if ev.GetEventType().String() != "ondemand" {
		return 0
	}
	return ev.FieldHandlers.ResolveOnDemandArg3Uint(ev, &ev.OnDemand)
}

// GetOndemandArg4Str returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg4Str() string {
	if ev.GetEventType().String() != "ondemand" {
		return ""
	}
	return ev.FieldHandlers.ResolveOnDemandArg4Str(ev, &ev.OnDemand)
}

// GetOndemandArg4Uint returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandArg4Uint() int {
	if ev.GetEventType().String() != "ondemand" {
		return 0
	}
	return ev.FieldHandlers.ResolveOnDemandArg4Uint(ev, &ev.OnDemand)
}

// GetOndemandName returns the value of the field, resolving if necessary
func (ev *Event) GetOndemandName() string {
	if ev.GetEventType().String() != "ondemand" {
		return ""
	}
	return ev.FieldHandlers.ResolveOnDemandName(ev, &ev.OnDemand)
}

// GetOpenFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileChangeTime() uint64 {
	if ev.GetEventType().String() != "open" {
		return uint64(0)
	}
	return ev.Open.File.FileFields.CTime
}

// GetOpenFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileDestinationMode() uint32 {
	if ev.GetEventType().String() != "open" {
		return uint32(0)
	}
	return ev.Open.Mode
}

// GetOpenFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileFilesystem() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File)
}

// GetOpenFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileGid() uint32 {
	if ev.GetEventType().String() != "open" {
		return uint32(0)
	}
	return ev.Open.File.FileFields.GID
}

// GetOpenFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileGroup() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields)
}

// GetOpenFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileHashes() []string {
	if ev.GetEventType().String() != "open" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File)
}

// GetOpenFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileInUpperLayer() bool {
	if ev.GetEventType().String() != "open" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Open.File.FileFields)
}

// GetOpenFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileInode() uint64 {
	if ev.GetEventType().String() != "open" {
		return uint64(0)
	}
	return ev.Open.File.FileFields.PathKey.Inode
}

// GetOpenFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileMode() uint16 {
	if ev.GetEventType().String() != "open" {
		return uint16(0)
	}
	return ev.Open.File.FileFields.Mode
}

// GetOpenFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileModificationTime() uint64 {
	if ev.GetEventType().String() != "open" {
		return uint64(0)
	}
	return ev.Open.File.FileFields.MTime
}

// GetOpenFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileMountId() uint32 {
	if ev.GetEventType().String() != "open" {
		return uint32(0)
	}
	return ev.Open.File.FileFields.PathKey.MountID
}

// GetOpenFileName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileName() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File)
}

// GetOpenFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileNameLength() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File))
}

// GetOpenFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageName() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File)
}

// GetOpenFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File)
}

// GetOpenFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageVersion() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File)
}

// GetOpenFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePath() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File)
}

// GetOpenFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePathLength() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File))
}

// GetOpenFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileRights() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Open.File.FileFields)
}

// GetOpenFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileUid() uint32 {
	if ev.GetEventType().String() != "open" {
		return uint32(0)
	}
	return ev.Open.File.FileFields.UID
}

// GetOpenFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileUser() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields)
}

// GetOpenFlags returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFlags() uint32 {
	if ev.GetEventType().String() != "open" {
		return uint32(0)
	}
	return ev.Open.Flags
}

// GetOpenRetval returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRetval() int64 {
	if ev.GetEventType().String() != "open" {
		return int64(0)
	}
	return ev.Open.SyscallEvent.Retval
}

// GetOpenSyscallFlags returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallFlags() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallInt1() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallInt2() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallInt3() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallMode() int {
	if ev.GetEventType().String() != "open" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallPath() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallStr1() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallStr2() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Open.SyscallContext)
}

// GetOpenSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetOpenSyscallStr3() string {
	if ev.GetEventType().String() != "open" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Open.SyscallContext)
}

// GetPacketDestinationIp returns the value of the field, resolving if necessary
func (ev *Event) GetPacketDestinationIp() net.IPNet {
	if ev.GetEventType().String() != "packet" {
		return net.IPNet{}
	}
	return ev.RawPacket.NetworkContext.Destination.IPNet
}

// GetPacketDestinationPort returns the value of the field, resolving if necessary
func (ev *Event) GetPacketDestinationPort() uint16 {
	if ev.GetEventType().String() != "packet" {
		return uint16(0)
	}
	return ev.RawPacket.NetworkContext.Destination.Port
}

// GetPacketDeviceIfname returns the value of the field, resolving if necessary
func (ev *Event) GetPacketDeviceIfname() string {
	if ev.GetEventType().String() != "packet" {
		return ""
	}
	return ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.RawPacket.NetworkContext.Device)
}

// GetPacketFilter returns the value of the field, resolving if necessary
func (ev *Event) GetPacketFilter() string {
	if ev.GetEventType().String() != "packet" {
		return ""
	}
	return ev.RawPacket.Filter
}

// GetPacketL3Protocol returns the value of the field, resolving if necessary
func (ev *Event) GetPacketL3Protocol() uint16 {
	if ev.GetEventType().String() != "packet" {
		return uint16(0)
	}
	return ev.RawPacket.NetworkContext.L3Protocol
}

// GetPacketL4Protocol returns the value of the field, resolving if necessary
func (ev *Event) GetPacketL4Protocol() uint16 {
	if ev.GetEventType().String() != "packet" {
		return uint16(0)
	}
	return ev.RawPacket.NetworkContext.L4Protocol
}

// GetPacketSize returns the value of the field, resolving if necessary
func (ev *Event) GetPacketSize() uint32 {
	if ev.GetEventType().String() != "packet" {
		return uint32(0)
	}
	return ev.RawPacket.NetworkContext.Size
}

// GetPacketSourceIp returns the value of the field, resolving if necessary
func (ev *Event) GetPacketSourceIp() net.IPNet {
	if ev.GetEventType().String() != "packet" {
		return net.IPNet{}
	}
	return ev.RawPacket.NetworkContext.Source.IPNet
}

// GetPacketSourcePort returns the value of the field, resolving if necessary
func (ev *Event) GetPacketSourcePort() uint16 {
	if ev.GetEventType().String() != "packet" {
		return uint16(0)
	}
	return ev.RawPacket.NetworkContext.Source.Port
}

// GetPacketTlsVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPacketTlsVersion() uint16 {
	if ev.GetEventType().String() != "packet" {
		return uint16(0)
	}
	return ev.RawPacket.TLSContext.Version
}

// GetProcessAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgs() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgsFlags() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgsOptions() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgsScrubbed() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgsTruncated() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgv0() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgvScrubbed() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsAuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsAuid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.AUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCapEffective() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.CapEffective
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCapPermitted() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.CapPermitted
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCgroupFileInode() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.CGroup.CGroupFile.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCgroupFileMountId() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.CGroup.CGroupFile.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCgroupId() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveCGroupID(ev, &element.ProcessContext.Process.CGroup)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCgroupManager() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveCGroupManager(ev, &element.ProcessContext.Process.CGroup)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCmdargv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessCmdArgv(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsComm() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Comm
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsContainerId() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessContainerID(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCreatedAt() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEgid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EGID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEgroup() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EGroup
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvp() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvs() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvsTruncated() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEuid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEuser() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EUser
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileChangeTime() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.CTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileFilesystem() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileGid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileGroup() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileHashes() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileInUpperLayer() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileInode() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileMode() []uint16 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint16{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint16{}
	}
	var values []uint16
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.Mode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileModificationTime() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.MTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileMountId() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileName() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileNameLength() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFilePackageName() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFilePackageSourceVersion() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFilePackageVersion() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFilePath() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFilePathLength() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileRights() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileUid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileUser() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFsgid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSGID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFsgroup() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSGroup
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFsuid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFsuser() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSUser
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsGid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsGroup() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.Group
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileChangeTime() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileFilesystem() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileGid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileGroup() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileHashes() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileInUpperLayer() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileInode() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileMode() []uint16 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint16{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint16{}
	}
	var values []uint16
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileModificationTime() []uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint64{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileMountId() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileName() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileNameLength() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFilePackageName() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFilePackageSourceVersion() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFilePackageVersion() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFilePath() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFilePathLength() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileRights() []int {
	if ev.BaseEvent.ProcessContext == nil {
		return []int{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileUid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileUser() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsIsExec() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.IsExec
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsIsKworker() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.IsKworker
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsIsThread() []bool {
	if ev.BaseEvent.ProcessContext == nil {
		return []bool{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return 0
	}
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	return iterator.Len(ctx)
}

// GetProcessAncestorsPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsPid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.Pid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsPpid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PPid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsTid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.Tid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsTtyName() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.TTYName
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUid() []uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return []uint32{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUser() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.User
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUserSessionK8sGroups() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUserSessionK8sUid() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUserSessionK8sUsername() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgs() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsFlags() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsOptions() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsScrubbed() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsTruncated() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgv0() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgvScrubbed() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessAuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAuid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.AUID
}

// GetProcessCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCapEffective() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.CapEffective
}

// GetProcessCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCapPermitted() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.CapPermitted
}

// GetProcessCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCgroupFileInode() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.CGroup.CGroupFile.Inode
}

// GetProcessCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCgroupFileMountId() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.CGroup.CGroupFile.MountID
}

// GetProcessCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCgroupId() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.BaseEvent.ProcessContext.Process.CGroup)
}

// GetProcessCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCgroupManager() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.BaseEvent.ProcessContext.Process.CGroup)
}

// GetProcessCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCmdargv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessComm() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Comm
}

// GetProcessContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessContainerId() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCreatedAt() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEgid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EGID
}

// GetProcessEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEgroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EGroup
}

// GetProcessEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvp() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvs() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvsTruncated() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEuid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EUID
}

// GetProcessEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEuser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EUser
}

// GetProcessExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessExecTime() time.Time {
	if ev.BaseEvent.ProcessContext == nil {
		return time.Time{}
	}
	return ev.BaseEvent.ProcessContext.Process.ExecTime
}

// GetProcessExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessExitTime() time.Time {
	if ev.BaseEvent.ProcessContext == nil {
		return time.Time{}
	}
	return ev.BaseEvent.ProcessContext.Process.ExitTime
}

// GetProcessFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileChangeTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.CTime
}

// GetProcessFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileFilesystem() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileGid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.GID
}

// GetProcessFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileGroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileHashes() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileInUpperLayer() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileInode() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode
}

// GetProcessFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileMode() uint16 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint16(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.Mode
}

// GetProcessFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileModificationTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.MTime
}

// GetProcessFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileMountId() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID
}

// GetProcessFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileNameLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
}

// GetProcessFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageSourceVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePath() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePathLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
}

// GetProcessFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileRights() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileUid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.UID
}

// GetProcessFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessForkTime() time.Time {
	if ev.BaseEvent.ProcessContext == nil {
		return time.Time{}
	}
	return ev.BaseEvent.ProcessContext.Process.ForkTime
}

// GetProcessFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsgid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSGID
}

// GetProcessFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsgroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSGroup
}

// GetProcessFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsuid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSUID
}

// GetProcessFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsuser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSUser
}

// GetProcessGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessGid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.GID
}

// GetProcessGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessGroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.Group
}

// GetProcessInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileChangeTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetProcessInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileFilesystem() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileGid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetProcessInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileGroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileHashes() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileInUpperLayer() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileInode() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetProcessInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileMode() uint16 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint16(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetProcessInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileModificationTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetProcessInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileMountId() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetProcessInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileNameLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
}

// GetProcessInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageSourceVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePath() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePathLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
}

// GetProcessInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileRights() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileUid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetProcessInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIsExec() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	return ev.BaseEvent.ProcessContext.Process.IsExec
}

// GetProcessIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIsKworker() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	return ev.BaseEvent.ProcessContext.Process.PIDContext.IsKworker
}

// GetProcessIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIsThread() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgs() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsFlags() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsOptions() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsScrubbed() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsTruncated() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgv0() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgvScrubbed() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentAuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentAuid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.AUID
}

// GetProcessParentCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCapEffective() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.CapEffective
}

// GetProcessParentCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCapPermitted() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.CapPermitted
}

// GetProcessParentCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCgroupFileInode() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupFile.Inode
}

// GetProcessParentCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCgroupFileMountId() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupFile.MountID
}

// GetProcessParentCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCgroupId() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.BaseEvent.ProcessContext.Parent.CGroup)
}

// GetProcessParentCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCgroupManager() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.BaseEvent.ProcessContext.Parent.CGroup)
}

// GetProcessParentCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCmdargv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentComm() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Comm
}

// GetProcessParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentContainerId() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCreatedAt() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEgid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EGID
}

// GetProcessParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEgroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EGroup
}

// GetProcessParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvp() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvs() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvsTruncated() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEuid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EUID
}

// GetProcessParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEuser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EUser
}

// GetProcessParentFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileChangeTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.CTime
}

// GetProcessParentFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileFilesystem() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileGid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.GID
}

// GetProcessParentFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileGroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
}

// GetProcessParentFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileHashes() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileInUpperLayer() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
}

// GetProcessParentFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileInode() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.PathKey.Inode
}

// GetProcessParentFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileMode() uint16 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint16(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint16(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint16(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint16(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.Mode
}

// GetProcessParentFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileModificationTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.MTime
}

// GetProcessParentFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileMountId() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.PathKey.MountID
}

// GetProcessParentFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileNameLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
}

// GetProcessParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageSourceVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePath() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePathLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
}

// GetProcessParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileRights() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
}

// GetProcessParentFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileUid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.UID
}

// GetProcessParentFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
}

// GetProcessParentFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsgid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSGID
}

// GetProcessParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsgroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSGroup
}

// GetProcessParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsuid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSUID
}

// GetProcessParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsuser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSUser
}

// GetProcessParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentGid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.GID
}

// GetProcessParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentGroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.Group
}

// GetProcessParentInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileChangeTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetProcessParentInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileFilesystem() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileGid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID
}

// GetProcessParentInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileGroup() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessParentInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileHashes() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileInUpperLayer() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessParentInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileInode() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetProcessParentInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileMode() uint16 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint16(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint16(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint16(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint16(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetProcessParentInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileModificationTime() uint64 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint64(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetProcessParentInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileMountId() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetProcessParentInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileNameLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
}

// GetProcessParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageSourceVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageVersion() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePath() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePathLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
}

// GetProcessParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileRights() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessParentInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileUid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID
}

// GetProcessParentInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessParentIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIsExec() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.BaseEvent.ProcessContext.Parent.IsExec
}

// GetProcessParentIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIsKworker() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.IsKworker
}

// GetProcessParentIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIsThread() bool {
	if ev.BaseEvent.ProcessContext == nil {
		return false
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return false
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentPid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid
}

// GetProcessParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentPpid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.PPid
}

// GetProcessParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentTid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.Tid
}

// GetProcessParentTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentTtyName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.TTYName
}

// GetProcessParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return uint32(0)
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.UID
}

// GetProcessParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.User
}

// GetProcessParentUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUserSessionK8sGroups() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession)
}

// GetProcessParentUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUserSessionK8sUid() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession)
}

// GetProcessParentUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUserSessionK8sUsername() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession)
}

// GetProcessPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessPid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.PIDContext.Pid
}

// GetProcessPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessPpid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.PPid
}

// GetProcessTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessTid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.PIDContext.Tid
}

// GetProcessTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessTtyName() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.TTYName
}

// GetProcessUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUid() uint32 {
	if ev.BaseEvent.ProcessContext == nil {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.UID
}

// GetProcessUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.User
}

// GetProcessUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUserSessionK8sGroups() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Process.UserSession)
}

// GetProcessUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUserSessionK8sUid() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Process.UserSession)
}

// GetProcessUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUserSessionK8sUsername() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Process.UserSession)
}

// GetPtraceRequest returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceRequest() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	return ev.PTrace.Request
}

// GetPtraceRetval returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceRetval() int64 {
	if ev.GetEventType().String() != "ptrace" {
		return int64(0)
	}
	return ev.PTrace.SyscallEvent.Retval
}

// GetPtraceTraceeAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgs() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgsFlags() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgsOptions() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgsScrubbed() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgsTruncated() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgv() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgv0() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgvScrubbed() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsAuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsAuid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.AUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCapEffective() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.CapEffective
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCapPermitted() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.CapPermitted
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCgroupFileInode() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.CGroup.CGroupFile.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCgroupFileMountId() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.CGroup.CGroupFile.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCgroupId() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveCGroupID(ev, &element.ProcessContext.Process.CGroup)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCgroupManager() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveCGroupManager(ev, &element.ProcessContext.Process.CGroup)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCmdargv() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessCmdArgv(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsComm() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Comm
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsContainerId() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessContainerID(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCreatedAt() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEgid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EGID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEgroup() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EGroup
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEnvp() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEnvs() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEnvsTruncated() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEuid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEuser() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EUser
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileChangeTime() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.CTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileFilesystem() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileGid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileGroup() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileHashes() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileInUpperLayer() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileInode() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileMode() []uint16 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint16{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint16{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint16{}
	}
	var values []uint16
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.Mode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileModificationTime() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.MTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileMountId() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileName() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileNameLength() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFilePackageName() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFilePackageSourceVersion() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFilePackageVersion() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFilePath() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFilePathLength() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileRights() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileUid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileUser() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFsgid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSGID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFsgroup() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSGroup
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFsuid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFsuser() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSUser
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsGid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsGroup() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.Group
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileChangeTime() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileFilesystem() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileGid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileGroup() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileInUpperLayer() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileInode() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileMode() []uint16 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint16{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint16{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint16{}
	}
	var values []uint16
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileModificationTime() []uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint64{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint64{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileMountId() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileName() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileNameLength() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFilePackageName() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFilePackageSourceVersion() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFilePackageVersion() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFilePath() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFilePathLength() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileRights() []int {
	if ev.GetEventType().String() != "ptrace" {
		return []int{}
	}
	if ev.PTrace.Tracee == nil {
		return []int{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileUid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileUser() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsIsExec() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.IsExec
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsIsKworker() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.IsKworker
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsIsThread() []bool {
	if ev.GetEventType().String() != "ptrace" {
		return []bool{}
	}
	if ev.PTrace.Tracee == nil {
		return []bool{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return 0
	}
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	return iterator.Len(ctx)
}

// GetPtraceTraceeAncestorsPid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsPid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.Pid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsPpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsPpid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PPid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsTid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.Tid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsTtyName() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.TTYName
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsUid() []uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return []uint32{}
	}
	if ev.PTrace.Tracee == nil {
		return []uint32{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsUser() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.User
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsUserSessionK8sUid() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsUserSessionK8sUsername() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgs() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsFlags() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsOptions() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsScrubbed() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsTruncated() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgv() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgv0() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgvScrubbed() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeAuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAuid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.AUID
}

// GetPtraceTraceeCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCapEffective() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.CapEffective
}

// GetPtraceTraceeCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCapPermitted() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.CapPermitted
}

// GetPtraceTraceeCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCgroupFileInode() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.CGroup.CGroupFile.Inode
}

// GetPtraceTraceeCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCgroupFileMountId() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.CGroup.CGroupFile.MountID
}

// GetPtraceTraceeCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCgroupId() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.PTrace.Tracee.Process.CGroup)
}

// GetPtraceTraceeCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCgroupManager() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.PTrace.Tracee.Process.CGroup)
}

// GetPtraceTraceeCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCmdargv() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeComm() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Comm
}

// GetPtraceTraceeContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeContainerId() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCreatedAt() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEgid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.EGID
}

// GetPtraceTraceeEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEgroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Credentials.EGroup
}

// GetPtraceTraceeEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvp() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvs() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvsTruncated() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEuid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.EUID
}

// GetPtraceTraceeEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEuser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Credentials.EUser
}

// GetPtraceTraceeExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeExecTime() time.Time {
	if ev.GetEventType().String() != "ptrace" {
		return time.Time{}
	}
	if ev.PTrace.Tracee == nil {
		return time.Time{}
	}
	return ev.PTrace.Tracee.Process.ExecTime
}

// GetPtraceTraceeExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeExitTime() time.Time {
	if ev.GetEventType().String() != "ptrace" {
		return time.Time{}
	}
	if ev.PTrace.Tracee == nil {
		return time.Time{}
	}
	return ev.PTrace.Tracee.Process.ExitTime
}

// GetPtraceTraceeFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileChangeTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.CTime
}

// GetPtraceTraceeFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileFilesystem() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileGid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.GID
}

// GetPtraceTraceeFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileGroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileHashes() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileInUpperLayer() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileInode() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.Inode
}

// GetPtraceTraceeFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileMode() uint16 {
	if ev.GetEventType().String() != "ptrace" {
		return uint16(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint16(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.Mode
}

// GetPtraceTraceeFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileModificationTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.MTime
}

// GetPtraceTraceeFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileMountId() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.MountID
}

// GetPtraceTraceeFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileNameLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent))
}

// GetPtraceTraceeFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePath() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePathLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent))
}

// GetPtraceTraceeFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileRights() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileUid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.UID
}

// GetPtraceTraceeFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileUser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeForkTime() time.Time {
	if ev.GetEventType().String() != "ptrace" {
		return time.Time{}
	}
	if ev.PTrace.Tracee == nil {
		return time.Time{}
	}
	return ev.PTrace.Tracee.Process.ForkTime
}

// GetPtraceTraceeFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsgid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.FSGID
}

// GetPtraceTraceeFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsgroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Credentials.FSGroup
}

// GetPtraceTraceeFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsuid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.FSUID
}

// GetPtraceTraceeFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsuser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Credentials.FSUser
}

// GetPtraceTraceeGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeGid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.GID
}

// GetPtraceTraceeGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeGroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Credentials.Group
}

// GetPtraceTraceeInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileChangeTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetPtraceTraceeInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileFilesystem() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileGid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetPtraceTraceeInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileGroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileInUpperLayer() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileInode() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetPtraceTraceeInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileMode() uint16 {
	if ev.GetEventType().String() != "ptrace" {
		return uint16(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint16(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetPtraceTraceeInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileModificationTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetPtraceTraceeInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileMountId() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetPtraceTraceeInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileNameLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePath() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePathLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileRights() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileUid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetPtraceTraceeInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileUser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIsExec() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	return ev.PTrace.Tracee.Process.IsExec
}

// GetPtraceTraceeIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIsKworker() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	return ev.PTrace.Tracee.Process.PIDContext.IsKworker
}

// GetPtraceTraceeIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIsThread() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgs() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsFlags() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsOptions() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsScrubbed() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsTruncated() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgv() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgv0() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgvScrubbed() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentAuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentAuid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.AUID
}

// GetPtraceTraceeParentCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCapEffective() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.CapEffective
}

// GetPtraceTraceeParentCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCapPermitted() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.CapPermitted
}

// GetPtraceTraceeParentCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCgroupFileInode() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.CGroup.CGroupFile.Inode
}

// GetPtraceTraceeParentCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCgroupFileMountId() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.CGroup.CGroupFile.MountID
}

// GetPtraceTraceeParentCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCgroupId() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.PTrace.Tracee.Parent.CGroup)
}

// GetPtraceTraceeParentCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCgroupManager() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.PTrace.Tracee.Parent.CGroup)
}

// GetPtraceTraceeParentCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCmdargv() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentComm() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Comm
}

// GetPtraceTraceeParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentContainerId() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCreatedAt() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	if !ev.PTrace.Tracee.HasParent() {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEgid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.EGID
}

// GetPtraceTraceeParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEgroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.EGroup
}

// GetPtraceTraceeParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvp() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvs() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvsTruncated() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEuid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.EUID
}

// GetPtraceTraceeParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEuser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.EUser
}

// GetPtraceTraceeParentFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileChangeTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.CTime
}

// GetPtraceTraceeParentFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileFilesystem() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileGid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.GID
}

// GetPtraceTraceeParentFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileGroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
}

// GetPtraceTraceeParentFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileHashes() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileInUpperLayer() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
}

// GetPtraceTraceeParentFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileInode() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.PathKey.Inode
}

// GetPtraceTraceeParentFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileMode() uint16 {
	if ev.GetEventType().String() != "ptrace" {
		return uint16(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint16(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint16(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint16(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint16(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.Mode
}

// GetPtraceTraceeParentFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileModificationTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.MTime
}

// GetPtraceTraceeParentFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileMountId() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.PathKey.MountID
}

// GetPtraceTraceeParentFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileNameLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent))
}

// GetPtraceTraceeParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePath() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePathLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent))
}

// GetPtraceTraceeParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileRights() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	if !ev.PTrace.Tracee.HasParent() {
		return 0
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
}

// GetPtraceTraceeParentFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileUid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.FileEvent.FileFields.UID
}

// GetPtraceTraceeParentFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileUser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
}

// GetPtraceTraceeParentFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsgid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSGID
}

// GetPtraceTraceeParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsgroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSGroup
}

// GetPtraceTraceeParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsuid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSUID
}

// GetPtraceTraceeParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsuser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSUser
}

// GetPtraceTraceeParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentGid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.GID
}

// GetPtraceTraceeParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentGroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.Group
}

// GetPtraceTraceeParentInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileChangeTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetPtraceTraceeParentInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileFilesystem() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileGid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.GID
}

// GetPtraceTraceeParentInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileGroup() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeParentInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileInUpperLayer() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeParentInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileInode() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetPtraceTraceeParentInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileMode() uint16 {
	if ev.GetEventType().String() != "ptrace" {
		return uint16(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint16(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint16(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint16(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint16(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetPtraceTraceeParentInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileModificationTime() uint64 {
	if ev.GetEventType().String() != "ptrace" {
		return uint64(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint64(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetPtraceTraceeParentInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileMountId() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetPtraceTraceeParentInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileNameLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageVersion() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePath() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePathLength() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileRights() int {
	if ev.GetEventType().String() != "ptrace" {
		return 0
	}
	if ev.PTrace.Tracee == nil {
		return 0
	}
	if ev.PTrace.Tracee.Parent == nil {
		return 0
	}
	if !ev.PTrace.Tracee.HasParent() {
		return 0
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeParentInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileUid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.UID
}

// GetPtraceTraceeParentInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileUser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeParentIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIsExec() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.PTrace.Tracee.Parent.IsExec
}

// GetPtraceTraceeParentIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIsKworker() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.PTrace.Tracee.Parent.PIDContext.IsKworker
}

// GetPtraceTraceeParentIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIsThread() bool {
	if ev.GetEventType().String() != "ptrace" {
		return false
	}
	if ev.PTrace.Tracee == nil {
		return false
	}
	if ev.PTrace.Tracee.Parent == nil {
		return false
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentPid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.PIDContext.Pid
}

// GetPtraceTraceeParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentPpid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.PPid
}

// GetPtraceTraceeParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentTid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.PIDContext.Tid
}

// GetPtraceTraceeParentTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentTtyName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.TTYName
}

// GetPtraceTraceeParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	if ev.PTrace.Tracee.Parent == nil {
		return uint32(0)
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.UID
}

// GetPtraceTraceeParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.User
}

// GetPtraceTraceeParentUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	if ev.PTrace.Tracee.Parent == nil {
		return []string{}
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Parent.UserSession)
}

// GetPtraceTraceeParentUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUserSessionK8sUid() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Parent.UserSession)
}

// GetPtraceTraceeParentUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUserSessionK8sUsername() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	if ev.PTrace.Tracee.Parent == nil {
		return ""
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Parent.UserSession)
}

// GetPtraceTraceePid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceePid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.PIDContext.Pid
}

// GetPtraceTraceePpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceePpid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.PPid
}

// GetPtraceTraceeTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeTid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.PIDContext.Tid
}

// GetPtraceTraceeTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeTtyName() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.TTYName
}

// GetPtraceTraceeUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUid() uint32 {
	if ev.GetEventType().String() != "ptrace" {
		return uint32(0)
	}
	if ev.PTrace.Tracee == nil {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.Credentials.UID
}

// GetPtraceTraceeUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUser() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.PTrace.Tracee.Process.Credentials.User
}

// GetPtraceTraceeUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "ptrace" {
		return []string{}
	}
	if ev.PTrace.Tracee == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Process.UserSession)
}

// GetPtraceTraceeUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUserSessionK8sUid() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Process.UserSession)
}

// GetPtraceTraceeUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUserSessionK8sUsername() string {
	if ev.GetEventType().String() != "ptrace" {
		return ""
	}
	if ev.PTrace.Tracee == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Process.UserSession)
}

// GetRemovexattrFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileChangeTime() uint64 {
	if ev.GetEventType().String() != "removexattr" {
		return uint64(0)
	}
	return ev.RemoveXAttr.File.FileFields.CTime
}

// GetRemovexattrFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileDestinationName() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr)
}

// GetRemovexattrFileDestinationNamespace returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileDestinationNamespace() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr)
}

// GetRemovexattrFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileFilesystem() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileGid() uint32 {
	if ev.GetEventType().String() != "removexattr" {
		return uint32(0)
	}
	return ev.RemoveXAttr.File.FileFields.GID
}

// GetRemovexattrFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileGroup() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileHashes() []string {
	if ev.GetEventType().String() != "removexattr" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileInUpperLayer() bool {
	if ev.GetEventType().String() != "removexattr" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileInode() uint64 {
	if ev.GetEventType().String() != "removexattr" {
		return uint64(0)
	}
	return ev.RemoveXAttr.File.FileFields.PathKey.Inode
}

// GetRemovexattrFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileMode() uint16 {
	if ev.GetEventType().String() != "removexattr" {
		return uint16(0)
	}
	return ev.RemoveXAttr.File.FileFields.Mode
}

// GetRemovexattrFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileModificationTime() uint64 {
	if ev.GetEventType().String() != "removexattr" {
		return uint64(0)
	}
	return ev.RemoveXAttr.File.FileFields.MTime
}

// GetRemovexattrFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileMountId() uint32 {
	if ev.GetEventType().String() != "removexattr" {
		return uint32(0)
	}
	return ev.RemoveXAttr.File.FileFields.PathKey.MountID
}

// GetRemovexattrFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileName() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileNameLength() int {
	if ev.GetEventType().String() != "removexattr" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File))
}

// GetRemovexattrFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageName() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageVersion() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePath() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePathLength() int {
	if ev.GetEventType().String() != "removexattr" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File))
}

// GetRemovexattrFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileRights() int {
	if ev.GetEventType().String() != "removexattr" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileUid() uint32 {
	if ev.GetEventType().String() != "removexattr" {
		return uint32(0)
	}
	return ev.RemoveXAttr.File.FileFields.UID
}

// GetRemovexattrFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileUser() string {
	if ev.GetEventType().String() != "removexattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrRetval() int64 {
	if ev.GetEventType().String() != "removexattr" {
		return int64(0)
	}
	return ev.RemoveXAttr.SyscallEvent.Retval
}

// GetRenameFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileChangeTime() uint64 {
	if ev.GetEventType().String() != "rename" {
		return uint64(0)
	}
	return ev.Rename.Old.FileFields.CTime
}

// GetRenameFileDestinationChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationChangeTime() uint64 {
	if ev.GetEventType().String() != "rename" {
		return uint64(0)
	}
	return ev.Rename.New.FileFields.CTime
}

// GetRenameFileDestinationFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationFilesystem() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New)
}

// GetRenameFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationGid() uint32 {
	if ev.GetEventType().String() != "rename" {
		return uint32(0)
	}
	return ev.Rename.New.FileFields.GID
}

// GetRenameFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationGroup() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationHashes() []string {
	if ev.GetEventType().String() != "rename" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New)
}

// GetRenameFileDestinationInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationInUpperLayer() bool {
	if ev.GetEventType().String() != "rename" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationInode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationInode() uint64 {
	if ev.GetEventType().String() != "rename" {
		return uint64(0)
	}
	return ev.Rename.New.FileFields.PathKey.Inode
}

// GetRenameFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationMode() uint16 {
	if ev.GetEventType().String() != "rename" {
		return uint16(0)
	}
	return ev.Rename.New.FileFields.Mode
}

// GetRenameFileDestinationModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationModificationTime() uint64 {
	if ev.GetEventType().String() != "rename" {
		return uint64(0)
	}
	return ev.Rename.New.FileFields.MTime
}

// GetRenameFileDestinationMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationMountId() uint32 {
	if ev.GetEventType().String() != "rename" {
		return uint32(0)
	}
	return ev.Rename.New.FileFields.PathKey.MountID
}

// GetRenameFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationName() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New)
}

// GetRenameFileDestinationNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationNameLength() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New))
}

// GetRenameFileDestinationPackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageName() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageSourceVersion() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageVersion() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPath() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPathLength() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New))
}

// GetRenameFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationRights() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationUid() uint32 {
	if ev.GetEventType().String() != "rename" {
		return uint32(0)
	}
	return ev.Rename.New.FileFields.UID
}

// GetRenameFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationUser() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileFilesystem() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old)
}

// GetRenameFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileGid() uint32 {
	if ev.GetEventType().String() != "rename" {
		return uint32(0)
	}
	return ev.Rename.Old.FileFields.GID
}

// GetRenameFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileGroup() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileHashes() []string {
	if ev.GetEventType().String() != "rename" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old)
}

// GetRenameFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileInUpperLayer() bool {
	if ev.GetEventType().String() != "rename" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileInode() uint64 {
	if ev.GetEventType().String() != "rename" {
		return uint64(0)
	}
	return ev.Rename.Old.FileFields.PathKey.Inode
}

// GetRenameFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileMode() uint16 {
	if ev.GetEventType().String() != "rename" {
		return uint16(0)
	}
	return ev.Rename.Old.FileFields.Mode
}

// GetRenameFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileModificationTime() uint64 {
	if ev.GetEventType().String() != "rename" {
		return uint64(0)
	}
	return ev.Rename.Old.FileFields.MTime
}

// GetRenameFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileMountId() uint32 {
	if ev.GetEventType().String() != "rename" {
		return uint32(0)
	}
	return ev.Rename.Old.FileFields.PathKey.MountID
}

// GetRenameFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileName() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old)
}

// GetRenameFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileNameLength() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old))
}

// GetRenameFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageName() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old)
}

// GetRenameFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old)
}

// GetRenameFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageVersion() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old)
}

// GetRenameFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePath() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old)
}

// GetRenameFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePathLength() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old))
}

// GetRenameFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileRights() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileUid() uint32 {
	if ev.GetEventType().String() != "rename" {
		return uint32(0)
	}
	return ev.Rename.Old.FileFields.UID
}

// GetRenameFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileUser() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields)
}

// GetRenameRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRenameRetval() int64 {
	if ev.GetEventType().String() != "rename" {
		return int64(0)
	}
	return ev.Rename.SyscallEvent.Retval
}

// GetRenameSyscallDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallDestinationPath() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallInt1() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallInt2() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallInt3() int {
	if ev.GetEventType().String() != "rename" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallPath() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallStr1() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallStr2() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Rename.SyscallContext)
}

// GetRenameSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetRenameSyscallStr3() string {
	if ev.GetEventType().String() != "rename" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Rename.SyscallContext)
}

// GetRmdirFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileChangeTime() uint64 {
	if ev.GetEventType().String() != "rmdir" {
		return uint64(0)
	}
	return ev.Rmdir.File.FileFields.CTime
}

// GetRmdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileFilesystem() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File)
}

// GetRmdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileGid() uint32 {
	if ev.GetEventType().String() != "rmdir" {
		return uint32(0)
	}
	return ev.Rmdir.File.FileFields.GID
}

// GetRmdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileGroup() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileHashes() []string {
	if ev.GetEventType().String() != "rmdir" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File)
}

// GetRmdirFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileInUpperLayer() bool {
	if ev.GetEventType().String() != "rmdir" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileInode() uint64 {
	if ev.GetEventType().String() != "rmdir" {
		return uint64(0)
	}
	return ev.Rmdir.File.FileFields.PathKey.Inode
}

// GetRmdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileMode() uint16 {
	if ev.GetEventType().String() != "rmdir" {
		return uint16(0)
	}
	return ev.Rmdir.File.FileFields.Mode
}

// GetRmdirFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileModificationTime() uint64 {
	if ev.GetEventType().String() != "rmdir" {
		return uint64(0)
	}
	return ev.Rmdir.File.FileFields.MTime
}

// GetRmdirFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileMountId() uint32 {
	if ev.GetEventType().String() != "rmdir" {
		return uint32(0)
	}
	return ev.Rmdir.File.FileFields.PathKey.MountID
}

// GetRmdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileName() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File)
}

// GetRmdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileNameLength() int {
	if ev.GetEventType().String() != "rmdir" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File))
}

// GetRmdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageName() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File)
}

// GetRmdirFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File)
}

// GetRmdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageVersion() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File)
}

// GetRmdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePath() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File)
}

// GetRmdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePathLength() int {
	if ev.GetEventType().String() != "rmdir" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File))
}

// GetRmdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileRights() int {
	if ev.GetEventType().String() != "rmdir" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileUid() uint32 {
	if ev.GetEventType().String() != "rmdir" {
		return uint32(0)
	}
	return ev.Rmdir.File.FileFields.UID
}

// GetRmdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileUser() string {
	if ev.GetEventType().String() != "rmdir" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirRetval() int64 {
	if ev.GetEventType().String() != "rmdir" {
		return int64(0)
	}
	return ev.Rmdir.SyscallEvent.Retval
}

// GetSelinuxBoolName returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolName() string {
	if ev.GetEventType().String() != "selinux" {
		return ""
	}
	return ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux)
}

// GetSelinuxBoolState returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolState() string {
	if ev.GetEventType().String() != "selinux" {
		return ""
	}
	return ev.SELinux.BoolChangeValue
}

// GetSelinuxBoolCommitState returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolCommitState() bool {
	if ev.GetEventType().String() != "selinux" {
		return false
	}
	return ev.SELinux.BoolCommitValue
}

// GetSelinuxEnforceStatus returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxEnforceStatus() string {
	if ev.GetEventType().String() != "selinux" {
		return ""
	}
	return ev.SELinux.EnforceStatus
}

// GetSetgidEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidEgid() uint32 {
	if ev.GetEventType().String() != "setgid" {
		return uint32(0)
	}
	return ev.SetGID.EGID
}

// GetSetgidEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidEgroup() string {
	if ev.GetEventType().String() != "setgid" {
		return ""
	}
	return ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID)
}

// GetSetgidFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidFsgid() uint32 {
	if ev.GetEventType().String() != "setgid" {
		return uint32(0)
	}
	return ev.SetGID.FSGID
}

// GetSetgidFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidFsgroup() string {
	if ev.GetEventType().String() != "setgid" {
		return ""
	}
	return ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID)
}

// GetSetgidGid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidGid() uint32 {
	if ev.GetEventType().String() != "setgid" {
		return uint32(0)
	}
	return ev.SetGID.GID
}

// GetSetgidGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidGroup() string {
	if ev.GetEventType().String() != "setgid" {
		return ""
	}
	return ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID)
}

// GetSetuidEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidEuid() uint32 {
	if ev.GetEventType().String() != "setuid" {
		return uint32(0)
	}
	return ev.SetUID.EUID
}

// GetSetuidEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidEuser() string {
	if ev.GetEventType().String() != "setuid" {
		return ""
	}
	return ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID)
}

// GetSetuidFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidFsuid() uint32 {
	if ev.GetEventType().String() != "setuid" {
		return uint32(0)
	}
	return ev.SetUID.FSUID
}

// GetSetuidFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidFsuser() string {
	if ev.GetEventType().String() != "setuid" {
		return ""
	}
	return ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID)
}

// GetSetuidUid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidUid() uint32 {
	if ev.GetEventType().String() != "setuid" {
		return uint32(0)
	}
	return ev.SetUID.UID
}

// GetSetuidUser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidUser() string {
	if ev.GetEventType().String() != "setuid" {
		return ""
	}
	return ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID)
}

// GetSetxattrFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileChangeTime() uint64 {
	if ev.GetEventType().String() != "setxattr" {
		return uint64(0)
	}
	return ev.SetXAttr.File.FileFields.CTime
}

// GetSetxattrFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileDestinationName() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr)
}

// GetSetxattrFileDestinationNamespace returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileDestinationNamespace() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr)
}

// GetSetxattrFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileFilesystem() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File)
}

// GetSetxattrFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileGid() uint32 {
	if ev.GetEventType().String() != "setxattr" {
		return uint32(0)
	}
	return ev.SetXAttr.File.FileFields.GID
}

// GetSetxattrFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileGroup() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileHashes() []string {
	if ev.GetEventType().String() != "setxattr" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File)
}

// GetSetxattrFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileInUpperLayer() bool {
	if ev.GetEventType().String() != "setxattr" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileInode() uint64 {
	if ev.GetEventType().String() != "setxattr" {
		return uint64(0)
	}
	return ev.SetXAttr.File.FileFields.PathKey.Inode
}

// GetSetxattrFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileMode() uint16 {
	if ev.GetEventType().String() != "setxattr" {
		return uint16(0)
	}
	return ev.SetXAttr.File.FileFields.Mode
}

// GetSetxattrFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileModificationTime() uint64 {
	if ev.GetEventType().String() != "setxattr" {
		return uint64(0)
	}
	return ev.SetXAttr.File.FileFields.MTime
}

// GetSetxattrFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileMountId() uint32 {
	if ev.GetEventType().String() != "setxattr" {
		return uint32(0)
	}
	return ev.SetXAttr.File.FileFields.PathKey.MountID
}

// GetSetxattrFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileName() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File)
}

// GetSetxattrFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileNameLength() int {
	if ev.GetEventType().String() != "setxattr" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File))
}

// GetSetxattrFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageName() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageVersion() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePath() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePathLength() int {
	if ev.GetEventType().String() != "setxattr" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File))
}

// GetSetxattrFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileRights() int {
	if ev.GetEventType().String() != "setxattr" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileUid() uint32 {
	if ev.GetEventType().String() != "setxattr" {
		return uint32(0)
	}
	return ev.SetXAttr.File.FileFields.UID
}

// GetSetxattrFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileUser() string {
	if ev.GetEventType().String() != "setxattr" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrRetval() int64 {
	if ev.GetEventType().String() != "setxattr" {
		return int64(0)
	}
	return ev.SetXAttr.SyscallEvent.Retval
}

// GetSignalPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalPid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	return ev.Signal.PID
}

// GetSignalRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSignalRetval() int64 {
	if ev.GetEventType().String() != "signal" {
		return int64(0)
	}
	return ev.Signal.SyscallEvent.Retval
}

// GetSignalTargetAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgs() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgsFlags() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgsOptions() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgsScrubbed() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgsTruncated() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgv() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgv0() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgvScrubbed() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsAuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsAuid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.AUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCapEffective() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.CapEffective
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCapPermitted() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.CapPermitted
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCgroupFileInode() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.CGroup.CGroupFile.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCgroupFileMountId() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.CGroup.CGroupFile.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCgroupId() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveCGroupID(ev, &element.ProcessContext.Process.CGroup)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCgroupManager() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveCGroupManager(ev, &element.ProcessContext.Process.CGroup)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCmdargv() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessCmdArgv(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsComm() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Comm
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsContainerId() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessContainerID(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCreatedAt() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEgid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EGID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEgroup() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EGroup
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEnvp() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEnvs() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEnvsTruncated() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEuid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEuser() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.EUser
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileChangeTime() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.CTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileFilesystem() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileGid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileGroup() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileHashes() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileInUpperLayer() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileInode() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileMode() []uint16 {
	if ev.GetEventType().String() != "signal" {
		return []uint16{}
	}
	if ev.Signal.Target == nil {
		return []uint16{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint16{}
	}
	var values []uint16
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.Mode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileModificationTime() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.MTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileMountId() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileName() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileNameLength() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFilePackageName() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFilePackageSourceVersion() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFilePackageVersion() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFilePath() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFilePathLength() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileRights() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileUid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.FileEvent.FileFields.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileUser() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFsgid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSGID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFsgroup() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSGroup
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFsuid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSUID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFsuser() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.FSUser
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsGid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsGroup() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.Group
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileChangeTime() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileFilesystem() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileGid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileGroup() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileInUpperLayer() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileInode() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileMode() []uint16 {
	if ev.GetEventType().String() != "signal" {
		return []uint16{}
	}
	if ev.Signal.Target == nil {
		return []uint16{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint16{}
	}
	var values []uint16
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileModificationTime() []uint64 {
	if ev.GetEventType().String() != "signal" {
		return []uint64{}
	}
	if ev.Signal.Target == nil {
		return []uint64{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint64{}
	}
	var values []uint64
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileMountId() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileName() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileNameLength() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFilePackageName() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFilePackageSourceVersion() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFilePackageVersion() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFilePath() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFilePathLength() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileRights() []int {
	if ev.GetEventType().String() != "signal" {
		return []int{}
	}
	if ev.Signal.Target == nil {
		return []int{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []int{}
	}
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileUid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileUser() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsIsExec() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.IsExec
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsIsKworker() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.IsKworker
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsIsThread() []bool {
	if ev.GetEventType().String() != "signal" {
		return []bool{}
	}
	if ev.Signal.Target == nil {
		return []bool{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []bool{}
	}
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Ancestor == nil {
		return 0
	}
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	return iterator.Len(ctx)
}

// GetSignalTargetAncestorsPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsPid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.Pid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsPpid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PPid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsTid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.PIDContext.Tid
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsTtyName() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.TTYName
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsUid() []uint32 {
	if ev.GetEventType().String() != "signal" {
		return []uint32{}
	}
	if ev.Signal.Target == nil {
		return []uint32{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []uint32{}
	}
	var values []uint32
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.UID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsUser() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.Credentials.User
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsUserSessionK8sUid() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsUserSessionK8sUsername() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Ancestor == nil {
		return []string{}
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgs() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsFlags() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsOptions() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsScrubbed() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsTruncated() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgv() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgv0() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgvScrubbed() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetAuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAuid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.AUID
}

// GetSignalTargetCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCapEffective() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	return ev.Signal.Target.Process.Credentials.CapEffective
}

// GetSignalTargetCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCapPermitted() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	return ev.Signal.Target.Process.Credentials.CapPermitted
}

// GetSignalTargetCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCgroupFileInode() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	return ev.Signal.Target.Process.CGroup.CGroupFile.Inode
}

// GetSignalTargetCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCgroupFileMountId() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.CGroup.CGroupFile.MountID
}

// GetSignalTargetCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCgroupId() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.Signal.Target.Process.CGroup)
}

// GetSignalTargetCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCgroupManager() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Signal.Target.Process.CGroup)
}

// GetSignalTargetCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCmdargv() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetComm() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Comm
}

// GetSignalTargetContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetContainerId() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCreatedAt() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEgid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.EGID
}

// GetSignalTargetEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEgroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Credentials.EGroup
}

// GetSignalTargetEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvp() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvs() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvsTruncated() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEuid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.EUID
}

// GetSignalTargetEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEuser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Credentials.EUser
}

// GetSignalTargetExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetExecTime() time.Time {
	if ev.GetEventType().String() != "signal" {
		return time.Time{}
	}
	if ev.Signal.Target == nil {
		return time.Time{}
	}
	return ev.Signal.Target.Process.ExecTime
}

// GetSignalTargetExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetExitTime() time.Time {
	if ev.GetEventType().String() != "signal" {
		return time.Time{}
	}
	if ev.Signal.Target == nil {
		return time.Time{}
	}
	return ev.Signal.Target.Process.ExitTime
}

// GetSignalTargetFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileChangeTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.CTime
}

// GetSignalTargetFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileFilesystem() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileGid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.GID
}

// GetSignalTargetFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileGroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileHashes() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileInUpperLayer() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileInode() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.PathKey.Inode
}

// GetSignalTargetFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileMode() uint16 {
	if ev.GetEventType().String() != "signal" {
		return uint16(0)
	}
	if ev.Signal.Target == nil {
		return uint16(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.Mode
}

// GetSignalTargetFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileModificationTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.MTime
}

// GetSignalTargetFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileMountId() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.PathKey.MountID
}

// GetSignalTargetFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileNameLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent))
}

// GetSignalTargetFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePath() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePathLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent))
}

// GetSignalTargetFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileRights() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileUid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.UID
}

// GetSignalTargetFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileUser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetForkTime() time.Time {
	if ev.GetEventType().String() != "signal" {
		return time.Time{}
	}
	if ev.Signal.Target == nil {
		return time.Time{}
	}
	return ev.Signal.Target.Process.ForkTime
}

// GetSignalTargetFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsgid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.FSGID
}

// GetSignalTargetFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsgroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Credentials.FSGroup
}

// GetSignalTargetFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsuid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.FSUID
}

// GetSignalTargetFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsuser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Credentials.FSUser
}

// GetSignalTargetGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetGid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.GID
}

// GetSignalTargetGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetGroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Credentials.Group
}

// GetSignalTargetInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileChangeTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetSignalTargetInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileFilesystem() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileGid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetSignalTargetInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileGroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileInUpperLayer() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileInode() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetSignalTargetInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileMode() uint16 {
	if ev.GetEventType().String() != "signal" {
		return uint16(0)
	}
	if ev.Signal.Target == nil {
		return uint16(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetSignalTargetInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileModificationTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetSignalTargetInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileMountId() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetSignalTargetInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileNameLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
}

// GetSignalTargetInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePath() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePathLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
}

// GetSignalTargetInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileRights() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileUid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetSignalTargetInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileUser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIsExec() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	return ev.Signal.Target.Process.IsExec
}

// GetSignalTargetIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIsKworker() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	return ev.Signal.Target.Process.PIDContext.IsKworker
}

// GetSignalTargetIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIsThread() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgs() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsFlags() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsOptions() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgsScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsScrubbed() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgsScrubbed(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsTruncated() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgv() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgv0() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgvScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgvScrubbed() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentAuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentAuid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.AUID
}

// GetSignalTargetParentCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCapEffective() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.Credentials.CapEffective
}

// GetSignalTargetParentCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCapPermitted() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.Credentials.CapPermitted
}

// GetSignalTargetParentCgroupFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCgroupFileInode() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.CGroup.CGroupFile.Inode
}

// GetSignalTargetParentCgroupFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCgroupFileMountId() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.CGroup.CGroupFile.MountID
}

// GetSignalTargetParentCgroupId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCgroupId() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupID(ev, &ev.Signal.Target.Parent.CGroup)
}

// GetSignalTargetParentCgroupManager returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCgroupManager() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Signal.Target.Parent.CGroup)
}

// GetSignalTargetParentCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCmdargv() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentComm() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Comm
}

// GetSignalTargetParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentContainerId() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessContainerID(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCreatedAt() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	if !ev.Signal.Target.HasParent() {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEgid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.EGID
}

// GetSignalTargetParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEgroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.EGroup
}

// GetSignalTargetParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvp() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvs() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvsTruncated() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEuid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.EUID
}

// GetSignalTargetParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEuser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.EUser
}

// GetSignalTargetParentFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileChangeTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.CTime
}

// GetSignalTargetParentFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileFilesystem() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileGid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.GID
}

// GetSignalTargetParentFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileGroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
}

// GetSignalTargetParentFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileHashes() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileInUpperLayer() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
}

// GetSignalTargetParentFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileInode() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.PathKey.Inode
}

// GetSignalTargetParentFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileMode() uint16 {
	if ev.GetEventType().String() != "signal" {
		return uint16(0)
	}
	if ev.Signal.Target == nil {
		return uint16(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint16(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint16(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint16(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.Mode
}

// GetSignalTargetParentFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileModificationTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.MTime
}

// GetSignalTargetParentFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileMountId() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.PathKey.MountID
}

// GetSignalTargetParentFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileNameLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent))
}

// GetSignalTargetParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePath() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePathLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent))
}

// GetSignalTargetParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileRights() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	if !ev.Signal.Target.HasParent() {
		return 0
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
}

// GetSignalTargetParentFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileUid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.FileEvent.FileFields.UID
}

// GetSignalTargetParentFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileUser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
}

// GetSignalTargetParentFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsgid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.FSGID
}

// GetSignalTargetParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsgroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.FSGroup
}

// GetSignalTargetParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsuid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.FSUID
}

// GetSignalTargetParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsuser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.FSUser
}

// GetSignalTargetParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentGid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.GID
}

// GetSignalTargetParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentGroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.Group
}

// GetSignalTargetParentInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileChangeTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetSignalTargetParentInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileFilesystem() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileGid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.GID
}

// GetSignalTargetParentInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileGroup() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetParentInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileHashes() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileInUpperLayer() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetParentInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileInode() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetSignalTargetParentInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileMode() uint16 {
	if ev.GetEventType().String() != "signal" {
		return uint16(0)
	}
	if ev.Signal.Target == nil {
		return uint16(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint16(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint16(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint16(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetSignalTargetParentInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileModificationTime() uint64 {
	if ev.GetEventType().String() != "signal" {
		return uint64(0)
	}
	if ev.Signal.Target == nil {
		return uint64(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint64(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetSignalTargetParentInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileMountId() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetSignalTargetParentInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileNameLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
}

// GetSignalTargetParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageVersion() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePath() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePathLength() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
}

// GetSignalTargetParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileRights() int {
	if ev.GetEventType().String() != "signal" {
		return 0
	}
	if ev.Signal.Target == nil {
		return 0
	}
	if ev.Signal.Target.Parent == nil {
		return 0
	}
	if !ev.Signal.Target.HasParent() {
		return 0
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetParentInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileUid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.UID
}

// GetSignalTargetParentInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileUser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetParentIsExec returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIsExec() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.Signal.Target.Parent.IsExec
}

// GetSignalTargetParentIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIsKworker() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.Signal.Target.Parent.PIDContext.IsKworker
}

// GetSignalTargetParentIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIsThread() bool {
	if ev.GetEventType().String() != "signal" {
		return false
	}
	if ev.Signal.Target == nil {
		return false
	}
	if ev.Signal.Target.Parent == nil {
		return false
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentPid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.PIDContext.Pid
}

// GetSignalTargetParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentPpid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.PPid
}

// GetSignalTargetParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentTid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.PIDContext.Tid
}

// GetSignalTargetParentTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentTtyName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.TTYName
}

// GetSignalTargetParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	if ev.Signal.Target.Parent == nil {
		return uint32(0)
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.UID
}

// GetSignalTargetParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.User
}

// GetSignalTargetParentUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	if ev.Signal.Target.Parent == nil {
		return []string{}
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Parent.UserSession)
}

// GetSignalTargetParentUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUserSessionK8sUid() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Parent.UserSession)
}

// GetSignalTargetParentUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUserSessionK8sUsername() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	if ev.Signal.Target.Parent == nil {
		return ""
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Parent.UserSession)
}

// GetSignalTargetPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetPid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.PIDContext.Pid
}

// GetSignalTargetPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetPpid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.PPid
}

// GetSignalTargetTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetTid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.PIDContext.Tid
}

// GetSignalTargetTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetTtyName() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.TTYName
}

// GetSignalTargetUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUid() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	if ev.Signal.Target == nil {
		return uint32(0)
	}
	return ev.Signal.Target.Process.Credentials.UID
}

// GetSignalTargetUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUser() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.Signal.Target.Process.Credentials.User
}

// GetSignalTargetUserSessionK8sGroups returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUserSessionK8sGroups() []string {
	if ev.GetEventType().String() != "signal" {
		return []string{}
	}
	if ev.Signal.Target == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Process.UserSession)
}

// GetSignalTargetUserSessionK8sUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUserSessionK8sUid() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Process.UserSession)
}

// GetSignalTargetUserSessionK8sUsername returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUserSessionK8sUsername() string {
	if ev.GetEventType().String() != "signal" {
		return ""
	}
	if ev.Signal.Target == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Process.UserSession)
}

// GetSignalType returns the value of the field, resolving if necessary
func (ev *Event) GetSignalType() uint32 {
	if ev.GetEventType().String() != "signal" {
		return uint32(0)
	}
	return ev.Signal.Type
}

// GetSpliceFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileChangeTime() uint64 {
	if ev.GetEventType().String() != "splice" {
		return uint64(0)
	}
	return ev.Splice.File.FileFields.CTime
}

// GetSpliceFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileFilesystem() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File)
}

// GetSpliceFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileGid() uint32 {
	if ev.GetEventType().String() != "splice" {
		return uint32(0)
	}
	return ev.Splice.File.FileFields.GID
}

// GetSpliceFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileGroup() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileHashes() []string {
	if ev.GetEventType().String() != "splice" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File)
}

// GetSpliceFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileInUpperLayer() bool {
	if ev.GetEventType().String() != "splice" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileInode() uint64 {
	if ev.GetEventType().String() != "splice" {
		return uint64(0)
	}
	return ev.Splice.File.FileFields.PathKey.Inode
}

// GetSpliceFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileMode() uint16 {
	if ev.GetEventType().String() != "splice" {
		return uint16(0)
	}
	return ev.Splice.File.FileFields.Mode
}

// GetSpliceFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileModificationTime() uint64 {
	if ev.GetEventType().String() != "splice" {
		return uint64(0)
	}
	return ev.Splice.File.FileFields.MTime
}

// GetSpliceFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileMountId() uint32 {
	if ev.GetEventType().String() != "splice" {
		return uint32(0)
	}
	return ev.Splice.File.FileFields.PathKey.MountID
}

// GetSpliceFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileName() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File)
}

// GetSpliceFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileNameLength() int {
	if ev.GetEventType().String() != "splice" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File))
}

// GetSpliceFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageName() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File)
}

// GetSpliceFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File)
}

// GetSpliceFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageVersion() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File)
}

// GetSpliceFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePath() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File)
}

// GetSpliceFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePathLength() int {
	if ev.GetEventType().String() != "splice" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File))
}

// GetSpliceFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileRights() int {
	if ev.GetEventType().String() != "splice" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileUid() uint32 {
	if ev.GetEventType().String() != "splice" {
		return uint32(0)
	}
	return ev.Splice.File.FileFields.UID
}

// GetSpliceFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileUser() string {
	if ev.GetEventType().String() != "splice" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields)
}

// GetSplicePipeEntryFlag returns the value of the field, resolving if necessary
func (ev *Event) GetSplicePipeEntryFlag() uint32 {
	if ev.GetEventType().String() != "splice" {
		return uint32(0)
	}
	return ev.Splice.PipeEntryFlag
}

// GetSplicePipeExitFlag returns the value of the field, resolving if necessary
func (ev *Event) GetSplicePipeExitFlag() uint32 {
	if ev.GetEventType().String() != "splice" {
		return uint32(0)
	}
	return ev.Splice.PipeExitFlag
}

// GetSpliceRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceRetval() int64 {
	if ev.GetEventType().String() != "splice" {
		return int64(0)
	}
	return ev.Splice.SyscallEvent.Retval
}

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev, &ev.BaseEvent)
}

// GetUnlinkFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileChangeTime() uint64 {
	if ev.GetEventType().String() != "unlink" {
		return uint64(0)
	}
	return ev.Unlink.File.FileFields.CTime
}

// GetUnlinkFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileFilesystem() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File)
}

// GetUnlinkFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileGid() uint32 {
	if ev.GetEventType().String() != "unlink" {
		return uint32(0)
	}
	return ev.Unlink.File.FileFields.GID
}

// GetUnlinkFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileGroup() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileHashes() []string {
	if ev.GetEventType().String() != "unlink" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File)
}

// GetUnlinkFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileInUpperLayer() bool {
	if ev.GetEventType().String() != "unlink" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileInode() uint64 {
	if ev.GetEventType().String() != "unlink" {
		return uint64(0)
	}
	return ev.Unlink.File.FileFields.PathKey.Inode
}

// GetUnlinkFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileMode() uint16 {
	if ev.GetEventType().String() != "unlink" {
		return uint16(0)
	}
	return ev.Unlink.File.FileFields.Mode
}

// GetUnlinkFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileModificationTime() uint64 {
	if ev.GetEventType().String() != "unlink" {
		return uint64(0)
	}
	return ev.Unlink.File.FileFields.MTime
}

// GetUnlinkFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileMountId() uint32 {
	if ev.GetEventType().String() != "unlink" {
		return uint32(0)
	}
	return ev.Unlink.File.FileFields.PathKey.MountID
}

// GetUnlinkFileName returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileName() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File)
}

// GetUnlinkFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileNameLength() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File))
}

// GetUnlinkFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageName() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File)
}

// GetUnlinkFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File)
}

// GetUnlinkFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageVersion() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File)
}

// GetUnlinkFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePath() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File)
}

// GetUnlinkFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePathLength() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File))
}

// GetUnlinkFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileRights() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileUid() uint32 {
	if ev.GetEventType().String() != "unlink" {
		return uint32(0)
	}
	return ev.Unlink.File.FileFields.UID
}

// GetUnlinkFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileUser() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFlags returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFlags() uint32 {
	if ev.GetEventType().String() != "unlink" {
		return uint32(0)
	}
	return ev.Unlink.Flags
}

// GetUnlinkRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkRetval() int64 {
	if ev.GetEventType().String() != "unlink" {
		return int64(0)
	}
	return ev.Unlink.SyscallEvent.Retval
}

// GetUnlinkSyscallDirfd returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallDirfd() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallFlags returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallFlags() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallInt1() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallInt2() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallInt3() int {
	if ev.GetEventType().String() != "unlink" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallPath() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallStr1() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallStr2() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Unlink.SyscallContext)
}

// GetUnlinkSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkSyscallStr3() string {
	if ev.GetEventType().String() != "unlink" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Unlink.SyscallContext)
}

// GetUnloadModuleName returns the value of the field, resolving if necessary
func (ev *Event) GetUnloadModuleName() string {
	if ev.GetEventType().String() != "unload_module" {
		return ""
	}
	return ev.UnloadModule.Name
}

// GetUnloadModuleRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUnloadModuleRetval() int64 {
	if ev.GetEventType().String() != "unload_module" {
		return int64(0)
	}
	return ev.UnloadModule.SyscallEvent.Retval
}

// GetUtimesFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileChangeTime() uint64 {
	if ev.GetEventType().String() != "utimes" {
		return uint64(0)
	}
	return ev.Utimes.File.FileFields.CTime
}

// GetUtimesFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileFilesystem() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File)
}

// GetUtimesFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileGid() uint32 {
	if ev.GetEventType().String() != "utimes" {
		return uint32(0)
	}
	return ev.Utimes.File.FileFields.GID
}

// GetUtimesFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileGroup() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileHashes() []string {
	if ev.GetEventType().String() != "utimes" {
		return []string{}
	}
	return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File)
}

// GetUtimesFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileInUpperLayer() bool {
	if ev.GetEventType().String() != "utimes" {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileInode() uint64 {
	if ev.GetEventType().String() != "utimes" {
		return uint64(0)
	}
	return ev.Utimes.File.FileFields.PathKey.Inode
}

// GetUtimesFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileMode() uint16 {
	if ev.GetEventType().String() != "utimes" {
		return uint16(0)
	}
	return ev.Utimes.File.FileFields.Mode
}

// GetUtimesFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileModificationTime() uint64 {
	if ev.GetEventType().String() != "utimes" {
		return uint64(0)
	}
	return ev.Utimes.File.FileFields.MTime
}

// GetUtimesFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileMountId() uint32 {
	if ev.GetEventType().String() != "utimes" {
		return uint32(0)
	}
	return ev.Utimes.File.FileFields.PathKey.MountID
}

// GetUtimesFileName returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileName() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File)
}

// GetUtimesFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileNameLength() int {
	if ev.GetEventType().String() != "utimes" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File))
}

// GetUtimesFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageName() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File)
}

// GetUtimesFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageSourceVersion() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File)
}

// GetUtimesFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageVersion() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File)
}

// GetUtimesFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePath() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File)
}

// GetUtimesFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePathLength() int {
	if ev.GetEventType().String() != "utimes" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File))
}

// GetUtimesFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileRights() int {
	if ev.GetEventType().String() != "utimes" {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileUid() uint32 {
	if ev.GetEventType().String() != "utimes" {
		return uint32(0)
	}
	return ev.Utimes.File.FileFields.UID
}

// GetUtimesFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileUser() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesRetval() int64 {
	if ev.GetEventType().String() != "utimes" {
		return int64(0)
	}
	return ev.Utimes.SyscallEvent.Retval
}

// GetUtimesSyscallInt1 returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallInt1() int {
	if ev.GetEventType().String() != "utimes" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Utimes.SyscallContext)
}

// GetUtimesSyscallInt2 returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallInt2() int {
	if ev.GetEventType().String() != "utimes" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Utimes.SyscallContext)
}

// GetUtimesSyscallInt3 returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallInt3() int {
	if ev.GetEventType().String() != "utimes" {
		return 0
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Utimes.SyscallContext)
}

// GetUtimesSyscallPath returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallPath() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Utimes.SyscallContext)
}

// GetUtimesSyscallStr1 returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallStr1() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Utimes.SyscallContext)
}

// GetUtimesSyscallStr2 returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallStr2() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Utimes.SyscallContext)
}

// GetUtimesSyscallStr3 returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesSyscallStr3() string {
	if ev.GetEventType().String() != "utimes" {
		return ""
	}
	return ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Utimes.SyscallContext)
}
