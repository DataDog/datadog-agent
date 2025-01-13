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

var _ = time.Time{}
var _ = net.IP{}
var _ = eval.NewContext

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

// GetEventService returns the value of the field, resolving if necessary
func (ev *Event) GetEventService() string {
	return ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
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

// GetMountMountpointPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountMountpointPath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount)
}

// GetMountRootPath returns the value of the field, resolving if necessary
func (ev *Event) GetMountRootPath() string {
	if ev.GetEventType().String() != "mount" {
		return ""
	}
	return ev.FieldHandlers.ResolveMountRootPath(ev, &ev.Mount)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
	}
	return values
}

// GetProcessCmdargv returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCmdargv() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessCmdArgv(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvp() []string {
	if ev.BaseEvent.ProcessContext == nil {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
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

// GetProcessForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessForkTime() time.Time {
	if ev.BaseEvent.ProcessContext == nil {
		return time.Time{}
	}
	return ev.BaseEvent.ProcessContext.Process.ForkTime
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
	}
	return values
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
	}
	return values
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
	}
	return values
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
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
		ptr = iterator.Next(ctx)
	}
	return values
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

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev, &ev.BaseEvent)
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
