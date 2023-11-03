// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix
// +build unix

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"net"
	"time"
)

// GetBindAddrFamily returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrFamily() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "bind" {
		return zeroValue
	}
	return ev.Bind.AddrFamily
}

// GetBindAddrIp returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrIp() net.IPNet {
	zeroValue := net.IPNet{}
	if ev.GetEventType().String() != "bind" {
		return zeroValue
	}
	return ev.Bind.Addr.IPNet
}

// GetBindAddrPort returns the value of the field, resolving if necessary
func (ev *Event) GetBindAddrPort() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "bind" {
		return zeroValue
	}
	return ev.Bind.Addr.Port
}

// GetBindRetval returns the value of the field, resolving if necessary
func (ev *Event) GetBindRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "bind" {
		return zeroValue
	}
	return ev.Bind.SyscallEvent.Retval
}

// GetBpfCmd returns the value of the field, resolving if necessary
func (ev *Event) GetBpfCmd() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Cmd
}

// GetBpfMapName returns the value of the field, resolving if necessary
func (ev *Event) GetBpfMapName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Map.Name
}

// GetBpfMapType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfMapType() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Map.Type
}

// GetBpfProgAttachType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgAttachType() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Program.AttachType
}

// GetBpfProgHelpers returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgHelpers() []uint32 {
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	resolvedField := ev.BPF.Program.Helpers
	fieldCopy := make([]uint32, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetBpfProgName returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Program.Name
}

// GetBpfProgTag returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgTag() string {
	zeroValue := ""
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Program.Tag
}

// GetBpfProgType returns the value of the field, resolving if necessary
func (ev *Event) GetBpfProgType() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.Program.Type
}

// GetBpfRetval returns the value of the field, resolving if necessary
func (ev *Event) GetBpfRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "bpf" {
		return zeroValue
	}
	return ev.BPF.SyscallEvent.Retval
}

// GetCapsetCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetCapsetCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "capset" {
		return zeroValue
	}
	return ev.Capset.CapEffective
}

// GetCapsetCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetCapsetCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "capset" {
		return zeroValue
	}
	return ev.Capset.CapPermitted
}

// GetChmodFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.CTime
}

// GetChmodFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileDestinationMode() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.Mode
}

// GetChmodFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileDestinationRights() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.Mode
}

// GetChmodFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File)
}

// GetChmodFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.GID
}

// GetChmodFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetChmodFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.PathKey.Inode
}

// GetChmodFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.Mode
}

// GetChmodFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.MTime
}

// GetChmodFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.PathKey.MountID
}

// GetChmodFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File)
}

// GetChmodFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File))
}

// GetChmodFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File)
}

// GetChmodFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File)
}

// GetChmodFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File)
}

// GetChmodFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File)
}

// GetChmodFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File))
}

// GetChmodFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Chmod.File.FileFields)
}

// GetChmodFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.File.FileFields.UID
}

// GetChmodFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChmodFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields)
}

// GetChmodRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChmodRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "chmod" {
		return zeroValue
	}
	return ev.Chmod.SyscallEvent.Retval
}

// GetChownFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.CTime
}

// GetChownFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationGid() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.GID
}

// GetChownFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown)
}

// GetChownFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationUid() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.UID
}

// GetChownFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileDestinationUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown)
}

// GetChownFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File)
}

// GetChownFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.GID
}

// GetChownFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields)
}

// GetChownFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetChownFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chown.File.FileFields)
}

// GetChownFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.PathKey.Inode
}

// GetChownFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.Mode
}

// GetChownFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.MTime
}

// GetChownFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.PathKey.MountID
}

// GetChownFileName returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File)
}

// GetChownFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File))
}

// GetChownFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File)
}

// GetChownFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File)
}

// GetChownFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File)
}

// GetChownFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File)
}

// GetChownFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetChownFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File))
}

// GetChownFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Chown.File.FileFields)
}

// GetChownFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.File.FileFields.UID
}

// GetChownFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetChownFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields)
}

// GetChownRetval returns the value of the field, resolving if necessary
func (ev *Event) GetChownRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "chown" {
		return zeroValue
	}
	return ev.Chown.SyscallEvent.Retval
}

// GetContainerCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetContainerCreatedAt() int {
	zeroValue := 0
	if ev.BaseEvent.ContainerContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext)
}

// GetContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetContainerId() string {
	zeroValue := ""
	if ev.BaseEvent.ContainerContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
}

// GetContainerTags returns the value of the field, resolving if necessary
func (ev *Event) GetContainerTags() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ContainerContext == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetDnsId returns the value of the field, resolving if necessary
func (ev *Event) GetDnsId() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.DNS.ID
}

// GetDnsQuestionClass returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionClass() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.DNS.Class
}

// GetDnsQuestionCount returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionCount() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.DNS.Count
}

// GetDnsQuestionLength returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionLength() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.DNS.Size
}

// GetDnsQuestionName returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.DNS.Name
}

// GetDnsQuestionNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return len(ev.DNS.Name)
}

// GetDnsQuestionType returns the value of the field, resolving if necessary
func (ev *Event) GetDnsQuestionType() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.DNS.Type
}

// GetEventAsync returns the value of the field, resolving if necessary
func (ev *Event) GetEventAsync() bool {
	return ev.FieldHandlers.ResolveAsync(ev)
}

// GetEventTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetEventTimestamp() int {
	return ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)
}

// GetExecArgs returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process)
}

// GetExecArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsFlags() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsOptions() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process)
}

// GetExecArgv returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.Exec.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetExecArgv0() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process)
}

// GetExecCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetExecCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.CapEffective
}

// GetExecCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetExecCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.CapPermitted
}

// GetExecComm returns the value of the field, resolving if necessary
func (ev *Event) GetExecComm() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Comm
}

// GetExecContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExecContainerId() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.ContainerID
}

// GetExecCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetExecCreatedAt() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)
}

// GetExecEgid returns the value of the field, resolving if necessary
func (ev *Event) GetExecEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.EGID
}

// GetExecEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.EGroup
}

// GetExecEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process)
}

// GetExecEuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.EUID
}

// GetExecEuser returns the value of the field, resolving if necessary
func (ev *Event) GetExecEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.EUser
}

// GetExecExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecExecTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.ExecTime
}

// GetExecExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecExitTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.ExitTime
}

// GetExecFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.CTime
}

// GetExecFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.GID
}

// GetExecFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.PathKey.Inode
}

// GetExecFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.Mode
}

// GetExecFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.MTime
}

// GetExecFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.PathKey.MountID
}

// GetExecFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
}

// GetExecFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
}

// GetExecFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
}

// GetExecFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exec.Process.FileEvent.FileFields.UID
}

// GetExecFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields)
}

// GetExecForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecForkTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.ForkTime
}

// GetExecFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.FSGID
}

// GetExecFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.FSGroup
}

// GetExecFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.FSUID
}

// GetExecFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetExecFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.FSUser
}

// GetExecGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.GID
}

// GetExecGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.Group
}

// GetExecInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetExecInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetExecInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExecInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetExecInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetExecInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetExecInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetExecInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
}

// GetExecInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
}

// GetExecInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
}

// GetExecInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetExecInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecInterpreterFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	if !ev.Exec.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExecIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetExecIsKworker() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.PIDContext.IsKworker
}

// GetExecIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetExecIsThread() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.IsThread
}

// GetExecPid returns the value of the field, resolving if necessary
func (ev *Event) GetExecPid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.PIDContext.Pid
}

// GetExecPpid returns the value of the field, resolving if necessary
func (ev *Event) GetExecPpid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.PPid
}

// GetExecTid returns the value of the field, resolving if necessary
func (ev *Event) GetExecTid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.PIDContext.Tid
}

// GetExecTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetExecTtyName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.TTYName
}

// GetExecUid returns the value of the field, resolving if necessary
func (ev *Event) GetExecUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.UID
}

// GetExecUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.Exec.Process.Credentials.User
}

// GetExitArgs returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
}

// GetExitArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsFlags() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsOptions() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
}

// GetExitArgv returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.Exit.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetExitArgv0() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
}

// GetExitCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetExitCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.CapEffective
}

// GetExitCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetExitCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.CapPermitted
}

// GetExitCause returns the value of the field, resolving if necessary
func (ev *Event) GetExitCause() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	return ev.Exit.Cause
}

// GetExitCode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCode() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	return ev.Exit.Code
}

// GetExitComm returns the value of the field, resolving if necessary
func (ev *Event) GetExitComm() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Comm
}

// GetExitContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExitContainerId() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.ContainerID
}

// GetExitCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetExitCreatedAt() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
}

// GetExitEgid returns the value of the field, resolving if necessary
func (ev *Event) GetExitEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.EGID
}

// GetExitEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.EGroup
}

// GetExitEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
}

// GetExitEuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.EUID
}

// GetExitEuser returns the value of the field, resolving if necessary
func (ev *Event) GetExitEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.EUser
}

// GetExitExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitExecTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.ExecTime
}

// GetExitExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitExitTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.ExitTime
}

// GetExitFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.CTime
}

// GetExitFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.GID
}

// GetExitFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.PathKey.Inode
}

// GetExitFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.Mode
}

// GetExitFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.MTime
}

// GetExitFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.PathKey.MountID
}

// GetExitFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
}

// GetExitFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
}

// GetExitFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
}

// GetExitFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Exit.Process.FileEvent.FileFields.UID
}

// GetExitFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields)
}

// GetExitForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitForkTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.ForkTime
}

// GetExitFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.FSGID
}

// GetExitFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.FSGroup
}

// GetExitFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.FSUID
}

// GetExitFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetExitFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.FSUser
}

// GetExitGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.GID
}

// GetExitGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.Group
}

// GetExitInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetExitInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetExitInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetExitInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetExitInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetExitInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetExitInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetExitInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
}

// GetExitInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
}

// GetExitInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
}

// GetExitInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetExitInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitInterpreterFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	if !ev.Exit.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetExitIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetExitIsKworker() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.PIDContext.IsKworker
}

// GetExitIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetExitIsThread() bool {
	zeroValue := false
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.IsThread
}

// GetExitPid returns the value of the field, resolving if necessary
func (ev *Event) GetExitPid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.PIDContext.Pid
}

// GetExitPpid returns the value of the field, resolving if necessary
func (ev *Event) GetExitPpid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.PPid
}

// GetExitTid returns the value of the field, resolving if necessary
func (ev *Event) GetExitTid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.PIDContext.Tid
}

// GetExitTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetExitTtyName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.TTYName
}

// GetExitUid returns the value of the field, resolving if necessary
func (ev *Event) GetExitUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.UID
}

// GetExitUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.Exit.Process.Credentials.User
}

// GetLinkFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.CTime
}

// GetLinkFileDestinationChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.CTime
}

// GetLinkFileDestinationFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target)
}

// GetLinkFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.GID
}

// GetLinkFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetLinkFileDestinationInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationInode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.PathKey.Inode
}

// GetLinkFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.Mode
}

// GetLinkFileDestinationModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.MTime
}

// GetLinkFileDestinationMountId returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.PathKey.MountID
}

// GetLinkFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target)
}

// GetLinkFileDestinationNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target))
}

// GetLinkFileDestinationPackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target)
}

// GetLinkFileDestinationPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationPathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target))
}

// GetLinkFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Target.FileFields.UID
}

// GetLinkFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileDestinationUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields)
}

// GetLinkFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source)
}

// GetLinkFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.GID
}

// GetLinkFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetLinkFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.PathKey.Inode
}

// GetLinkFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.Mode
}

// GetLinkFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.MTime
}

// GetLinkFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.PathKey.MountID
}

// GetLinkFileName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source)
}

// GetLinkFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source))
}

// GetLinkFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source)
}

// GetLinkFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source)
}

// GetLinkFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source)
}

// GetLinkFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source)
}

// GetLinkFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source))
}

// GetLinkFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Link.Source.FileFields)
}

// GetLinkFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.Source.FileFields.UID
}

// GetLinkFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetLinkFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields)
}

// GetLinkRetval returns the value of the field, resolving if necessary
func (ev *Event) GetLinkRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "link" {
		return zeroValue
	}
	return ev.Link.SyscallEvent.Retval
}

// GetLoadModuleArgs returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule)
}

// GetLoadModuleArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.ArgsTruncated
}

// GetLoadModuleArgv returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetLoadModuleFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.CTime
}

// GetLoadModuleFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File)
}

// GetLoadModuleFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.GID
}

// GetLoadModuleFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetLoadModuleFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.PathKey.Inode
}

// GetLoadModuleFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.Mode
}

// GetLoadModuleFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.MTime
}

// GetLoadModuleFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.PathKey.MountID
}

// GetLoadModuleFileName returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File)
}

// GetLoadModuleFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File))
}

// GetLoadModuleFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File)
}

// GetLoadModuleFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File))
}

// GetLoadModuleFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.File.FileFields.UID
}

// GetLoadModuleFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields)
}

// GetLoadModuleLoadedFromMemory returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleLoadedFromMemory() bool {
	zeroValue := false
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.LoadedFromMemory
}

// GetLoadModuleName returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.Name
}

// GetLoadModuleRetval returns the value of the field, resolving if necessary
func (ev *Event) GetLoadModuleRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "load_module" {
		return zeroValue
	}
	return ev.LoadModule.SyscallEvent.Retval
}

// GetMkdirFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.CTime
}

// GetMkdirFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileDestinationMode() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.Mode
}

// GetMkdirFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileDestinationRights() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.Mode
}

// GetMkdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File)
}

// GetMkdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.GID
}

// GetMkdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetMkdirFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.PathKey.Inode
}

// GetMkdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.Mode
}

// GetMkdirFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.MTime
}

// GetMkdirFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.PathKey.MountID
}

// GetMkdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File)
}

// GetMkdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File))
}

// GetMkdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File)
}

// GetMkdirFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File)
}

// GetMkdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File)
}

// GetMkdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File)
}

// GetMkdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File))
}

// GetMkdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.File.FileFields.UID
}

// GetMkdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields)
}

// GetMkdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMkdirRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "mkdir" {
		return zeroValue
	}
	return ev.Mkdir.SyscallEvent.Retval
}

// GetMmapFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.CTime
}

// GetMmapFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File)
}

// GetMmapFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.GID
}

// GetMmapFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetMmapFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.PathKey.Inode
}

// GetMmapFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.Mode
}

// GetMmapFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.MTime
}

// GetMmapFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.PathKey.MountID
}

// GetMmapFileName returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File)
}

// GetMmapFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File))
}

// GetMmapFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File)
}

// GetMmapFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File)
}

// GetMmapFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File)
}

// GetMmapFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File)
}

// GetMmapFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File))
}

// GetMmapFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.MMap.File.FileFields)
}

// GetMmapFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.File.FileFields.UID
}

// GetMmapFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields)
}

// GetMmapFlags returns the value of the field, resolving if necessary
func (ev *Event) GetMmapFlags() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.Flags
}

// GetMmapProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMmapProtection() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.Protection
}

// GetMmapRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMmapRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "mmap" {
		return zeroValue
	}
	return ev.MMap.SyscallEvent.Retval
}

// GetMountFsType returns the value of the field, resolving if necessary
func (ev *Event) GetMountFsType() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mount" {
		return zeroValue
	}
	return ev.Mount.Mount.FSType
}

// GetMountMountpointPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountMountpointPath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mount" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount)
}

// GetMountRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMountRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "mount" {
		return zeroValue
	}
	return ev.Mount.SyscallEvent.Retval
}

// GetMountSourcePath returns the value of the field, resolving if necessary
func (ev *Event) GetMountSourcePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "mount" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount)
}

// GetMprotectReqProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectReqProtection() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mprotect" {
		return zeroValue
	}
	return ev.MProtect.ReqProtection
}

// GetMprotectRetval returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "mprotect" {
		return zeroValue
	}
	return ev.MProtect.SyscallEvent.Retval
}

// GetMprotectVmProtection returns the value of the field, resolving if necessary
func (ev *Event) GetMprotectVmProtection() int {
	zeroValue := 0
	if ev.GetEventType().String() != "mprotect" {
		return zeroValue
	}
	return ev.MProtect.VMProtection
}

// GetNetworkDestinationIp returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDestinationIp() net.IPNet {
	zeroValue := net.IPNet{}
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.Destination.IPNet
}

// GetNetworkDestinationPort returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDestinationPort() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.Destination.Port
}

// GetNetworkDeviceIfindex returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDeviceIfindex() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.Device.IfIndex
}

// GetNetworkDeviceIfname returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkDeviceIfname() string {
	zeroValue := ""
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.BaseEvent.NetworkContext.Device)
}

// GetNetworkL3Protocol returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkL3Protocol() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.L3Protocol
}

// GetNetworkL4Protocol returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkL4Protocol() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.L4Protocol
}

// GetNetworkSize returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSize() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.Size
}

// GetNetworkSourceIp returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSourceIp() net.IPNet {
	zeroValue := net.IPNet{}
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.Source.IPNet
}

// GetNetworkSourcePort returns the value of the field, resolving if necessary
func (ev *Event) GetNetworkSourcePort() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "dns" {
		return zeroValue
	}
	return ev.BaseEvent.NetworkContext.Source.Port
}

// GetOpenFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.CTime
}

// GetOpenFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileDestinationMode() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.Mode
}

// GetOpenFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File)
}

// GetOpenFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.GID
}

// GetOpenFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields)
}

// GetOpenFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetOpenFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Open.File.FileFields)
}

// GetOpenFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.PathKey.Inode
}

// GetOpenFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.Mode
}

// GetOpenFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.MTime
}

// GetOpenFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.PathKey.MountID
}

// GetOpenFileName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File)
}

// GetOpenFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File))
}

// GetOpenFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File)
}

// GetOpenFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File)
}

// GetOpenFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File)
}

// GetOpenFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File)
}

// GetOpenFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File))
}

// GetOpenFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Open.File.FileFields)
}

// GetOpenFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.File.FileFields.UID
}

// GetOpenFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields)
}

// GetOpenFlags returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFlags() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.Flags
}

// GetOpenRetval returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.SyscallEvent.Retval
}

// GetProcessAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgs() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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

// GetProcessAncestorsArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgsTruncated() []bool {
	zeroValue := []bool{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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

// GetProcessAncestorsArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsArgv0() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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

// GetProcessAncestorsCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCapEffective() []uint64 {
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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

// GetProcessAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsComm() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
	}
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

// GetProcessAncestorsCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCreatedAt() []int {
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
func (ev *Event) GetProcessAncestorsEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
		result = filterEnvs(result, desiredKeys)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
		result = filterEnvs(result, desiredKeys)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvsTruncated() []bool {
	zeroValue := []bool{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint16{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint16{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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

// GetProcessAncestorsIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsIsKworker() []bool {
	zeroValue := []bool{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
	}
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
func (ev *Event) GetProcessAncestorsPid() []uint32 {
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Ancestor == nil {
		return zeroValue
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

// GetProcessArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgs() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsFlags() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.BaseEvent.ProcessContext.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsOptions() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.BaseEvent.ProcessContext.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgsTruncated() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgv() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &ev.BaseEvent.ProcessContext.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessArgv0() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.CapEffective
}

// GetProcessCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.CapPermitted
}

// GetProcessComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessComm() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Comm
}

// GetProcessContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessContainerId() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.ContainerID
}

// GetProcessCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCreatedAt() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEgid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EGID
}

// GetProcessEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEgroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EGroup
}

// GetProcessEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvsTruncated() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEuid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EUID
}

// GetProcessEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEuser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.EUser
}

// GetProcessExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessExecTime() time.Time {
	zeroValue := time.Time{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.ExecTime
}

// GetProcessExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessExitTime() time.Time {
	zeroValue := time.Time{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.ExitTime
}

// GetProcessFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.CTime
}

// GetProcessFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileFilesystem() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.GID
}

// GetProcessFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileGroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileHashes() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileInUpperLayer() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode
}

// GetProcessFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.Mode
}

// GetProcessFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.MTime
}

// GetProcessFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID
}

// GetProcessFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileNameLength() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
}

// GetProcessFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePackageVersion() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePath() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePathLength() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
}

// GetProcessFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileRights() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.UID
}

// GetProcessFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileUser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
}

// GetProcessForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessForkTime() time.Time {
	zeroValue := time.Time{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.ForkTime
}

// GetProcessFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsgid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSGID
}

// GetProcessFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsgroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSGroup
}

// GetProcessFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSUID
}

// GetProcessFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFsuser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.FSUser
}

// GetProcessGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessGid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.GID
}

// GetProcessGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessGroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.Group
}

// GetProcessInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetProcessInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileFilesystem() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetProcessInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileGroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileHashes() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetProcessInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetProcessInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetProcessInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetProcessInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileNameLength() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
}

// GetProcessInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePackageVersion() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePath() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
}

// GetProcessInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFilePathLength() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
}

// GetProcessInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileRights() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetProcessInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessInterpreterFileUser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIsKworker() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.PIDContext.IsKworker
}

// GetProcessIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessIsThread() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.IsThread
}

// GetProcessParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgs() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsFlags() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.BaseEvent.ProcessContext.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsOptions() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.BaseEvent.ProcessContext.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgsTruncated() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgv() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.BaseEvent.ProcessContext.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentArgv0() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.CapEffective
}

// GetProcessParentCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint64(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.CapPermitted
}

// GetProcessParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentComm() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Comm
}

// GetProcessParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentContainerId() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.ContainerID
}

// GetProcessParentCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCreatedAt() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEgid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EGID
}

// GetProcessParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEgroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EGroup
}

// GetProcessParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvsTruncated() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEuid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EUID
}

// GetProcessParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEuser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.EUser
}

// GetProcessParentFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileInUpperLayer() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint16(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
}

// GetProcessParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFilePackageName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
}

// GetProcessParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFileRights() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSGID
}

// GetProcessParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsgroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSGroup
}

// GetProcessParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSUID
}

// GetProcessParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentFsuser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.FSUser
}

// GetProcessParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentGid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.GID
}

// GetProcessParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentGroup() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.Group
}

// GetProcessParentInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetProcessParentInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint16(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
}

// GetProcessParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
}

// GetProcessParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentInterpreterFileRights() int {
	zeroValue := 0
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetProcessParentIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIsKworker() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.IsKworker
}

// GetProcessParentIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentIsThread() bool {
	zeroValue := false
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return false
	}
	return ev.BaseEvent.ProcessContext.Parent.IsThread
}

// GetProcessParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentPid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid
}

// GetProcessParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentPpid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.PPid
}

// GetProcessParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentTid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.PIDContext.Tid
}

// GetProcessParentTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentTtyName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.TTYName
}

// GetProcessParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return uint32(0)
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.UID
}

// GetProcessParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return zeroValue
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.Credentials.User
}

// GetProcessPid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessPid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.PIDContext.Pid
}

// GetProcessPpid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessPpid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.PPid
}

// GetProcessTid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessTid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.PIDContext.Tid
}

// GetProcessTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessTtyName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.TTYName
}

// GetProcessUid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUid() uint32 {
	zeroValue := uint32(0)
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.UID
}

// GetProcessUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUser() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.Credentials.User
}

// GetPtraceRequest returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceRequest() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	return ev.PTrace.Request
}

// GetPtraceRetval returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	return ev.PTrace.SyscallEvent.Retval
}

// GetPtraceTraceeAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgs() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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

// GetPtraceTraceeAncestorsArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgsTruncated() []bool {
	zeroValue := []bool{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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

// GetPtraceTraceeAncestorsArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsArgv0() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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

// GetPtraceTraceeAncestorsCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCapEffective() []uint64 {
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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

// GetPtraceTraceeAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsComm() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
	}
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

// GetPtraceTraceeAncestorsCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsCreatedAt() []int {
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
func (ev *Event) GetPtraceTraceeAncestorsEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
		result = filterEnvs(result, desiredKeys)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
		result = filterEnvs(result, desiredKeys)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetPtraceTraceeAncestorsEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsEnvsTruncated() []bool {
	zeroValue := []bool{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint16{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint16{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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

// GetPtraceTraceeAncestorsIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeAncestorsIsKworker() []bool {
	zeroValue := []bool{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
	}
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
func (ev *Event) GetPtraceTraceeAncestorsPid() []uint32 {
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Ancestor == nil {
		return zeroValue
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

// GetPtraceTraceeArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsFlags() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.PTrace.Tracee.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsOptions() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.PTrace.Tracee.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &ev.PTrace.Tracee.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeArgv0() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.CapEffective
}

// GetPtraceTraceeCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.CapPermitted
}

// GetPtraceTraceeComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeComm() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Comm
}

// GetPtraceTraceeContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeContainerId() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.ContainerID
}

// GetPtraceTraceeCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeCreatedAt() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.EGID
}

// GetPtraceTraceeEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.EGroup
}

// GetPtraceTraceeEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEnvsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process)
}

// GetPtraceTraceeEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.EUID
}

// GetPtraceTraceeEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.EUser
}

// GetPtraceTraceeExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeExecTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.ExecTime
}

// GetPtraceTraceeExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeExitTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.ExitTime
}

// GetPtraceTraceeFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.CTime
}

// GetPtraceTraceeFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.GID
}

// GetPtraceTraceeFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.Inode
}

// GetPtraceTraceeFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.Mode
}

// GetPtraceTraceeFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.MTime
}

// GetPtraceTraceeFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.MountID
}

// GetPtraceTraceeFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent))
}

// GetPtraceTraceeFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent)
}

// GetPtraceTraceeFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent))
}

// GetPtraceTraceeFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.FileEvent.FileFields.UID
}

// GetPtraceTraceeFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
}

// GetPtraceTraceeForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeForkTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.ForkTime
}

// GetPtraceTraceeFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.FSGID
}

// GetPtraceTraceeFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.FSGroup
}

// GetPtraceTraceeFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.FSUID
}

// GetPtraceTraceeFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.FSUser
}

// GetPtraceTraceeGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.GID
}

// GetPtraceTraceeGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.Group
}

// GetPtraceTraceeInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetPtraceTraceeInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetPtraceTraceeInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetPtraceTraceeInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetPtraceTraceeInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetPtraceTraceeInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetPtraceTraceeInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
}

// GetPtraceTraceeInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetPtraceTraceeInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeInterpreterFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIsKworker() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.PIDContext.IsKworker
}

// GetPtraceTraceeIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeIsThread() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.IsThread
}

// GetPtraceTraceeParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsFlags() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.PTrace.Tracee.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsOptions() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.PTrace.Tracee.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.PTrace.Tracee.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentArgv0() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.CapEffective
}

// GetPtraceTraceeParentCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint64(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.CapPermitted
}

// GetPtraceTraceeParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentComm() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Comm
}

// GetPtraceTraceeParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentContainerId() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.ContainerID
}

// GetPtraceTraceeParentCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentCreatedAt() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.EGID
}

// GetPtraceTraceeParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.EGroup
}

// GetPtraceTraceeParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEnvsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.PTrace.Tracee.Parent)
}

// GetPtraceTraceeParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.EUID
}

// GetPtraceTraceeParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.EUser
}

// GetPtraceTraceeParentFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	if !ev.PTrace.Tracee.Parent.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent))
}

// GetPtraceTraceeParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent))
}

// GetPtraceTraceeParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSGID
}

// GetPtraceTraceeParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSGroup
}

// GetPtraceTraceeParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSUID
}

// GetPtraceTraceeParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.FSUser
}

// GetPtraceTraceeParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.GID
}

// GetPtraceTraceeParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.Group
}

// GetPtraceTraceeParentInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return []string{}
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetPtraceTraceeParentInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
}

// GetPtraceTraceeParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentInterpreterFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	if !ev.PTrace.Tracee.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetPtraceTraceeParentIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIsKworker() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.PTrace.Tracee.Parent.PIDContext.IsKworker
}

// GetPtraceTraceeParentIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentIsThread() bool {
	zeroValue := false
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return false
	}
	return ev.PTrace.Tracee.Parent.IsThread
}

// GetPtraceTraceeParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentPid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.PIDContext.Pid
}

// GetPtraceTraceeParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentPpid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.PPid
}

// GetPtraceTraceeParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentTid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.PIDContext.Tid
}

// GetPtraceTraceeParentTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentTtyName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.TTYName
}

// GetPtraceTraceeParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return uint32(0)
	}
	return ev.PTrace.Tracee.Parent.Credentials.UID
}

// GetPtraceTraceeParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeParentUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	if ev.PTrace.Tracee.Parent == nil {
		return zeroValue
	}
	if !ev.PTrace.Tracee.HasParent() {
		return ""
	}
	return ev.PTrace.Tracee.Parent.Credentials.User
}

// GetPtraceTraceePid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceePid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.PIDContext.Pid
}

// GetPtraceTraceePpid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceePpid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.PPid
}

// GetPtraceTraceeTid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeTid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.PIDContext.Tid
}

// GetPtraceTraceeTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeTtyName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.TTYName
}

// GetPtraceTraceeUid returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.UID
}

// GetPtraceTraceeUser returns the value of the field, resolving if necessary
func (ev *Event) GetPtraceTraceeUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "ptrace" {
		return zeroValue
	}
	if ev.PTrace.Tracee == nil {
		return zeroValue
	}
	return ev.PTrace.Tracee.Process.Credentials.User
}

// GetRemovexattrFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.CTime
}

// GetRemovexattrFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileDestinationName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr)
}

// GetRemovexattrFileDestinationNamespace returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileDestinationNamespace() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr)
}

// GetRemovexattrFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.GID
}

// GetRemovexattrFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetRemovexattrFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.PathKey.Inode
}

// GetRemovexattrFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.Mode
}

// GetRemovexattrFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.MTime
}

// GetRemovexattrFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.PathKey.MountID
}

// GetRemovexattrFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File))
}

// GetRemovexattrFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File)
}

// GetRemovexattrFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File))
}

// GetRemovexattrFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.File.FileFields.UID
}

// GetRemovexattrFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields)
}

// GetRemovexattrRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRemovexattrRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "removexattr" {
		return zeroValue
	}
	return ev.RemoveXAttr.SyscallEvent.Retval
}

// GetRenameFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.CTime
}

// GetRenameFileDestinationChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.CTime
}

// GetRenameFileDestinationFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New)
}

// GetRenameFileDestinationGid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.GID
}

// GetRenameFileDestinationGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetRenameFileDestinationInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationInode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.PathKey.Inode
}

// GetRenameFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.Mode
}

// GetRenameFileDestinationModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.MTime
}

// GetRenameFileDestinationMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.PathKey.MountID
}

// GetRenameFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New)
}

// GetRenameFileDestinationNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New))
}

// GetRenameFileDestinationPackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New)
}

// GetRenameFileDestinationPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationPathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New))
}

// GetRenameFileDestinationRights returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileDestinationUid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.New.FileFields.UID
}

// GetRenameFileDestinationUser returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileDestinationUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields)
}

// GetRenameFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old)
}

// GetRenameFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.GID
}

// GetRenameFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetRenameFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.PathKey.Inode
}

// GetRenameFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.Mode
}

// GetRenameFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.MTime
}

// GetRenameFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.PathKey.MountID
}

// GetRenameFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old)
}

// GetRenameFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old))
}

// GetRenameFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old)
}

// GetRenameFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old)
}

// GetRenameFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old)
}

// GetRenameFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old)
}

// GetRenameFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old))
}

// GetRenameFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Rename.Old.FileFields)
}

// GetRenameFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.Old.FileFields.UID
}

// GetRenameFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRenameFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields)
}

// GetRenameRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRenameRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "rename" {
		return zeroValue
	}
	return ev.Rename.SyscallEvent.Retval
}

// GetRmdirFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.CTime
}

// GetRmdirFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File)
}

// GetRmdirFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.GID
}

// GetRmdirFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetRmdirFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.PathKey.Inode
}

// GetRmdirFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.Mode
}

// GetRmdirFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.MTime
}

// GetRmdirFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.PathKey.MountID
}

// GetRmdirFileName returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File)
}

// GetRmdirFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File))
}

// GetRmdirFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File)
}

// GetRmdirFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File)
}

// GetRmdirFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File)
}

// GetRmdirFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File)
}

// GetRmdirFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File))
}

// GetRmdirFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.File.FileFields.UID
}

// GetRmdirFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields)
}

// GetRmdirRetval returns the value of the field, resolving if necessary
func (ev *Event) GetRmdirRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "rmdir" {
		return zeroValue
	}
	return ev.Rmdir.SyscallEvent.Retval
}

// GetSelinuxBoolName returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "selinux" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux)
}

// GetSelinuxBoolState returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolState() string {
	zeroValue := ""
	if ev.GetEventType().String() != "selinux" {
		return zeroValue
	}
	return ev.SELinux.BoolChangeValue
}

// GetSelinuxBoolCommitState returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxBoolCommitState() bool {
	zeroValue := false
	if ev.GetEventType().String() != "selinux" {
		return zeroValue
	}
	return ev.SELinux.BoolCommitValue
}

// GetSelinuxEnforceStatus returns the value of the field, resolving if necessary
func (ev *Event) GetSelinuxEnforceStatus() string {
	zeroValue := ""
	if ev.GetEventType().String() != "selinux" {
		return zeroValue
	}
	return ev.SELinux.EnforceStatus
}

// GetSetgidEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setgid" {
		return zeroValue
	}
	return ev.SetGID.EGID
}

// GetSetgidEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setgid" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID)
}

// GetSetgidFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidFsgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setgid" {
		return zeroValue
	}
	return ev.SetGID.FSGID
}

// GetSetgidFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setgid" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID)
}

// GetSetgidGid returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setgid" {
		return zeroValue
	}
	return ev.SetGID.GID
}

// GetSetgidGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetgidGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setgid" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID)
}

// GetSetuidEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setuid" {
		return zeroValue
	}
	return ev.SetUID.EUID
}

// GetSetuidEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setuid" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID)
}

// GetSetuidFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setuid" {
		return zeroValue
	}
	return ev.SetUID.FSUID
}

// GetSetuidFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setuid" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID)
}

// GetSetuidUid returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setuid" {
		return zeroValue
	}
	return ev.SetUID.UID
}

// GetSetuidUser returns the value of the field, resolving if necessary
func (ev *Event) GetSetuidUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setuid" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID)
}

// GetSetxattrFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.CTime
}

// GetSetxattrFileDestinationName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileDestinationName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr)
}

// GetSetxattrFileDestinationNamespace returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileDestinationNamespace() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr)
}

// GetSetxattrFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File)
}

// GetSetxattrFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.GID
}

// GetSetxattrFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSetxattrFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.PathKey.Inode
}

// GetSetxattrFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.Mode
}

// GetSetxattrFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.MTime
}

// GetSetxattrFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.PathKey.MountID
}

// GetSetxattrFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File)
}

// GetSetxattrFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File))
}

// GetSetxattrFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File)
}

// GetSetxattrFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File))
}

// GetSetxattrFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.File.FileFields.UID
}

// GetSetxattrFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields)
}

// GetSetxattrRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSetxattrRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "setxattr" {
		return zeroValue
	}
	return ev.SetXAttr.SyscallEvent.Retval
}

// GetSignalPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalPid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	return ev.Signal.PID
}

// GetSignalRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSignalRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	return ev.Signal.SyscallEvent.Retval
}

// GetSignalTargetAncestorsArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgs() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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

// GetSignalTargetAncestorsArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgsTruncated() []bool {
	zeroValue := []bool{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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

// GetSignalTargetAncestorsArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsArgv0() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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

// GetSignalTargetAncestorsCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCapEffective() []uint64 {
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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

// GetSignalTargetAncestorsComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsComm() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
	}
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

// GetSignalTargetAncestorsCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsCreatedAt() []int {
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
func (ev *Event) GetSignalTargetAncestorsEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
		result = filterEnvs(result, desiredKeys)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
	}
	var values []string
	ctx := eval.NewContext(ev)
	iterator := &ProcessAncestorsIterator{}
	ptr := iterator.Front(ctx)
	for ptr != nil {
		element := (*ProcessCacheEntry)(ptr)
		result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
		result = filterEnvs(result, desiredKeys)
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetSignalTargetAncestorsEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsEnvsTruncated() []bool {
	zeroValue := []bool{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint16{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint16{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint64{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []int{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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

// GetSignalTargetAncestorsIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetAncestorsIsKworker() []bool {
	zeroValue := []bool{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []bool{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
	}
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
func (ev *Event) GetSignalTargetAncestorsPid() []uint32 {
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []uint32{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Ancestor == nil {
		return zeroValue
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

// GetSignalTargetArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsFlags() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Signal.Target.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsOptions() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Signal.Target.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, &ev.Signal.Target.Process)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetArgv0() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.CapEffective
}

// GetSignalTargetCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.CapPermitted
}

// GetSignalTargetComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetComm() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Comm
}

// GetSignalTargetContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetContainerId() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.ContainerID
}

// GetSignalTargetCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetCreatedAt() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.EGID
}

// GetSignalTargetEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.EGroup
}

// GetSignalTargetEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEnvsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process)
}

// GetSignalTargetEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.EUID
}

// GetSignalTargetEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.EUser
}

// GetSignalTargetExecTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetExecTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.ExecTime
}

// GetSignalTargetExitTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetExitTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.ExitTime
}

// GetSignalTargetFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.CTime
}

// GetSignalTargetFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.GID
}

// GetSignalTargetFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.PathKey.Inode
}

// GetSignalTargetFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint16(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.Mode
}

// GetSignalTargetFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.MTime
}

// GetSignalTargetFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.PathKey.MountID
}

// GetSignalTargetFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent))
}

// GetSignalTargetFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent)
}

// GetSignalTargetFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent))
}

// GetSignalTargetFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.FileEvent.FileFields.UID
}

// GetSignalTargetFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.IsNotKworker() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
}

// GetSignalTargetForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetForkTime() time.Time {
	zeroValue := time.Time{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.ForkTime
}

// GetSignalTargetFsgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.FSGID
}

// GetSignalTargetFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.FSGroup
}

// GetSignalTargetFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.FSUID
}

// GetSignalTargetFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.FSUser
}

// GetSignalTargetGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.GID
}

// GetSignalTargetGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.Group
}

// GetSignalTargetInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime
}

// GetSignalTargetInterpreterFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID
}

// GetSignalTargetInterpreterFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return false
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode
}

// GetSignalTargetInterpreterFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint16(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode
}

// GetSignalTargetInterpreterFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint64(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime
}

// GetSignalTargetInterpreterFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID
}

// GetSignalTargetInterpreterFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
}

// GetSignalTargetInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
}

// GetSignalTargetInterpreterFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
}

// GetSignalTargetInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return 0
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetInterpreterFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return uint32(0)
	}
	return ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID
}

// GetSignalTargetInterpreterFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetInterpreterFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if !ev.Signal.Target.Process.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIsKworker() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.PIDContext.IsKworker
}

// GetSignalTargetIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetIsThread() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.IsThread
}

// GetSignalTargetParentArgs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgs() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgsFlags returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsFlags() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Signal.Target.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentArgsOptions returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsOptions() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Signal.Target.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentArgsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentArgv returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgv() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessArgvScrubbed(ev, ev.Signal.Target.Parent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentArgv0 returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentArgv0() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentCapEffective returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCapEffective() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.Credentials.CapEffective
}

// GetSignalTargetParentCapPermitted returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCapPermitted() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint64(0)
	}
	return ev.Signal.Target.Parent.Credentials.CapPermitted
}

// GetSignalTargetParentComm returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentComm() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Comm
}

// GetSignalTargetParentContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentContainerId() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.ContainerID
}

// GetSignalTargetParentCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentCreatedAt() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEgid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEgid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.EGID
}

// GetSignalTargetParentEgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.EGroup
}

// GetSignalTargetParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvp(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvs(desiredKeys map[string]bool) []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent)
	resolvedField = filterEnvs(resolvedField, desiredKeys)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentEnvsTruncated returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEnvsTruncated() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Signal.Target.Parent)
}

// GetSignalTargetParentEuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.EUID
}

// GetSignalTargetParentEuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentEuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.EUser
}

// GetSignalTargetParentFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	if !ev.Signal.Target.Parent.IsNotKworker() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent))
}

// GetSignalTargetParentFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent))
}

// GetSignalTargetParentFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.FSGID
}

// GetSignalTargetParentFsgroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsgroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.FSGroup
}

// GetSignalTargetParentFsuid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsuid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.FSUID
}

// GetSignalTargetParentFsuser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentFsuser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.FSUser
}

// GetSignalTargetParentGid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.GID
}

// GetSignalTargetParentGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.Group
}

// GetSignalTargetParentInterpreterFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := []string{}
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return []string{}
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return []string{}
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSignalTargetParentInterpreterFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
}

// GetSignalTargetParentInterpreterFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
}

// GetSignalTargetParentInterpreterFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentInterpreterFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
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
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	if !ev.Signal.Target.Parent.HasInterpreter() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
}

// GetSignalTargetParentIsKworker returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIsKworker() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.Signal.Target.Parent.PIDContext.IsKworker
}

// GetSignalTargetParentIsThread returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentIsThread() bool {
	zeroValue := false
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return false
	}
	return ev.Signal.Target.Parent.IsThread
}

// GetSignalTargetParentPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentPid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.PIDContext.Pid
}

// GetSignalTargetParentPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentPpid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.PPid
}

// GetSignalTargetParentTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentTid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.PIDContext.Tid
}

// GetSignalTargetParentTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentTtyName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.TTYName
}

// GetSignalTargetParentUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return uint32(0)
	}
	return ev.Signal.Target.Parent.Credentials.UID
}

// GetSignalTargetParentUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetParentUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	if ev.Signal.Target.Parent == nil {
		return zeroValue
	}
	if !ev.Signal.Target.HasParent() {
		return ""
	}
	return ev.Signal.Target.Parent.Credentials.User
}

// GetSignalTargetPid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetPid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.PIDContext.Pid
}

// GetSignalTargetPpid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetPpid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.PPid
}

// GetSignalTargetTid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetTid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.PIDContext.Tid
}

// GetSignalTargetTtyName returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetTtyName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.TTYName
}

// GetSignalTargetUid returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.UID
}

// GetSignalTargetUser returns the value of the field, resolving if necessary
func (ev *Event) GetSignalTargetUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	if ev.Signal.Target == nil {
		return zeroValue
	}
	return ev.Signal.Target.Process.Credentials.User
}

// GetSignalType returns the value of the field, resolving if necessary
func (ev *Event) GetSignalType() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "signal" {
		return zeroValue
	}
	return ev.Signal.Type
}

// GetSpliceFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.CTime
}

// GetSpliceFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File)
}

// GetSpliceFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.GID
}

// GetSpliceFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetSpliceFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.PathKey.Inode
}

// GetSpliceFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.Mode
}

// GetSpliceFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.MTime
}

// GetSpliceFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.PathKey.MountID
}

// GetSpliceFileName returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File)
}

// GetSpliceFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File))
}

// GetSpliceFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File)
}

// GetSpliceFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File)
}

// GetSpliceFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File)
}

// GetSpliceFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File)
}

// GetSpliceFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File))
}

// GetSpliceFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Splice.File.FileFields)
}

// GetSpliceFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.File.FileFields.UID
}

// GetSpliceFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields)
}

// GetSplicePipeEntryFlag returns the value of the field, resolving if necessary
func (ev *Event) GetSplicePipeEntryFlag() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.PipeEntryFlag
}

// GetSplicePipeExitFlag returns the value of the field, resolving if necessary
func (ev *Event) GetSplicePipeExitFlag() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.PipeExitFlag
}

// GetSpliceRetval returns the value of the field, resolving if necessary
func (ev *Event) GetSpliceRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "splice" {
		return zeroValue
	}
	return ev.Splice.SyscallEvent.Retval
}

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev)
}

// GetUnlinkFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.CTime
}

// GetUnlinkFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File)
}

// GetUnlinkFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.GID
}

// GetUnlinkFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetUnlinkFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.PathKey.Inode
}

// GetUnlinkFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.Mode
}

// GetUnlinkFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.MTime
}

// GetUnlinkFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.PathKey.MountID
}

// GetUnlinkFileName returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File)
}

// GetUnlinkFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File))
}

// GetUnlinkFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File)
}

// GetUnlinkFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File)
}

// GetUnlinkFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File)
}

// GetUnlinkFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File)
}

// GetUnlinkFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File))
}

// GetUnlinkFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.File.FileFields.UID
}

// GetUnlinkFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields)
}

// GetUnlinkFlags returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkFlags() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.Flags
}

// GetUnlinkRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUnlinkRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "unlink" {
		return zeroValue
	}
	return ev.Unlink.SyscallEvent.Retval
}

// GetUnloadModuleName returns the value of the field, resolving if necessary
func (ev *Event) GetUnloadModuleName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "unload_module" {
		return zeroValue
	}
	return ev.UnloadModule.Name
}

// GetUnloadModuleRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUnloadModuleRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "unload_module" {
		return zeroValue
	}
	return ev.UnloadModule.SyscallEvent.Retval
}

// GetUtimesFileChangeTime returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileChangeTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.CTime
}

// GetUtimesFileFilesystem returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileFilesystem() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File)
}

// GetUtimesFileGid returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileGid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.GID
}

// GetUtimesFileGroup returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileGroup() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileHashes returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileHashes() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	resolvedField := ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
}

// GetUtimesFileInUpperLayer returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileInUpperLayer() bool {
	zeroValue := false
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileInode returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileInode() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.PathKey.Inode
}

// GetUtimesFileMode returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileMode() uint16 {
	zeroValue := uint16(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.Mode
}

// GetUtimesFileModificationTime returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileModificationTime() uint64 {
	zeroValue := uint64(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.MTime
}

// GetUtimesFileMountId returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileMountId() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.PathKey.MountID
}

// GetUtimesFileName returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File)
}

// GetUtimesFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileNameLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File))
}

// GetUtimesFilePackageName returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File)
}

// GetUtimesFilePackageSourceVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageSourceVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File)
}

// GetUtimesFilePackageVersion returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePackageVersion() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File)
}

// GetUtimesFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File)
}

// GetUtimesFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFilePathLength() int {
	zeroValue := 0
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File))
}

// GetUtimesFileRights returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileRights() int {
	zeroValue := 0
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveRights(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesFileUid returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileUid() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.File.FileFields.UID
}

// GetUtimesFileUser returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesFileUser() string {
	zeroValue := ""
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields)
}

// GetUtimesRetval returns the value of the field, resolving if necessary
func (ev *Event) GetUtimesRetval() int64 {
	zeroValue := int64(0)
	if ev.GetEventType().String() != "utimes" {
		return zeroValue
	}
	return ev.Utimes.SyscallEvent.Retval
}
