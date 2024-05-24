// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"time"
)

// GetChangePermissionNewSd returns the value of the field, resolving if necessary
func (ev *Event) GetChangePermissionNewSd() string {
	if ev.GetEventType().String() != "change_permission" {
		return ""
	}
	return ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission)
}

// GetChangePermissionOldSd returns the value of the field, resolving if necessary
func (ev *Event) GetChangePermissionOldSd() string {
	if ev.GetEventType().String() != "change_permission" {
		return ""
	}
	return ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission)
}

// GetChangePermissionPath returns the value of the field, resolving if necessary
func (ev *Event) GetChangePermissionPath() string {
	if ev.GetEventType().String() != "change_permission" {
		return ""
	}
	return ev.ChangePermission.ObjectName
}

// GetChangePermissionType returns the value of the field, resolving if necessary
func (ev *Event) GetChangePermissionType() string {
	if ev.GetEventType().String() != "change_permission" {
		return ""
	}
	return ev.ChangePermission.ObjectType
}

// GetChangePermissionUserDomain returns the value of the field, resolving if necessary
func (ev *Event) GetChangePermissionUserDomain() string {
	if ev.GetEventType().String() != "change_permission" {
		return ""
	}
	return ev.ChangePermission.UserDomain
}

// GetChangePermissionUsername returns the value of the field, resolving if necessary
func (ev *Event) GetChangePermissionUsername() string {
	if ev.GetEventType().String() != "change_permission" {
		return ""
	}
	return ev.ChangePermission.UserName
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

// GetContainerTags returns the value of the field, resolving if necessary
func (ev *Event) GetContainerTags() []string {
	if ev.BaseEvent.ContainerContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
}

// GetCreateFileName returns the value of the field, resolving if necessary
func (ev *Event) GetCreateFileName() string {
	if ev.GetEventType().String() != "create" {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.CreateNewFile.File)
}

// GetCreateFileNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetCreateFileNameLength() int {
	if ev.GetEventType().String() != "create" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.CreateNewFile.File))
}

// GetCreateFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetCreateFilePath() string {
	if ev.GetEventType().String() != "create" {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.CreateNewFile.File)
}

// GetCreateFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetCreateFilePathLength() int {
	if ev.GetEventType().String() != "create" {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.CreateNewFile.File))
}

// GetCreateRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetCreateRegistryKeyName() string {
	if ev.GetEventType().String() != "create_key" {
		return ""
	}
	return ev.CreateRegistryKey.Registry.KeyName
}

// GetCreateRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetCreateRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "create_key" {
		return 0
	}
	return len(ev.CreateRegistryKey.Registry.KeyName)
}

// GetCreateRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetCreateRegistryKeyPath() string {
	if ev.GetEventType().String() != "create_key" {
		return ""
	}
	return ev.CreateRegistryKey.Registry.KeyPath
}

// GetCreateRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetCreateRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "create_key" {
		return 0
	}
	return len(ev.CreateRegistryKey.Registry.KeyPath)
}

// GetCreateKeyRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetCreateKeyRegistryKeyName() string {
	if ev.GetEventType().String() != "create_key" {
		return ""
	}
	return ev.CreateRegistryKey.Registry.KeyName
}

// GetCreateKeyRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetCreateKeyRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "create_key" {
		return 0
	}
	return len(ev.CreateRegistryKey.Registry.KeyName)
}

// GetCreateKeyRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetCreateKeyRegistryKeyPath() string {
	if ev.GetEventType().String() != "create_key" {
		return ""
	}
	return ev.CreateRegistryKey.Registry.KeyPath
}

// GetCreateKeyRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetCreateKeyRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "create_key" {
		return 0
	}
	return len(ev.CreateRegistryKey.Registry.KeyPath)
}

// GetDeleteRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteRegistryKeyName() string {
	if ev.GetEventType().String() != "delete_key" {
		return ""
	}
	return ev.DeleteRegistryKey.Registry.KeyName
}

// GetDeleteRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "delete_key" {
		return 0
	}
	return len(ev.DeleteRegistryKey.Registry.KeyName)
}

// GetDeleteRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteRegistryKeyPath() string {
	if ev.GetEventType().String() != "delete_key" {
		return ""
	}
	return ev.DeleteRegistryKey.Registry.KeyPath
}

// GetDeleteRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "delete_key" {
		return 0
	}
	return len(ev.DeleteRegistryKey.Registry.KeyPath)
}

// GetDeleteKeyRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteKeyRegistryKeyName() string {
	if ev.GetEventType().String() != "delete_key" {
		return ""
	}
	return ev.DeleteRegistryKey.Registry.KeyName
}

// GetDeleteKeyRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteKeyRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "delete_key" {
		return 0
	}
	return len(ev.DeleteRegistryKey.Registry.KeyName)
}

// GetDeleteKeyRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteKeyRegistryKeyPath() string {
	if ev.GetEventType().String() != "delete_key" {
		return ""
	}
	return ev.DeleteRegistryKey.Registry.KeyPath
}

// GetDeleteKeyRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetDeleteKeyRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "delete_key" {
		return 0
	}
	return len(ev.DeleteRegistryKey.Registry.KeyPath)
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

// GetExecCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetExecCmdline() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
}

// GetExecCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExecCmdlineScrubbed() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, ev.Exec.Process)
}

// GetExecContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExecContainerId() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.ContainerID
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

// GetExecFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileName() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
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

// GetExecFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePath() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
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

// GetExecUser returns the value of the field, resolving if necessary
func (ev *Event) GetExecUser() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process)
}

// GetExecUserSid returns the value of the field, resolving if necessary
func (ev *Event) GetExecUserSid() string {
	if ev.GetEventType().String() != "exec" {
		return ""
	}
	if ev.Exec.Process == nil {
		return ""
	}
	return ev.Exec.Process.OwnerSidString
}

// GetExitCause returns the value of the field, resolving if necessary
func (ev *Event) GetExitCause() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	return ev.Exit.Cause
}

// GetExitCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetExitCmdline() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process)
}

// GetExitCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExitCmdlineScrubbed() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, ev.Exit.Process)
}

// GetExitCode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCode() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	return ev.Exit.Code
}

// GetExitContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetExitContainerId() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.ContainerID
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

// GetExitFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileName() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
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

// GetExitFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePath() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
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

// GetExitUser returns the value of the field, resolving if necessary
func (ev *Event) GetExitUser() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process)
}

// GetExitUserSid returns the value of the field, resolving if necessary
func (ev *Event) GetExitUserSid() string {
	if ev.GetEventType().String() != "exit" {
		return ""
	}
	if ev.Exit.Process == nil {
		return ""
	}
	return ev.Exit.Process.OwnerSidString
}

// GetOpenRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRegistryKeyName() string {
	if ev.GetEventType().String() != "open_key" {
		return ""
	}
	return ev.OpenRegistryKey.Registry.KeyName
}

// GetOpenRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "open_key" {
		return 0
	}
	return len(ev.OpenRegistryKey.Registry.KeyName)
}

// GetOpenRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRegistryKeyPath() string {
	if ev.GetEventType().String() != "open_key" {
		return ""
	}
	return ev.OpenRegistryKey.Registry.KeyPath
}

// GetOpenRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "open_key" {
		return 0
	}
	return len(ev.OpenRegistryKey.Registry.KeyPath)
}

// GetOpenKeyRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetOpenKeyRegistryKeyName() string {
	if ev.GetEventType().String() != "open_key" {
		return ""
	}
	return ev.OpenRegistryKey.Registry.KeyName
}

// GetOpenKeyRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenKeyRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "open_key" {
		return 0
	}
	return len(ev.OpenRegistryKey.Registry.KeyName)
}

// GetOpenKeyRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetOpenKeyRegistryKeyPath() string {
	if ev.GetEventType().String() != "open_key" {
		return ""
	}
	return ev.OpenRegistryKey.Registry.KeyPath
}

// GetOpenKeyRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetOpenKeyRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "open_key" {
		return 0
	}
	return len(ev.OpenRegistryKey.Registry.KeyPath)
}

// GetProcessAncestorsCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCmdline() []string {
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
		result := ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCmdlineScrubbed() []string {
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
		result := ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, &element.ProcessContext.Process)
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
		result := element.ProcessContext.Process.ContainerID
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
		result := ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsUserSid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsUserSid() []string {
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
		result := element.ProcessContext.Process.OwnerSidString
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCmdline() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCmdlineScrubbed() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessContainerId returns the value of the field, resolving if necessary
func (ev *Event) GetProcessContainerId() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.ContainerID
}

// GetProcessCreatedAt returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCreatedAt() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)
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

// GetProcessFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileName() string {
	if ev.BaseEvent.ProcessContext == nil {
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

// GetProcessFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePath() string {
	if ev.BaseEvent.ProcessContext == nil {
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

// GetProcessParentCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCmdline() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCmdlineScrubbed() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, ev.BaseEvent.ProcessContext.Parent)
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
	return ev.BaseEvent.ProcessContext.Parent.ContainerID
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
	return ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentUserSid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentUserSid() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	if ev.BaseEvent.ProcessContext.Parent == nil {
		return ""
	}
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.OwnerSidString
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

// GetProcessUser returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUser() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessUserSid returns the value of the field, resolving if necessary
func (ev *Event) GetProcessUserSid() string {
	if ev.BaseEvent.ProcessContext == nil {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Process.OwnerSidString
}

// GetSetRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetSetRegistryKeyName() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.Registry.KeyName
}

// GetSetRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "set_key_value" {
		return 0
	}
	return len(ev.SetRegistryKeyValue.Registry.KeyName)
}

// GetSetRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetSetRegistryKeyPath() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.Registry.KeyPath
}

// GetSetRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "set_key_value" {
		return 0
	}
	return len(ev.SetRegistryKeyValue.Registry.KeyPath)
}

// GetSetRegistryValueName returns the value of the field, resolving if necessary
func (ev *Event) GetSetRegistryValueName() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.ValueName
}

// GetSetRegistryValueNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetRegistryValueNameLength() int {
	if ev.GetEventType().String() != "set_key_value" {
		return 0
	}
	return len(ev.SetRegistryKeyValue.ValueName)
}

// GetSetValueName returns the value of the field, resolving if necessary
func (ev *Event) GetSetValueName() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.ValueName
}

// GetSetKeyValueRegistryKeyName returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueRegistryKeyName() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.Registry.KeyName
}

// GetSetKeyValueRegistryKeyNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueRegistryKeyNameLength() int {
	if ev.GetEventType().String() != "set_key_value" {
		return 0
	}
	return len(ev.SetRegistryKeyValue.Registry.KeyName)
}

// GetSetKeyValueRegistryKeyPath returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueRegistryKeyPath() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.Registry.KeyPath
}

// GetSetKeyValueRegistryKeyPathLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueRegistryKeyPathLength() int {
	if ev.GetEventType().String() != "set_key_value" {
		return 0
	}
	return len(ev.SetRegistryKeyValue.Registry.KeyPath)
}

// GetSetKeyValueRegistryValueName returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueRegistryValueName() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.ValueName
}

// GetSetKeyValueRegistryValueNameLength returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueRegistryValueNameLength() int {
	if ev.GetEventType().String() != "set_key_value" {
		return 0
	}
	return len(ev.SetRegistryKeyValue.ValueName)
}

// GetSetKeyValueValueName returns the value of the field, resolving if necessary
func (ev *Event) GetSetKeyValueValueName() string {
	if ev.GetEventType().String() != "set_key_value" {
		return ""
	}
	return ev.SetRegistryKeyValue.ValueName
}

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev, &ev.BaseEvent)
}
