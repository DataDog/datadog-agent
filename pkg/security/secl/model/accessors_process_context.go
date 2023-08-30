// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix
// +build unix

package model

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// GetBindAddrFamily returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrFamily() int {
	return int(ev.Bind.AddrFamily)
}

// GetBindAddrIp returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrIp() net.IPNet {
	if ev.Bind.Addr.IPNet != nil {
		return ev.Bind.Addr.IPNet
	} else {
		return nil
	}
}

// GetBindAddrPort returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrPort() int {
	return int(ev.Bind.Addr.Port)
}

// GetBindRetval returns the value of the field, resolving if necessary
func (ev *Event) GetBindRetval() int {
	return int(ev.Bind.SyscallEvent.Retval)
}

// GetBpfCmd returns the value of the field, resolving if necessary
func (ev *Event) GetBpfCmd() int {
	return int(ev.BPF.Cmd)
}

// GetBpfMapName returns the value of the field, resolving if necessary
func (ev *Event) GetBpfMapName() string {
	if ev.BPF.Map.Name != nil {
		return ev.BPF.Map.Name
	} else {
		return ""
	}
}

// GetBpfMapType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfMapType() int {
	return int(ev.BPF.Map.Type)
}

// GetBpfProgAttach_type returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgAttach_type() int {
	return int(ev.BPF.Program.AttachType)
}

// GetBpfProgHelpers returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgHelpers() []int {
	result := make([]int, len(ev.BPF.Program.Helpers))
	for i, v := range ev.BPF.Program.Helpers {
		result[i] = int(v)
	}
	return result
}

// GetBpfProgName returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgName() string {
	if ev.BPF.Program.Name != nil {
		return ev.BPF.Program.Name
	} else {
		return ""
	}
}

// GetBpfProgTag returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgTag() string {
	if ev.BPF.Program.Tag != nil {
		return ev.BPF.Program.Tag
	} else {
		return ""
	}
}

// GetBpfProgType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgType() int {
	return int(ev.BPF.Program.Type)
}

// GetBpfRetval returns the value of the field, resolving if necessary
func (ev *Event) GetBpfRetval() int {
	return int(ev.BPF.SyscallEvent.Retval)
}

// GetCapsetCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetCapsetCap_effective() int {
	return int(ev.Capset.CapEffective)
}

// GetCapsetCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetCapsetCap_permitted() int {
	return int(ev.Capset.CapPermitted)
}

// GetChmodFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileChange_time() int {
	return int(ev.Chmod.File.FileFields.CTime)
}

// GetChmodFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileDestinationMode() int {
	return int(ev.Chmod.Mode)
}

// GetChmodFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileDestinationRights() int {
	return int(ev.Chmod.Mode)
}

// GetChmodFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileGid() int {
	return int(ev.Chmod.File.FileFields.GID)
}

// GetChmodFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields)
	} else {
		return ""
	}
}

// GetChmodFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileInode() int {
	return int(ev.Chmod.File.FileFields.PathKey.Inode)
}

// GetChmodFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileMode() int {
	return int(ev.Chmod.File.FileFields.Mode)
}

// GetChmodFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileModification_time() int {
	return int(ev.Chmod.File.FileFields.MTime)
}

// GetChmodFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileMount_id() int {
	return int(ev.Chmod.File.FileFields.PathKey.MountID)
}

// GetChmodFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File)
}

// GetChmodFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File)
	} else {
		return ""
	}
}

// GetChmodFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File)
}

// GetChmodFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Chmod.File.FileFields))
}

// GetChmodFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileUid() int {
	return int(ev.Chmod.File.FileFields.UID)
}

// GetChmodFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields)
	} else {
		return ""
	}
}

// GetChmodRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChmodRetval() int {
	return int(ev.Chmod.SyscallEvent.Retval)
}

// GetChownFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileChange_time() int {
	return int(ev.Chown.File.FileFields.CTime)
}

// GetChownFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationGid() int {
	return int(ev.Chown.GID)
}

// GetChownFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationGroup() string {
	if ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown) != nil {
		return ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown)
	} else {
		return ""
	}
}

// GetChownFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationUid() int {
	return int(ev.Chown.UID)
}

// GetChownFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationUser() string {
	if ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown) != nil {
		return ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown)
	} else {
		return ""
	}
}

// GetChownFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileGid() int {
	return int(ev.Chown.File.FileFields.GID)
}

// GetChownFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields)
	} else {
		return ""
	}
}

// GetChownFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chown.File.FileFields)
}

// GetChownFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileInode() int {
	return int(ev.Chown.File.FileFields.PathKey.Inode)
}

// GetChownFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileMode() int {
	return int(ev.Chown.File.FileFields.Mode)
}

// GetChownFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileModification_time() int {
	return int(ev.Chown.File.FileFields.MTime)
}

// GetChownFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileMount_id() int {
	return int(ev.Chown.File.FileFields.PathKey.MountID)
}

// GetChownFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File)
}

// GetChownFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File)
	} else {
		return ""
	}
}

// GetChownFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File)
}

// GetChownFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Chown.File.FileFields))
}

// GetChownFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileUid() int {
	return int(ev.Chown.File.FileFields.UID)
}

// GetChownFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields)
	} else {
		return ""
	}
}

// GetChownRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChownRetval() int {
	return int(ev.Chown.SyscallEvent.Retval)
}

// GetContainerCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetContainerCreated_at() int {
	return int(ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext))
}

// GetContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetContainerId() string {
	if ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext) != nil {
		return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
	} else {
		return ""
	}
}

// GetContainerTags returns the value of the field, resolving if necessary
func (ev *Event) GetContainerTags() []string {
	if ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext) != nil {
		return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
	} else {
		return ""
	}
}

// GetDnsId returns the value of the field, resolving if necessary
func (ev *Event) GetDnsId() int {
	return int(ev.DNS.ID)
}

// GetDnsQuestionClass returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionClass() int {
	return int(ev.DNS.Class)
}

// GetDnsQuestionCount returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionCount() int {
	return int(ev.DNS.Count)
}

// GetDnsQuestionLength returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionLength() int {
	return int(ev.DNS.Size)
}

// GetDnsQuestionName returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionName() string {
	if ev.DNS.Name != nil {
		return ev.DNS.Name
	} else {
		return ""
	}
}

// GetDnsQuestionNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionNameLength() int {
	return len(ev.DNS.Name)
}

// GetDnsQuestionType returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionType() int {
	return int(ev.DNS.Type)
}

// GetEventAsync returns the value of the field, resolving if necessary
func (ev *Event) GetEventAsync() bool {
	return ev.FieldHandlers.ResolveAsync(ev)
}

// GetEventTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetEventTimestamp() int {
	return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent))
}

// GetExecArgs returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process)
}

// GetExecArgv returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetExecCap_effective() int {
	return int(ev.Exec.Process.Credentials.CapEffective)
}

// GetExecCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetExecCap_permitted() int {
	return int(ev.Exec.Process.Credentials.CapPermitted)
}

// GetExecComm returns the value of the field, resolving if necessary
func (ev *Event) GetExecComm() string {
	if ev.Exec.Process.Comm != nil {
		return ev.Exec.Process.Comm
	} else {
		return ""
	}
}

// GetExecContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExecContainerId() string {
	if ev.Exec.Process.ContainerID != nil {
		return ev.Exec.Process.ContainerID
	} else {
		return ""
	}
}

// GetExecCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetExecCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process))
}

// GetExecEgid returns the value of the field, resolving if necessary
func (ev *Event) GetExecEgid() int {
	return int(ev.Exec.Process.Credentials.EGID)
}

// GetExecEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecEgroup() string {
	if ev.Exec.Process.Credentials.EGroup != nil {
		return ev.Exec.Process.Credentials.EGroup
	} else {
		return ""
	}
}

// GetExecEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
	} else {
		return ""
	}
}

// GetExecEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process)
}

// GetExecEuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecEuid() int {
	return int(ev.Exec.Process.Credentials.EUID)
}

// GetExecEuser returns the value of the field, resolving if necessary
func (ev *Event) GetExecEuser() string {
	if ev.Exec.Process.Credentials.EUser != nil {
		return ev.Exec.Process.Credentials.EUser
	} else {
		return ""
	}
}

// GetExecFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileChange_time() int {
	return int(ev.Exec.Process.FileEvent.FileFields.CTime)
}

// GetExecFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileGid() int {
	return int(ev.Exec.Process.FileEvent.FileFields.GID)
}

// GetExecFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExecFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileInode() int {
	return int(ev.Exec.Process.FileEvent.FileFields.PathKey.Inode)
}

// GetExecFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileMode() int {
	return int(ev.Exec.Process.FileEvent.FileFields.Mode)
}

// GetExecFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileModification_time() int {
	return int(ev.Exec.Process.FileEvent.FileFields.MTime)
}

// GetExecFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileMount_id() int {
	return int(ev.Exec.Process.FileEvent.FileFields.PathKey.MountID)
}

// GetExecFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExecFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields))
}

// GetExecFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileUid() int {
	return int(ev.Exec.Process.FileEvent.FileFields.UID)
}

// GetExecFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExecFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsgid() int {
	return int(ev.Exec.Process.Credentials.FSGID)
}

// GetExecFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsgroup() string {
	if ev.Exec.Process.Credentials.FSGroup != nil {
		return ev.Exec.Process.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetExecFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsuid() int {
	return int(ev.Exec.Process.Credentials.FSUID)
}

// GetExecFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsuser() string {
	if ev.Exec.Process.Credentials.FSUser != nil {
		return ev.Exec.Process.Credentials.FSUser
	} else {
		return ""
	}
}

// GetExecGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecGid() int {
	return int(ev.Exec.Process.Credentials.GID)
}

// GetExecGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecGroup() string {
	if ev.Exec.Process.Credentials.Group != nil {
		return ev.Exec.Process.Credentials.Group
	} else {
		return ""
	}
}

// GetExecInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileChange_time() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetExecInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileGid() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetExecInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExecInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileInode() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetExecInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileMode() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetExecInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileModification_time() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetExecInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileMount_id() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetExecInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExecInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
}

// GetExecInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileUid() int {
	return int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetExecInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExecIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetExecIs_kworker() bool {
	return ev.Exec.Process.PIDContext.IsKworker
}

// GetExecIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetExecIs_thread() bool {
	return ev.Exec.Process.IsThread
}

// GetExecPid returns the value of the field, resolving if necessary
func (ev *Event) GetExecPid() int {
	return int(ev.Exec.Process.PIDContext.Pid)
}

// GetExecPpid returns the value of the field, resolving if necessary
func (ev *Event) GetExecPpid() int {
	return int(ev.Exec.Process.PPid)
}

// GetExecTid returns the value of the field, resolving if necessary
func (ev *Event) GetExecTid() int {
	return int(ev.Exec.Process.PIDContext.Tid)
}

// GetExecTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetExecTty_name() string {
	if ev.Exec.Process.TTYName != nil {
		return ev.Exec.Process.TTYName
	} else {
		return ""
	}
}

// GetExecUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecUid() int {
	return int(ev.Exec.Process.Credentials.UID)
}

// GetExecUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecUser() string {
	if ev.Exec.Process.Credentials.User != nil {
		return ev.Exec.Process.Credentials.User
	} else {
		return ""
	}
}

// GetExitArgs returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
}

// GetExitArgv returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetExitCap_effective() int {
	return int(ev.Exit.Process.Credentials.CapEffective)
}

// GetExitCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetExitCap_permitted() int {
	return int(ev.Exit.Process.Credentials.CapPermitted)
}

// GetExitCause returns the value of the field, resolving if necessary
func (ev *Event) GetExitCause() int {
	return int(ev.Exit.Cause)
}

// GetExitCode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCode() int {
	return int(ev.Exit.Code)
}

// GetExitComm returns the value of the field, resolving if necessary
func (ev *Event) GetExitComm() string {
	if ev.Exit.Process.Comm != nil {
		return ev.Exit.Process.Comm
	} else {
		return ""
	}
}

// GetExitContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExitContainerId() string {
	if ev.Exit.Process.ContainerID != nil {
		return ev.Exit.Process.ContainerID
	} else {
		return ""
	}
}

// GetExitCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetExitCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process))
}

// GetExitEgid returns the value of the field, resolving if necessary
func (ev *Event) GetExitEgid() int {
	return int(ev.Exit.Process.Credentials.EGID)
}

// GetExitEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitEgroup() string {
	if ev.Exit.Process.Credentials.EGroup != nil {
		return ev.Exit.Process.Credentials.EGroup
	} else {
		return ""
	}
}

// GetExitEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
	} else {
		return ""
	}
}

// GetExitEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
}

// GetExitEuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitEuid() int {
	return int(ev.Exit.Process.Credentials.EUID)
}

// GetExitEuser returns the value of the field, resolving if necessary
func (ev *Event) GetExitEuser() string {
	if ev.Exit.Process.Credentials.EUser != nil {
		return ev.Exit.Process.Credentials.EUser
	} else {
		return ""
	}
}

// GetExitFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileChange_time() int {
	return int(ev.Exit.Process.FileEvent.FileFields.CTime)
}

// GetExitFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileGid() int {
	return int(ev.Exit.Process.FileEvent.FileFields.GID)
}

// GetExitFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExitFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileInode() int {
	return int(ev.Exit.Process.FileEvent.FileFields.PathKey.Inode)
}

// GetExitFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileMode() int {
	return int(ev.Exit.Process.FileEvent.FileFields.Mode)
}

// GetExitFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileModification_time() int {
	return int(ev.Exit.Process.FileEvent.FileFields.MTime)
}

// GetExitFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileMount_id() int {
	return int(ev.Exit.Process.FileEvent.FileFields.PathKey.MountID)
}

// GetExitFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
	} else {
		return ""
	}
}

// GetExitFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields))
}

// GetExitFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileUid() int {
	return int(ev.Exit.Process.FileEvent.FileFields.UID)
}

// GetExitFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExitFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsgid() int {
	return int(ev.Exit.Process.Credentials.FSGID)
}

// GetExitFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsgroup() string {
	if ev.Exit.Process.Credentials.FSGroup != nil {
		return ev.Exit.Process.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetExitFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsuid() int {
	return int(ev.Exit.Process.Credentials.FSUID)
}

// GetExitFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsuser() string {
	if ev.Exit.Process.Credentials.FSUser != nil {
		return ev.Exit.Process.Credentials.FSUser
	} else {
		return ""
	}
}

// GetExitGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitGid() int {
	return int(ev.Exit.Process.Credentials.GID)
}

// GetExitGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitGroup() string {
	if ev.Exit.Process.Credentials.Group != nil {
		return ev.Exit.Process.Credentials.Group
	} else {
		return ""
	}
}

// GetExitInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileChange_time() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetExitInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileGid() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetExitInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExitInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileInode() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetExitInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileMode() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetExitInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileModification_time() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetExitInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileMount_id() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetExitInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetExitInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
}

// GetExitInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileUid() int {
	return int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetExitInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetExitIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetExitIs_kworker() bool {
	return ev.Exit.Process.PIDContext.IsKworker
}

// GetExitIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetExitIs_thread() bool {
	return ev.Exit.Process.IsThread
}

// GetExitPid returns the value of the field, resolving if necessary
func (ev *Event) GetExitPid() int {
	return int(ev.Exit.Process.PIDContext.Pid)
}

// GetExitPpid returns the value of the field, resolving if necessary
func (ev *Event) GetExitPpid() int {
	return int(ev.Exit.Process.PPid)
}

// GetExitTid returns the value of the field, resolving if necessary
func (ev *Event) GetExitTid() int {
	return int(ev.Exit.Process.PIDContext.Tid)
}

// GetExitTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetExitTty_name() string {
	if ev.Exit.Process.TTYName != nil {
		return ev.Exit.Process.TTYName
	} else {
		return ""
	}
}

// GetExitUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitUid() int {
	return int(ev.Exit.Process.Credentials.UID)
}

// GetExitUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitUser() string {
	if ev.Exit.Process.Credentials.User != nil {
		return ev.Exit.Process.Credentials.User
	} else {
		return ""
	}
}

// GetLinkFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileChange_time() int {
	return int(ev.Link.Source.FileFields.CTime)
}

// GetLinkFileDestinationChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationChange_time() int {
	return int(ev.Link.Target.FileFields.CTime)
}

// GetLinkFileDestinationFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationGid() int {
	return int(ev.Link.Target.FileFields.GID)
}

// GetLinkFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields)
	} else {
		return ""
	}
}

// GetLinkFileDestinationHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationInode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationInode() int {
	return int(ev.Link.Target.FileFields.PathKey.Inode)
}

// GetLinkFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationMode() int {
	return int(ev.Link.Target.FileFields.Mode)
}

// GetLinkFileDestinationModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationModification_time() int {
	return int(ev.Link.Target.FileFields.MTime)
}

// GetLinkFileDestinationMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationMount_id() int {
	return int(ev.Link.Target.FileFields.PathKey.MountID)
}

// GetLinkFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationPackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationPackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target)
	} else {
		return ""
	}
}

// GetLinkFileDestinationPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target)
}

// GetLinkFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Link.Target.FileFields))
}

// GetLinkFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationUid() int {
	return int(ev.Link.Target.FileFields.UID)
}

// GetLinkFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields)
	} else {
		return ""
	}
}

// GetLinkFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileGid() int {
	return int(ev.Link.Source.FileFields.GID)
}

// GetLinkFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields)
	} else {
		return ""
	}
}

// GetLinkFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileInode() int {
	return int(ev.Link.Source.FileFields.PathKey.Inode)
}

// GetLinkFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileMode() int {
	return int(ev.Link.Source.FileFields.Mode)
}

// GetLinkFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileModification_time() int {
	return int(ev.Link.Source.FileFields.MTime)
}

// GetLinkFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileMount_id() int {
	return int(ev.Link.Source.FileFields.PathKey.MountID)
}

// GetLinkFileName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source)
}

// GetLinkFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source)
	} else {
		return ""
	}
}

// GetLinkFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source)
}

// GetLinkFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Link.Source.FileFields))
}

// GetLinkFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileUid() int {
	return int(ev.Link.Source.FileFields.UID)
}

// GetLinkFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields)
	} else {
		return ""
	}
}

// GetLinkRetval returns the value of the field, resolving if necessary
func (ev *Event) GetLinkRetval() int {
	return int(ev.Link.SyscallEvent.Retval)
}

// GetLoad_moduleArgs returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleArgs() string {
	if ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule) != nil {
		return ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule)
	} else {
		return ""
	}
}

// GetLoad_moduleArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleArgs_truncated() bool {
	return ev.LoadModule.ArgsTruncated
}

// GetLoad_moduleArgv returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleArgv() []string {
	if ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule) != nil {
		return ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule)
	} else {
		return ""
	}
}

// GetLoad_moduleFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileChange_time() int {
	return int(ev.LoadModule.File.FileFields.CTime)
}

// GetLoad_moduleFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileGid() int {
	return int(ev.LoadModule.File.FileFields.GID)
}

// GetLoad_moduleFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields)
	} else {
		return ""
	}
}

// GetLoad_moduleFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.LoadModule.File.FileFields)
}

// GetLoad_moduleFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileInode() int {
	return int(ev.LoadModule.File.FileFields.PathKey.Inode)
}

// GetLoad_moduleFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileMode() int {
	return int(ev.LoadModule.File.FileFields.Mode)
}

// GetLoad_moduleFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileModification_time() int {
	return int(ev.LoadModule.File.FileFields.MTime)
}

// GetLoad_moduleFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileMount_id() int {
	return int(ev.LoadModule.File.FileFields.PathKey.MountID)
}

// GetLoad_moduleFileName returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File)
}

// GetLoad_moduleFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File)
	} else {
		return ""
	}
}

// GetLoad_moduleFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File)
}

// GetLoad_moduleFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.LoadModule.File.FileFields))
}

// GetLoad_moduleFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileUid() int {
	return int(ev.LoadModule.File.FileFields.UID)
}

// GetLoad_moduleFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields)
	} else {
		return ""
	}
}

// GetLoad_moduleLoaded_from_memory returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleLoaded_from_memory() bool {
	return ev.LoadModule.LoadedFromMemory
}

// GetLoad_moduleName returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleName() string {
	if ev.LoadModule.Name != nil {
		return ev.LoadModule.Name
	} else {
		return ""
	}
}

// GetLoad_moduleRetval returns the value of the field, resolving if necessary
func (ev *Event) GetLoad_moduleRetval() int {
	return int(ev.LoadModule.SyscallEvent.Retval)
}

// GetMkdirFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileChange_time() int {
	return int(ev.Mkdir.File.FileFields.CTime)
}

// GetMkdirFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileDestinationMode() int {
	return int(ev.Mkdir.Mode)
}

// GetMkdirFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileDestinationRights() int {
	return int(ev.Mkdir.Mode)
}

// GetMkdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileGid() int {
	return int(ev.Mkdir.File.FileFields.GID)
}

// GetMkdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields)
	} else {
		return ""
	}
}

// GetMkdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileInode() int {
	return int(ev.Mkdir.File.FileFields.PathKey.Inode)
}

// GetMkdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileMode() int {
	return int(ev.Mkdir.File.FileFields.Mode)
}

// GetMkdirFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileModification_time() int {
	return int(ev.Mkdir.File.FileFields.MTime)
}

// GetMkdirFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileMount_id() int {
	return int(ev.Mkdir.File.FileFields.PathKey.MountID)
}

// GetMkdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File)
}

// GetMkdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File)
	} else {
		return ""
	}
}

// GetMkdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File)
}

// GetMkdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Mkdir.File.FileFields))
}

// GetMkdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileUid() int {
	return int(ev.Mkdir.File.FileFields.UID)
}

// GetMkdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields)
	} else {
		return ""
	}
}

// GetMkdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirRetval() int {
	return int(ev.Mkdir.SyscallEvent.Retval)
}

// GetMmapFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileChange_time() int {
	return int(ev.MMap.File.FileFields.CTime)
}

// GetMmapFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileGid() int {
	return int(ev.MMap.File.FileFields.GID)
}

// GetMmapFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields)
	} else {
		return ""
	}
}

// GetMmapFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileInode() int {
	return int(ev.MMap.File.FileFields.PathKey.Inode)
}

// GetMmapFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileMode() int {
	return int(ev.MMap.File.FileFields.Mode)
}

// GetMmapFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileModification_time() int {
	return int(ev.MMap.File.FileFields.MTime)
}

// GetMmapFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileMount_id() int {
	return int(ev.MMap.File.FileFields.PathKey.MountID)
}

// GetMmapFileName returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File)
}

// GetMmapFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File)
	} else {
		return ""
	}
}

// GetMmapFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File)
}

// GetMmapFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.MMap.File.FileFields))
}

// GetMmapFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileUid() int {
	return int(ev.MMap.File.FileFields.UID)
}

// GetMmapFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields)
	} else {
		return ""
	}
}

// GetMmapFlags returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFlags() int {
	return ev.MMap.Flags
}

// GetMmapProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMmapProtection() int {
	return ev.MMap.Protection
}

// GetMmapRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMmapRetval() int {
	return int(ev.MMap.SyscallEvent.Retval)
}

// GetMountFs_type returns the value of the field, resolving if necessary
func (ev *Event) GetMountFs_type() string {
	if ev.Mount.Mount.FSType != nil {
		return ev.Mount.Mount.FSType
	} else {
		return ""
	}
}

// GetMountMountpointPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountMountpointPath() string {
	if ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount) != nil {
		return ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount)
	} else {
		return ""
	}
}

// GetMountRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMountRetval() int {
	return int(ev.Mount.SyscallEvent.Retval)
}

// GetMountSourcePath returns the value of the field, resolving if necessary
func (ev *Event) GetMountSourcePath() string {
	if ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount) != nil {
		return ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount)
	} else {
		return ""
	}
}

// GetMprotectReq_protection returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectReq_protection() int {
	return ev.MProtect.ReqProtection
}

// GetMprotectRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectRetval() int {
	return int(ev.MProtect.SyscallEvent.Retval)
}

// GetMprotectVm_protection returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectVm_protection() int {
	return ev.MProtect.VMProtection
}

// GetNetworkDestinationIp returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDestinationIp() net.IPNet {
	if ev.BaseEvent.NetworkContext.Destination.IPNet != nil {
		return ev.BaseEvent.NetworkContext.Destination.IPNet
	} else {
		return nil
	}
}

// GetNetworkDestinationPort returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDestinationPort() int {
	return int(ev.BaseEvent.NetworkContext.Destination.Port)
}

// GetNetworkDeviceIfindex returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDeviceIfindex() int {
	return int(ev.BaseEvent.NetworkContext.Device.IfIndex)
}

// GetNetworkDeviceIfname returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDeviceIfname() string {
	if ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.BaseEvent.NetworkContext.Device) != nil {
		return ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.BaseEvent.NetworkContext.Device)
	} else {
		return ""
	}
}

// GetNetworkL3_protocol returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkL3_protocol() int {
	return int(ev.BaseEvent.NetworkContext.L3Protocol)
}

// GetNetworkL4_protocol returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkL4_protocol() int {
	return int(ev.BaseEvent.NetworkContext.L4Protocol)
}

// GetNetworkSize returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSize() int {
	return int(ev.BaseEvent.NetworkContext.Size)
}

// GetNetworkSourceIp returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSourceIp() net.IPNet {
	if ev.BaseEvent.NetworkContext.Source.IPNet != nil {
		return ev.BaseEvent.NetworkContext.Source.IPNet
	} else {
		return nil
	}
}

// GetNetworkSourcePort returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSourcePort() int {
	return int(ev.BaseEvent.NetworkContext.Source.Port)
}

// GetOpenFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileChange_time() int {
	return int(ev.Open.File.FileFields.CTime)
}

// GetOpenFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileDestinationMode() int {
	return int(ev.Open.Mode)
}

// GetOpenFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileGid() int {
	return int(ev.Open.File.FileFields.GID)
}

// GetOpenFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields)
	} else {
		return ""
	}
}

// GetOpenFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Open.File.FileFields)
}

// GetOpenFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileInode() int {
	return int(ev.Open.File.FileFields.PathKey.Inode)
}

// GetOpenFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileMode() int {
	return int(ev.Open.File.FileFields.Mode)
}

// GetOpenFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileModification_time() int {
	return int(ev.Open.File.FileFields.MTime)
}

// GetOpenFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileMount_id() int {
	return int(ev.Open.File.FileFields.PathKey.MountID)
}

// GetOpenFileName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File)
}

// GetOpenFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File)
	} else {
		return ""
	}
}

// GetOpenFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File)
}

// GetOpenFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Open.File.FileFields))
}

// GetOpenFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileUid() int {
	return int(ev.Open.File.FileFields.UID)
}

// GetOpenFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields)
	} else {
		return ""
	}
}

// GetOpenFlags returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFlags() int {
	return int(ev.Open.Flags)
}

// GetOpenRetval returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRetval() int {
	return int(ev.Open.SyscallEvent.Retval)
}

// GetProcessAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgs() []string {
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

// GetProcessAncestorsArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgs_flags() []string {
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

// GetProcessAncestorsArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgs_options() []string {
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

// GetProcessAncestorsArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgs_truncated() []bool {
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

// GetProcessAncestorsCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCap_effective() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.CapEffective)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCap_permitted() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.CapPermitted)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsComm() []string {
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
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.ContainerID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCreated_at() []int {
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
func (ev *Event) GetProcessAncestorsEgid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.EGID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEgroup() []string {
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

// GetProcessAncestorsEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvs_truncated() []bool {
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
func (ev *Event) GetProcessAncestorsEuid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.EUID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEuser() []string {
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

// GetProcessAncestorsFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileChange_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileFilesystem() []string {
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
func (ev *Event) GetProcessAncestorsFileGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileGroup() []string {
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

// GetProcessAncestorsFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileIn_upper_layer() []bool {
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
func (ev *Event) GetProcessAncestorsFileInode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileMode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileModification_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileMount_id() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileName() []string {
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

// GetProcessAncestorsFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFilePackageSource_version() []string {
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
func (ev *Event) GetProcessAncestorsFileUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFileUser() []string {
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
func (ev *Event) GetProcessAncestorsFsgid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.FSGID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFsgroup() []string {
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
func (ev *Event) GetProcessAncestorsFsuid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.FSUID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsFsuser() []string {
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
func (ev *Event) GetProcessAncestorsGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsGroup() []string {
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

// GetProcessAncestorsInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileChange_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileFilesystem() []string {
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
func (ev *Event) GetProcessAncestorsInterpreterFileGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileGroup() []string {
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

// GetProcessAncestorsInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileIn_upper_layer() []bool {
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
func (ev *Event) GetProcessAncestorsInterpreterFileInode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileMode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileModification_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileMount_id() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileName() []string {
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

// GetProcessAncestorsInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFilePackageSource_version() []string {
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
func (ev *Event) GetProcessAncestorsInterpreterFileUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsInterpreterFileUser() []string {
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

// GetProcessAncestorsIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsIs_kworker() []bool {
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

// GetProcessAncestorsIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsIs_thread() []bool {
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.IsThread
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsPid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PIDContext.Pid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsPpid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PPid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsTid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PIDContext.Tid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsTty_name() []string {
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
func (ev *Event) GetProcessAncestorsUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUser() []string {
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

// GetProcessArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCap_effective() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.CapEffective)
}

// GetProcessCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCap_permitted() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.CapPermitted)
}

// GetProcessComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessComm() string {
	if ev.BaseEvent.ProcessContext.Process.Comm != nil {
		return ev.BaseEvent.ProcessContext.Process.Comm
	} else {
		return ""
	}
}

// GetProcessContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessContainerId() string {
	if ev.BaseEvent.ProcessContext.Process.ContainerID != nil {
		return ev.BaseEvent.ProcessContext.Process.ContainerID
	} else {
		return ""
	}
}

// GetProcessCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process))
}

// GetProcessEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEgid() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.EGID)
}

// GetProcessEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEgroup() string {
	if ev.BaseEvent.ProcessContext.Process.Credentials.EGroup != nil {
		return ev.BaseEvent.ProcessContext.Process.Credentials.EGroup
	} else {
		return ""
	}
}

// GetProcessEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
	} else {
		return ""
	}
}

// GetProcessEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEuid() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.EUID)
}

// GetProcessEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEuser() string {
	if ev.BaseEvent.ProcessContext.Process.Credentials.EUser != nil {
		return ev.BaseEvent.ProcessContext.Process.Credentials.EUser
	} else {
		return ""
	}
}

// GetProcessFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileChange_time() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.CTime)
}

// GetProcessFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileGid() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.GID)
}

// GetProcessFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileInode() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode)
}

// GetProcessFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileMode() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.Mode)
}

// GetProcessFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileModification_time() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.MTime)
}

// GetProcessFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileMount_id() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID)
}

// GetProcessFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	} else {
		return ""
	}
}

// GetProcessFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
}

// GetProcessFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileUid() int {
	return int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.UID)
}

// GetProcessFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsgid() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.FSGID)
}

// GetProcessFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsgroup() string {
	if ev.BaseEvent.ProcessContext.Process.Credentials.FSGroup != nil {
		return ev.BaseEvent.ProcessContext.Process.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetProcessFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsuid() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.FSUID)
}

// GetProcessFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsuser() string {
	if ev.BaseEvent.ProcessContext.Process.Credentials.FSUser != nil {
		return ev.BaseEvent.ProcessContext.Process.Credentials.FSUser
	} else {
		return ""
	}
}

// GetProcessGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessGid() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.GID)
}

// GetProcessGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessGroup() string {
	if ev.BaseEvent.ProcessContext.Process.Credentials.Group != nil {
		return ev.BaseEvent.ProcessContext.Process.Credentials.Group
	} else {
		return ""
	}
}

// GetProcessInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileChange_time() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetProcessInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileGid() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetProcessInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileInode() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetProcessInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileMode() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetProcessInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileModification_time() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetProcessInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileMount_id() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetProcessInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
}

// GetProcessInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileUid() int {
	return int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetProcessInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIs_kworker() bool {
	return ev.BaseEvent.ProcessContext.Process.PIDContext.IsKworker
}

// GetProcessIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIs_thread() bool {
	return ev.BaseEvent.ProcessContext.Process.IsThread
}

// GetProcessParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCap_effective() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.CapEffective)
}

// GetProcessParentCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCap_permitted() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.CapPermitted)
}

// GetProcessParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentComm() string {
	if ev.BaseEvent.ProcessContext.Parent.Comm != nil {
		return ev.BaseEvent.ProcessContext.Parent.Comm
	} else {
		return ""
	}
}

// GetProcessParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentContainerId() string {
	if ev.BaseEvent.ProcessContext.Parent.ContainerID != nil {
		return ev.BaseEvent.ProcessContext.Parent.ContainerID
	} else {
		return ""
	}
}

// GetProcessParentCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent))
}

// GetProcessParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEgid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.EGID)
}

// GetProcessParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEgroup() string {
	if ev.BaseEvent.ProcessContext.Parent.Credentials.EGroup != nil {
		return ev.BaseEvent.ProcessContext.Parent.Credentials.EGroup
	} else {
		return ""
	}
}

// GetProcessParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
	} else {
		return ""
	}
}

// GetProcessParentEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEuid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.EUID)
}

// GetProcessParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEuser() string {
	if ev.BaseEvent.ProcessContext.Parent.Credentials.EUser != nil {
		return ev.BaseEvent.ProcessContext.Parent.Credentials.EUser
	} else {
		return ""
	}
}

// GetProcessParentFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileChange_time() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.CTime)
}

// GetProcessParentFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileGid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.GID)
}

// GetProcessParentFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessParentFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
}

// GetProcessParentFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileInode() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.PathKey.Inode)
}

// GetProcessParentFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileMode() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.Mode)
}

// GetProcessParentFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileModification_time() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.MTime)
}

// GetProcessParentFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileMount_id() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.PathKey.MountID)
}

// GetProcessParentFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}

// GetProcessParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields))
}

// GetProcessParentFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileUid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.UID)
}

// GetProcessParentFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessParentFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsgid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.FSGID)
}

// GetProcessParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsgroup() string {
	if ev.BaseEvent.ProcessContext.Parent.Credentials.FSGroup != nil {
		return ev.BaseEvent.ProcessContext.Parent.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetProcessParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsuid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.FSUID)
}

// GetProcessParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsuser() string {
	if ev.BaseEvent.ProcessContext.Parent.Credentials.FSUser != nil {
		return ev.BaseEvent.ProcessContext.Parent.Credentials.FSUser
	} else {
		return ""
	}
}

// GetProcessParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentGid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.GID)
}

// GetProcessParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentGroup() string {
	if ev.BaseEvent.ProcessContext.Parent.Credentials.Group != nil {
		return ev.BaseEvent.ProcessContext.Parent.Credentials.Group
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileChange_time() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetProcessParentInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileGid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetProcessParentInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessParentInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileInode() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetProcessParentInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileMode() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetProcessParentInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileModification_time() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetProcessParentInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileMount_id() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetProcessParentInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetProcessParentInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
}

// GetProcessParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields))
}

// GetProcessParentInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileUid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetProcessParentInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetProcessParentIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIs_kworker() bool {
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.IsKworker
}

// GetProcessParentIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIs_thread() bool {
	return ev.BaseEvent.ProcessContext.Parent.IsThread
}

// GetProcessParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentPid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid)
}

// GetProcessParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentPpid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.PPid)
}

// GetProcessParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentTid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Tid)
}

// GetProcessParentTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentTty_name() string {
	if ev.BaseEvent.ProcessContext.Parent.TTYName != nil {
		return ev.BaseEvent.ProcessContext.Parent.TTYName
	} else {
		return ""
	}
}

// GetProcessParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUid() int {
	return int(ev.BaseEvent.ProcessContext.Parent.Credentials.UID)
}

// GetProcessParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUser() string {
	if ev.BaseEvent.ProcessContext.Parent.Credentials.User != nil {
		return ev.BaseEvent.ProcessContext.Parent.Credentials.User
	} else {
		return ""
	}
}

// GetProcessPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessPid() int {
	return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid)
}

// GetProcessPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessPpid() int {
	return int(ev.BaseEvent.ProcessContext.Process.PPid)
}

// GetProcessTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessTid() int {
	return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Tid)
}

// GetProcessTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetProcessTty_name() string {
	if ev.BaseEvent.ProcessContext.Process.TTYName != nil {
		return ev.BaseEvent.ProcessContext.Process.TTYName
	} else {
		return ""
	}
}

// GetProcessUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUid() int {
	return int(ev.BaseEvent.ProcessContext.Process.Credentials.UID)
}

// GetProcessUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUser() string {
	if ev.BaseEvent.ProcessContext.Process.Credentials.User != nil {
		return ev.BaseEvent.ProcessContext.Process.Credentials.User
	} else {
		return ""
	}
}

// GetPtraceRequest returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceRequest() int {
	return int(ev.PTrace.Request)
}

// GetPtraceRetval returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceRetval() int {
	return int(ev.PTrace.SyscallEvent.Retval)
}

// GetPtraceTraceeAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgs() []string {
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

// GetPtraceTraceeAncestorsArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgs_flags() []string {
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

// GetPtraceTraceeAncestorsArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgs_options() []string {
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

// GetPtraceTraceeAncestorsArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgs_truncated() []bool {
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

// GetPtraceTraceeAncestorsCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCap_effective() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.CapEffective)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCap_permitted() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.CapPermitted)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsComm() []string {
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
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.ContainerID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCreated_at() []int {
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
func (ev *Event) GetPtraceTraceeAncestorsEgid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.EGID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEgroup() []string {
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

// GetPtraceTraceeAncestorsEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEnvs_truncated() []bool {
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
func (ev *Event) GetPtraceTraceeAncestorsEuid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.EUID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEuser() []string {
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

// GetPtraceTraceeAncestorsFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileChange_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileFilesystem() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsFileGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileGroup() []string {
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

// GetPtraceTraceeAncestorsFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileIn_upper_layer() []bool {
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
func (ev *Event) GetPtraceTraceeAncestorsFileInode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileMode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileModification_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileMount_id() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileName() []string {
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

// GetPtraceTraceeAncestorsFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFilePackageSource_version() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsFileUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFileUser() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsFsgid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.FSGID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFsgroup() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsFsuid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.FSUID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsFsuser() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsGroup() []string {
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

// GetPtraceTraceeAncestorsInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileChange_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileFilesystem() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileGroup() []string {
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

// GetPtraceTraceeAncestorsInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileIn_upper_layer() []bool {
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
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileInode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileMode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileModification_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileMount_id() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileName() []string {
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

// GetPtraceTraceeAncestorsInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFilePackageSource_version() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsInterpreterFileUser() []string {
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

// GetPtraceTraceeAncestorsIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsIs_kworker() []bool {
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

// GetPtraceTraceeAncestorsIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsIs_thread() []bool {
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.IsThread
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsPid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsPid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PIDContext.Pid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsPpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsPpid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PPid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsTid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PIDContext.Tid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsTty_name() []string {
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
func (ev *Event) GetPtraceTraceeAncestorsUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsUser() []string {
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

// GetPtraceTraceeArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCap_effective() int {
	return int(ev.PTrace.Tracee.Process.Credentials.CapEffective)
}

// GetPtraceTraceeCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCap_permitted() int {
	return int(ev.PTrace.Tracee.Process.Credentials.CapPermitted)
}

// GetPtraceTraceeComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeComm() string {
	if ev.PTrace.Tracee.Process.Comm != nil {
		return ev.PTrace.Tracee.Process.Comm
	} else {
		return ""
	}
}

// GetPtraceTraceeContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeContainerId() string {
	if ev.PTrace.Tracee.Process.ContainerID != nil {
		return ev.PTrace.Tracee.Process.ContainerID
	} else {
		return ""
	}
}

// GetPtraceTraceeCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process))
}

// GetPtraceTraceeEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEgid() int {
	return int(ev.PTrace.Tracee.Process.Credentials.EGID)
}

// GetPtraceTraceeEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEgroup() string {
	if ev.PTrace.Tracee.Process.Credentials.EGroup != nil {
		return ev.PTrace.Tracee.Process.Credentials.EGroup
	} else {
		return ""
	}
}

// GetPtraceTraceeEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process)
	} else {
		return ""
	}
}

// GetPtraceTraceeEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEuid() int {
	return int(ev.PTrace.Tracee.Process.Credentials.EUID)
}

// GetPtraceTraceeEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEuser() string {
	if ev.PTrace.Tracee.Process.Credentials.EUser != nil {
		return ev.PTrace.Tracee.Process.Credentials.EUser
	} else {
		return ""
	}
}

// GetPtraceTraceeFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileChange_time() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.CTime)
}

// GetPtraceTraceeFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileGid() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.GID)
}

// GetPtraceTraceeFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileInode() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.Inode)
}

// GetPtraceTraceeFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileMode() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.Mode)
}

// GetPtraceTraceeFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileModification_time() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.MTime)
}

// GetPtraceTraceeFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileMount_id() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.MountID)
}

// GetPtraceTraceeFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
}

// GetPtraceTraceeFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileUid() int {
	return int(ev.PTrace.Tracee.Process.FileEvent.FileFields.UID)
}

// GetPtraceTraceeFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsgid() int {
	return int(ev.PTrace.Tracee.Process.Credentials.FSGID)
}

// GetPtraceTraceeFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsgroup() string {
	if ev.PTrace.Tracee.Process.Credentials.FSGroup != nil {
		return ev.PTrace.Tracee.Process.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetPtraceTraceeFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsuid() int {
	return int(ev.PTrace.Tracee.Process.Credentials.FSUID)
}

// GetPtraceTraceeFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsuser() string {
	if ev.PTrace.Tracee.Process.Credentials.FSUser != nil {
		return ev.PTrace.Tracee.Process.Credentials.FSUser
	} else {
		return ""
	}
}

// GetPtraceTraceeGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeGid() int {
	return int(ev.PTrace.Tracee.Process.Credentials.GID)
}

// GetPtraceTraceeGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeGroup() string {
	if ev.PTrace.Tracee.Process.Credentials.Group != nil {
		return ev.PTrace.Tracee.Process.Credentials.Group
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileChange_time() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetPtraceTraceeInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileGid() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetPtraceTraceeInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileInode() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetPtraceTraceeInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileMode() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetPtraceTraceeInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileModification_time() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetPtraceTraceeInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileMount_id() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetPtraceTraceeInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
}

// GetPtraceTraceeInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileUid() int {
	return int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetPtraceTraceeInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIs_kworker() bool {
	return ev.PTrace.Tracee.Process.PIDContext.IsKworker
}

// GetPtraceTraceeIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIs_thread() bool {
	return ev.PTrace.Tracee.Process.IsThread
}

// GetPtraceTraceeParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCap_effective() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.CapEffective)
}

// GetPtraceTraceeParentCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCap_permitted() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.CapPermitted)
}

// GetPtraceTraceeParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentComm() string {
	if ev.PTrace.Tracee.Parent.Comm != nil {
		return ev.PTrace.Tracee.Parent.Comm
	} else {
		return ""
	}
}

// GetPtraceTraceeParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentContainerId() string {
	if ev.PTrace.Tracee.Parent.ContainerID != nil {
		return ev.PTrace.Tracee.Parent.ContainerID
	} else {
		return ""
	}
}

// GetPtraceTraceeParentCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent))
}

// GetPtraceTraceeParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEgid() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.EGID)
}

// GetPtraceTraceeParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEgroup() string {
	if ev.PTrace.Tracee.Parent.Credentials.EGroup != nil {
		return ev.PTrace.Tracee.Parent.Credentials.EGroup
	} else {
		return ""
	}
}

// GetPtraceTraceeParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEuid() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.EUID)
}

// GetPtraceTraceeParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEuser() string {
	if ev.PTrace.Tracee.Parent.Credentials.EUser != nil {
		return ev.PTrace.Tracee.Parent.Credentials.EUser
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileChange_time() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.CTime)
}

// GetPtraceTraceeParentFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileGid() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.GID)
}

// GetPtraceTraceeParentFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
}

// GetPtraceTraceeParentFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileInode() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.PathKey.Inode)
}

// GetPtraceTraceeParentFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileMode() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.Mode)
}

// GetPtraceTraceeParentFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileModification_time() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.MTime)
}

// GetPtraceTraceeParentFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileMount_id() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.PathKey.MountID)
}

// GetPtraceTraceeParentFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent)
}

// GetPtraceTraceeParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields))
}

// GetPtraceTraceeParentFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileUid() int {
	return int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.UID)
}

// GetPtraceTraceeParentFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsgid() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.FSGID)
}

// GetPtraceTraceeParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsgroup() string {
	if ev.PTrace.Tracee.Parent.Credentials.FSGroup != nil {
		return ev.PTrace.Tracee.Parent.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetPtraceTraceeParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsuid() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.FSUID)
}

// GetPtraceTraceeParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsuser() string {
	if ev.PTrace.Tracee.Parent.Credentials.FSUser != nil {
		return ev.PTrace.Tracee.Parent.Credentials.FSUser
	} else {
		return ""
	}
}

// GetPtraceTraceeParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentGid() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.GID)
}

// GetPtraceTraceeParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentGroup() string {
	if ev.PTrace.Tracee.Parent.Credentials.Group != nil {
		return ev.PTrace.Tracee.Parent.Credentials.Group
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileChange_time() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetPtraceTraceeParentInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileGid() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetPtraceTraceeParentInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeParentInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileInode() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetPtraceTraceeParentInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileMode() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetPtraceTraceeParentInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileModification_time() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetPtraceTraceeParentInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileMount_id() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetPtraceTraceeParentInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields))
}

// GetPtraceTraceeParentInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileUid() int {
	return int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetPtraceTraceeParentInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetPtraceTraceeParentIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIs_kworker() bool {
	return ev.PTrace.Tracee.Parent.PIDContext.IsKworker
}

// GetPtraceTraceeParentIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIs_thread() bool {
	return ev.PTrace.Tracee.Parent.IsThread
}

// GetPtraceTraceeParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentPid() int {
	return int(ev.PTrace.Tracee.Parent.PIDContext.Pid)
}

// GetPtraceTraceeParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentPpid() int {
	return int(ev.PTrace.Tracee.Parent.PPid)
}

// GetPtraceTraceeParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentTid() int {
	return int(ev.PTrace.Tracee.Parent.PIDContext.Tid)
}

// GetPtraceTraceeParentTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentTty_name() string {
	if ev.PTrace.Tracee.Parent.TTYName != nil {
		return ev.PTrace.Tracee.Parent.TTYName
	} else {
		return ""
	}
}

// GetPtraceTraceeParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUid() int {
	return int(ev.PTrace.Tracee.Parent.Credentials.UID)
}

// GetPtraceTraceeParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUser() string {
	if ev.PTrace.Tracee.Parent.Credentials.User != nil {
		return ev.PTrace.Tracee.Parent.Credentials.User
	} else {
		return ""
	}
}

// GetPtraceTraceePid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceePid() int {
	return int(ev.PTrace.Tracee.Process.PIDContext.Pid)
}

// GetPtraceTraceePpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceePpid() int {
	return int(ev.PTrace.Tracee.Process.PPid)
}

// GetPtraceTraceeTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeTid() int {
	return int(ev.PTrace.Tracee.Process.PIDContext.Tid)
}

// GetPtraceTraceeTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeTty_name() string {
	if ev.PTrace.Tracee.Process.TTYName != nil {
		return ev.PTrace.Tracee.Process.TTYName
	} else {
		return ""
	}
}

// GetPtraceTraceeUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUid() int {
	return int(ev.PTrace.Tracee.Process.Credentials.UID)
}

// GetPtraceTraceeUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUser() string {
	if ev.PTrace.Tracee.Process.Credentials.User != nil {
		return ev.PTrace.Tracee.Process.Credentials.User
	} else {
		return ""
	}
}

// GetRemovexattrFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileChange_time() int {
	return int(ev.RemoveXAttr.File.FileFields.CTime)
}

// GetRemovexattrFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileDestinationName() string {
	if ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr) != nil {
		return ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr)
	} else {
		return ""
	}
}

// GetRemovexattrFileDestinationNamespace returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileDestinationNamespace() string {
	if ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr) != nil {
		return ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr)
	} else {
		return ""
	}
}

// GetRemovexattrFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileGid() int {
	return int(ev.RemoveXAttr.File.FileFields.GID)
}

// GetRemovexattrFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields)
	} else {
		return ""
	}
}

// GetRemovexattrFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileInode() int {
	return int(ev.RemoveXAttr.File.FileFields.PathKey.Inode)
}

// GetRemovexattrFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileMode() int {
	return int(ev.RemoveXAttr.File.FileFields.Mode)
}

// GetRemovexattrFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileModification_time() int {
	return int(ev.RemoveXAttr.File.FileFields.MTime)
}

// GetRemovexattrFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileMount_id() int {
	return int(ev.RemoveXAttr.File.FileFields.PathKey.MountID)
}

// GetRemovexattrFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File)
	} else {
		return ""
	}
}

// GetRemovexattrFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.RemoveXAttr.File.FileFields))
}

// GetRemovexattrFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileUid() int {
	return int(ev.RemoveXAttr.File.FileFields.UID)
}

// GetRemovexattrFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields)
	} else {
		return ""
	}
}

// GetRemovexattrRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrRetval() int {
	return int(ev.RemoveXAttr.SyscallEvent.Retval)
}

// GetRenameFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileChange_time() int {
	return int(ev.Rename.Old.FileFields.CTime)
}

// GetRenameFileDestinationChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationChange_time() int {
	return int(ev.Rename.New.FileFields.CTime)
}

// GetRenameFileDestinationFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationGid() int {
	return int(ev.Rename.New.FileFields.GID)
}

// GetRenameFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields)
	} else {
		return ""
	}
}

// GetRenameFileDestinationHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationInode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationInode() int {
	return int(ev.Rename.New.FileFields.PathKey.Inode)
}

// GetRenameFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationMode() int {
	return int(ev.Rename.New.FileFields.Mode)
}

// GetRenameFileDestinationModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationModification_time() int {
	return int(ev.Rename.New.FileFields.MTime)
}

// GetRenameFileDestinationMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationMount_id() int {
	return int(ev.Rename.New.FileFields.PathKey.MountID)
}

// GetRenameFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationPackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationPackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New)
	} else {
		return ""
	}
}

// GetRenameFileDestinationPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New)
}

// GetRenameFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Rename.New.FileFields))
}

// GetRenameFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationUid() int {
	return int(ev.Rename.New.FileFields.UID)
}

// GetRenameFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields)
	} else {
		return ""
	}
}

// GetRenameFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileGid() int {
	return int(ev.Rename.Old.FileFields.GID)
}

// GetRenameFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields)
	} else {
		return ""
	}
}

// GetRenameFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileInode() int {
	return int(ev.Rename.Old.FileFields.PathKey.Inode)
}

// GetRenameFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileMode() int {
	return int(ev.Rename.Old.FileFields.Mode)
}

// GetRenameFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileModification_time() int {
	return int(ev.Rename.Old.FileFields.MTime)
}

// GetRenameFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileMount_id() int {
	return int(ev.Rename.Old.FileFields.PathKey.MountID)
}

// GetRenameFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old)
}

// GetRenameFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old)
	} else {
		return ""
	}
}

// GetRenameFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old)
}

// GetRenameFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Rename.Old.FileFields))
}

// GetRenameFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileUid() int {
	return int(ev.Rename.Old.FileFields.UID)
}

// GetRenameFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields)
	} else {
		return ""
	}
}

// GetRenameRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRenameRetval() int {
	return int(ev.Rename.SyscallEvent.Retval)
}

// GetRmdirFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileChange_time() int {
	return int(ev.Rmdir.File.FileFields.CTime)
}

// GetRmdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileGid() int {
	return int(ev.Rmdir.File.FileFields.GID)
}

// GetRmdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields)
	} else {
		return ""
	}
}

// GetRmdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileInode() int {
	return int(ev.Rmdir.File.FileFields.PathKey.Inode)
}

// GetRmdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileMode() int {
	return int(ev.Rmdir.File.FileFields.Mode)
}

// GetRmdirFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileModification_time() int {
	return int(ev.Rmdir.File.FileFields.MTime)
}

// GetRmdirFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileMount_id() int {
	return int(ev.Rmdir.File.FileFields.PathKey.MountID)
}

// GetRmdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File)
}

// GetRmdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File)
	} else {
		return ""
	}
}

// GetRmdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File)
}

// GetRmdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Rmdir.File.FileFields))
}

// GetRmdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileUid() int {
	return int(ev.Rmdir.File.FileFields.UID)
}

// GetRmdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields)
	} else {
		return ""
	}
}

// GetRmdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirRetval() int {
	return int(ev.Rmdir.SyscallEvent.Retval)
}

// GetSelinuxBoolName returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolName() string {
	if ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux) != nil {
		return ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux)
	} else {
		return ""
	}
}

// GetSelinuxBoolState returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolState() string {
	if ev.SELinux.BoolChangeValue != nil {
		return ev.SELinux.BoolChangeValue
	} else {
		return ""
	}
}

// GetSelinuxBool_commitState returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBool_commitState() bool {
	return ev.SELinux.BoolCommitValue
}

// GetSelinuxEnforceStatus returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxEnforceStatus() string {
	if ev.SELinux.EnforceStatus != nil {
		return ev.SELinux.EnforceStatus
	} else {
		return ""
	}
}

// GetSetgidEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidEgid() int {
	return int(ev.SetGID.EGID)
}

// GetSetgidEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidEgroup() string {
	if ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID) != nil {
		return ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID)
	} else {
		return ""
	}
}

// GetSetgidFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidFsgid() int {
	return int(ev.SetGID.FSGID)
}

// GetSetgidFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidFsgroup() string {
	if ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID) != nil {
		return ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID)
	} else {
		return ""
	}
}

// GetSetgidGid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidGid() int {
	return int(ev.SetGID.GID)
}

// GetSetgidGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidGroup() string {
	if ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID) != nil {
		return ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID)
	} else {
		return ""
	}
}

// GetSetuidEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidEuid() int {
	return int(ev.SetUID.EUID)
}

// GetSetuidEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidEuser() string {
	if ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID) != nil {
		return ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID)
	} else {
		return ""
	}
}

// GetSetuidFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidFsuid() int {
	return int(ev.SetUID.FSUID)
}

// GetSetuidFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidFsuser() string {
	if ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID) != nil {
		return ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID)
	} else {
		return ""
	}
}

// GetSetuidUid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidUid() int {
	return int(ev.SetUID.UID)
}

// GetSetuidUser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidUser() string {
	if ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID) != nil {
		return ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID)
	} else {
		return ""
	}
}

// GetSetxattrFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileChange_time() int {
	return int(ev.SetXAttr.File.FileFields.CTime)
}

// GetSetxattrFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileDestinationName() string {
	if ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr) != nil {
		return ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr)
	} else {
		return ""
	}
}

// GetSetxattrFileDestinationNamespace returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileDestinationNamespace() string {
	if ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr) != nil {
		return ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr)
	} else {
		return ""
	}
}

// GetSetxattrFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileGid() int {
	return int(ev.SetXAttr.File.FileFields.GID)
}

// GetSetxattrFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields)
	} else {
		return ""
	}
}

// GetSetxattrFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileInode() int {
	return int(ev.SetXAttr.File.FileFields.PathKey.Inode)
}

// GetSetxattrFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileMode() int {
	return int(ev.SetXAttr.File.FileFields.Mode)
}

// GetSetxattrFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileModification_time() int {
	return int(ev.SetXAttr.File.FileFields.MTime)
}

// GetSetxattrFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileMount_id() int {
	return int(ev.SetXAttr.File.FileFields.PathKey.MountID)
}

// GetSetxattrFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File)
	} else {
		return ""
	}
}

// GetSetxattrFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File)
}

// GetSetxattrFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.SetXAttr.File.FileFields))
}

// GetSetxattrFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileUid() int {
	return int(ev.SetXAttr.File.FileFields.UID)
}

// GetSetxattrFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields)
	} else {
		return ""
	}
}

// GetSetxattrRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrRetval() int {
	return int(ev.SetXAttr.SyscallEvent.Retval)
}

// GetSignalPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalPid() int {
	return int(ev.Signal.PID)
}

// GetSignalRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSignalRetval() int {
	return int(ev.Signal.SyscallEvent.Retval)
}

// GetSignalTargetAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgs() []string {
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

// GetSignalTargetAncestorsArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgs_flags() []string {
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

// GetSignalTargetAncestorsArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgs_options() []string {
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

// GetSignalTargetAncestorsArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgs_truncated() []bool {
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

// GetSignalTargetAncestorsCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCap_effective() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.CapEffective)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCap_permitted() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.CapPermitted)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsComm() []string {
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
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.ContainerID
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCreated_at() []int {
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
func (ev *Event) GetSignalTargetAncestorsEgid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.EGID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEgroup() []string {
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

// GetSignalTargetAncestorsEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEnvs_truncated() []bool {
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
func (ev *Event) GetSignalTargetAncestorsEuid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.EUID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEuser() []string {
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

// GetSignalTargetAncestorsFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileChange_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.CTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileFilesystem() []string {
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
func (ev *Event) GetSignalTargetAncestorsFileGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileGroup() []string {
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

// GetSignalTargetAncestorsFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileIn_upper_layer() []bool {
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
func (ev *Event) GetSignalTargetAncestorsFileInode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileMode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.Mode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileModification_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.MTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileMount_id() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileName() []string {
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

// GetSignalTargetAncestorsFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFilePackageSource_version() []string {
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
func (ev *Event) GetSignalTargetAncestorsFileUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.FileEvent.FileFields.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFileUser() []string {
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
func (ev *Event) GetSignalTargetAncestorsFsgid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.FSGID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFsgroup() []string {
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
func (ev *Event) GetSignalTargetAncestorsFsuid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.FSUID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsFsuser() []string {
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
func (ev *Event) GetSignalTargetAncestorsGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsGroup() []string {
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

// GetSignalTargetAncestorsInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileChange_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileFilesystem() []string {
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
func (ev *Event) GetSignalTargetAncestorsInterpreterFileGid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileGroup() []string {
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

// GetSignalTargetAncestorsInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileIn_upper_layer() []bool {
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
func (ev *Event) GetSignalTargetAncestorsInterpreterFileInode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileMode() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileModification_time() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileMount_id() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileName() []string {
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

// GetSignalTargetAncestorsInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFilePackageSource_version() []string {
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
func (ev *Event) GetSignalTargetAncestorsInterpreterFileUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsInterpreterFileUser() []string {
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

// GetSignalTargetAncestorsIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsIs_kworker() []bool {
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

// GetSignalTargetAncestorsIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsIs_thread() []bool {
	var values []bool
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := element.ProcessContext.Process.IsThread
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsPid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PIDContext.Pid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsPpid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PPid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsTid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.PIDContext.Tid)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsTty_name() []string {
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
func (ev *Event) GetSignalTargetAncestorsUid() []int {
	var values []int
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := int(element.ProcessContext.Process.Credentials.UID)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsUser() []string {
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

// GetSignalTargetArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCap_effective() int {
	return int(ev.Signal.Target.Process.Credentials.CapEffective)
}

// GetSignalTargetCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCap_permitted() int {
	return int(ev.Signal.Target.Process.Credentials.CapPermitted)
}

// GetSignalTargetComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetComm() string {
	if ev.Signal.Target.Process.Comm != nil {
		return ev.Signal.Target.Process.Comm
	} else {
		return ""
	}
}

// GetSignalTargetContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetContainerId() string {
	if ev.Signal.Target.Process.ContainerID != nil {
		return ev.Signal.Target.Process.ContainerID
	} else {
		return ""
	}
}

// GetSignalTargetCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process))
}

// GetSignalTargetEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEgid() int {
	return int(ev.Signal.Target.Process.Credentials.EGID)
}

// GetSignalTargetEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEgroup() string {
	if ev.Signal.Target.Process.Credentials.EGroup != nil {
		return ev.Signal.Target.Process.Credentials.EGroup
	} else {
		return ""
	}
}

// GetSignalTargetEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process)
	} else {
		return ""
	}
}

// GetSignalTargetEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEuid() int {
	return int(ev.Signal.Target.Process.Credentials.EUID)
}

// GetSignalTargetEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEuser() string {
	if ev.Signal.Target.Process.Credentials.EUser != nil {
		return ev.Signal.Target.Process.Credentials.EUser
	} else {
		return ""
	}
}

// GetSignalTargetFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileChange_time() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.CTime)
}

// GetSignalTargetFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileGid() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.GID)
}

// GetSignalTargetFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileInode() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.PathKey.Inode)
}

// GetSignalTargetFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileMode() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.Mode)
}

// GetSignalTargetFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileModification_time() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.MTime)
}

// GetSignalTargetFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileMount_id() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.PathKey.MountID)
}

// GetSignalTargetFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
}

// GetSignalTargetFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileUid() int {
	return int(ev.Signal.Target.Process.FileEvent.FileFields.UID)
}

// GetSignalTargetFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsgid() int {
	return int(ev.Signal.Target.Process.Credentials.FSGID)
}

// GetSignalTargetFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsgroup() string {
	if ev.Signal.Target.Process.Credentials.FSGroup != nil {
		return ev.Signal.Target.Process.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetSignalTargetFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsuid() int {
	return int(ev.Signal.Target.Process.Credentials.FSUID)
}

// GetSignalTargetFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsuser() string {
	if ev.Signal.Target.Process.Credentials.FSUser != nil {
		return ev.Signal.Target.Process.Credentials.FSUser
	} else {
		return ""
	}
}

// GetSignalTargetGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetGid() int {
	return int(ev.Signal.Target.Process.Credentials.GID)
}

// GetSignalTargetGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetGroup() string {
	if ev.Signal.Target.Process.Credentials.Group != nil {
		return ev.Signal.Target.Process.Credentials.Group
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileChange_time() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetSignalTargetInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileGid() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetSignalTargetInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileInode() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetSignalTargetInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileMode() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetSignalTargetInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileModification_time() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetSignalTargetInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileMount_id() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetSignalTargetInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
}

// GetSignalTargetInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileUid() int {
	return int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetSignalTargetInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIs_kworker() bool {
	return ev.Signal.Target.Process.PIDContext.IsKworker
}

// GetSignalTargetIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIs_thread() bool {
	return ev.Signal.Target.Process.IsThread
}

// GetSignalTargetParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgs() string {
	if ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentArgs_flags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgs_flags() []string {
	if ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentArgs_options returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgs_options() []string {
	if ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentArgs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgv() []string {
	if ev.FieldHandlers.ResolveProcessArgv(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgv(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgv0() string {
	if ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentCap_effective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCap_effective() int {
	return int(ev.Signal.Target.Parent.Credentials.CapEffective)
}

// GetSignalTargetParentCap_permitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCap_permitted() int {
	return int(ev.Signal.Target.Parent.Credentials.CapPermitted)
}

// GetSignalTargetParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentComm() string {
	if ev.Signal.Target.Parent.Comm != nil {
		return ev.Signal.Target.Parent.Comm
	} else {
		return ""
	}
}

// GetSignalTargetParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentContainerId() string {
	if ev.Signal.Target.Parent.ContainerID != nil {
		return ev.Signal.Target.Parent.ContainerID
	} else {
		return ""
	}
}

// GetSignalTargetParentCreated_at returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCreated_at() int {
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent))
}

// GetSignalTargetParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEgid() int {
	return int(ev.Signal.Target.Parent.Credentials.EGID)
}

// GetSignalTargetParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEgroup() string {
	if ev.Signal.Target.Parent.Credentials.EGroup != nil {
		return ev.Signal.Target.Parent.Credentials.EGroup
	} else {
		return ""
	}
}

// GetSignalTargetParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvp() []string {
	if ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvs() []string {
	if ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent) != nil {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent)
	} else {
		return ""
	}
}

// GetSignalTargetParentEnvs_truncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvs_truncated() bool {
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEuid() int {
	return int(ev.Signal.Target.Parent.Credentials.EUID)
}

// GetSignalTargetParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEuser() string {
	if ev.Signal.Target.Parent.Credentials.EUser != nil {
		return ev.Signal.Target.Parent.Credentials.EUser
	} else {
		return ""
	}
}

// GetSignalTargetParentFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileChange_time() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.CTime)
}

// GetSignalTargetParentFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileGid() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.GID)
}

// GetSignalTargetParentFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetParentFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
}

// GetSignalTargetParentFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileInode() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.PathKey.Inode)
}

// GetSignalTargetParentFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileMode() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.Mode)
}

// GetSignalTargetParentFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileModification_time() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.MTime)
}

// GetSignalTargetParentFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileMount_id() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.PathKey.MountID)
}

// GetSignalTargetParentFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent)
}

// GetSignalTargetParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Parent.FileEvent.FileFields))
}

// GetSignalTargetParentFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileUid() int {
	return int(ev.Signal.Target.Parent.FileEvent.FileFields.UID)
}

// GetSignalTargetParentFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetParentFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsgid() int {
	return int(ev.Signal.Target.Parent.Credentials.FSGID)
}

// GetSignalTargetParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsgroup() string {
	if ev.Signal.Target.Parent.Credentials.FSGroup != nil {
		return ev.Signal.Target.Parent.Credentials.FSGroup
	} else {
		return ""
	}
}

// GetSignalTargetParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsuid() int {
	return int(ev.Signal.Target.Parent.Credentials.FSUID)
}

// GetSignalTargetParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsuser() string {
	if ev.Signal.Target.Parent.Credentials.FSUser != nil {
		return ev.Signal.Target.Parent.Credentials.FSUser
	} else {
		return ""
	}
}

// GetSignalTargetParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentGid() int {
	return int(ev.Signal.Target.Parent.Credentials.GID)
}

// GetSignalTargetParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentGroup() string {
	if ev.Signal.Target.Parent.Credentials.Group != nil {
		return ev.Signal.Target.Parent.Credentials.Group
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileChange_time() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.CTime)
}

// GetSignalTargetParentInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileGid() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.GID)
}

// GetSignalTargetParentInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetParentInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileInode() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode)
}

// GetSignalTargetParentInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileMode() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.Mode)
}

// GetSignalTargetParentInterpreterFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileModification_time() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.MTime)
}

// GetSignalTargetParentInterpreterFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileMount_id() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID)
}

// GetSignalTargetParentInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	} else {
		return ""
	}
}

// GetSignalTargetParentInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
}

// GetSignalTargetParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields))
}

// GetSignalTargetParentInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileUid() int {
	return int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.UID)
}

// GetSignalTargetParentInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
	} else {
		return ""
	}
}

// GetSignalTargetParentIs_kworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIs_kworker() bool {
	return ev.Signal.Target.Parent.PIDContext.IsKworker
}

// GetSignalTargetParentIs_thread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIs_thread() bool {
	return ev.Signal.Target.Parent.IsThread
}

// GetSignalTargetParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentPid() int {
	return int(ev.Signal.Target.Parent.PIDContext.Pid)
}

// GetSignalTargetParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentPpid() int {
	return int(ev.Signal.Target.Parent.PPid)
}

// GetSignalTargetParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentTid() int {
	return int(ev.Signal.Target.Parent.PIDContext.Tid)
}

// GetSignalTargetParentTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentTty_name() string {
	if ev.Signal.Target.Parent.TTYName != nil {
		return ev.Signal.Target.Parent.TTYName
	} else {
		return ""
	}
}

// GetSignalTargetParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUid() int {
	return int(ev.Signal.Target.Parent.Credentials.UID)
}

// GetSignalTargetParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUser() string {
	if ev.Signal.Target.Parent.Credentials.User != nil {
		return ev.Signal.Target.Parent.Credentials.User
	} else {
		return ""
	}
}

// GetSignalTargetPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetPid() int {
	return int(ev.Signal.Target.Process.PIDContext.Pid)
}

// GetSignalTargetPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetPpid() int {
	return int(ev.Signal.Target.Process.PPid)
}

// GetSignalTargetTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetTid() int {
	return int(ev.Signal.Target.Process.PIDContext.Tid)
}

// GetSignalTargetTty_name returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetTty_name() string {
	if ev.Signal.Target.Process.TTYName != nil {
		return ev.Signal.Target.Process.TTYName
	} else {
		return ""
	}
}

// GetSignalTargetUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUid() int {
	return int(ev.Signal.Target.Process.Credentials.UID)
}

// GetSignalTargetUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUser() string {
	if ev.Signal.Target.Process.Credentials.User != nil {
		return ev.Signal.Target.Process.Credentials.User
	} else {
		return ""
	}
}

// GetSignalType returns the value of the field, resolving if necessary
func (ev *Event) GetSignalType() int {
	return int(ev.Signal.Type)
}

// GetSpliceFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileChange_time() int {
	return int(ev.Splice.File.FileFields.CTime)
}

// GetSpliceFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileGid() int {
	return int(ev.Splice.File.FileFields.GID)
}

// GetSpliceFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields)
	} else {
		return ""
	}
}

// GetSpliceFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileInode() int {
	return int(ev.Splice.File.FileFields.PathKey.Inode)
}

// GetSpliceFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileMode() int {
	return int(ev.Splice.File.FileFields.Mode)
}

// GetSpliceFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileModification_time() int {
	return int(ev.Splice.File.FileFields.MTime)
}

// GetSpliceFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileMount_id() int {
	return int(ev.Splice.File.FileFields.PathKey.MountID)
}

// GetSpliceFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File)
}

// GetSpliceFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File)
	} else {
		return ""
	}
}

// GetSpliceFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File)
}

// GetSpliceFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Splice.File.FileFields))
}

// GetSpliceFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileUid() int {
	return int(ev.Splice.File.FileFields.UID)
}

// GetSpliceFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields)
	} else {
		return ""
	}
}

// GetSplicePipe_entry_flag returns the value of the field, resolving if necessary
func (ev *Event) GetSplicePipe_entry_flag() int {
	return int(ev.Splice.PipeEntryFlag)
}

// GetSplicePipe_exit_flag returns the value of the field, resolving if necessary
func (ev *Event) GetSplicePipe_exit_flag() int {
	return int(ev.Splice.PipeExitFlag)
}

// GetSpliceRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceRetval() int {
	return int(ev.Splice.SyscallEvent.Retval)
}

// GetUnlinkFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileChange_time() int {
	return int(ev.Unlink.File.FileFields.CTime)
}

// GetUnlinkFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileGid() int {
	return int(ev.Unlink.File.FileFields.GID)
}

// GetUnlinkFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields)
	} else {
		return ""
	}
}

// GetUnlinkFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileInode() int {
	return int(ev.Unlink.File.FileFields.PathKey.Inode)
}

// GetUnlinkFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileMode() int {
	return int(ev.Unlink.File.FileFields.Mode)
}

// GetUnlinkFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileModification_time() int {
	return int(ev.Unlink.File.FileFields.MTime)
}

// GetUnlinkFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileMount_id() int {
	return int(ev.Unlink.File.FileFields.PathKey.MountID)
}

// GetUnlinkFileName returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File)
}

// GetUnlinkFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File)
	} else {
		return ""
	}
}

// GetUnlinkFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File)
}

// GetUnlinkFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Unlink.File.FileFields))
}

// GetUnlinkFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileUid() int {
	return int(ev.Unlink.File.FileFields.UID)
}

// GetUnlinkFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields)
	} else {
		return ""
	}
}

// GetUnlinkFlags returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFlags() int {
	return int(ev.Unlink.Flags)
}

// GetUnlinkRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkRetval() int {
	return int(ev.Unlink.SyscallEvent.Retval)
}

// GetUnload_moduleName returns the value of the field, resolving if necessary
func (ev *Event) GetUnload_moduleName() string {
	if ev.UnloadModule.Name != nil {
		return ev.UnloadModule.Name
	} else {
		return ""
	}
}

// GetUnload_moduleRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUnload_moduleRetval() int {
	return int(ev.UnloadModule.SyscallEvent.Retval)
}

// GetUtimesFileChange_time returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileChange_time() int {
	return int(ev.Utimes.File.FileFields.CTime)
}

// GetUtimesFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileFilesystem() string {
	if ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileGid() int {
	return int(ev.Utimes.File.FileFields.GID)
}

// GetUtimesFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileGroup() string {
	if ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields)
	} else {
		return ""
	}
}

// GetUtimesFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileHashes() []string {
	if ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFileIn_upper_layer returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileIn_upper_layer() bool {
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileInode() int {
	return int(ev.Utimes.File.FileFields.PathKey.Inode)
}

// GetUtimesFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileMode() int {
	return int(ev.Utimes.File.FileFields.Mode)
}

// GetUtimesFileModification_time returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileModification_time() int {
	return int(ev.Utimes.File.FileFields.MTime)
}

// GetUtimesFileMount_id returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileMount_id() int {
	return int(ev.Utimes.File.FileFields.PathKey.MountID)
}

// GetUtimesFileName returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileName() string {
	if ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileNameLength() int {
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File)
}

// GetUtimesFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageName() string {
	if ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFilePackageSource_version returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageSource_version() string {
	if ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageVersion() string {
	if ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePath() string {
	if ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File) != nil {
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File)
	} else {
		return ""
	}
}

// GetUtimesFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePathLength() int {
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File)
}

// GetUtimesFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileRights() int {
	return int(ev.FieldHandlers.ResolveRights(ev, &ev.Utimes.File.FileFields))
}

// GetUtimesFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileUid() int {
	return int(ev.Utimes.File.FileFields.UID)
}

// GetUtimesFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileUser() string {
	if ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields) != nil {
		return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields)
	} else {
		return ""
	}
}

// GetUtimesRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesRetval() int {
	return int(ev.Utimes.SyscallEvent.Retval)
}
