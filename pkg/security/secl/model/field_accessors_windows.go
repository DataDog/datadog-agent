// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"net"
	"time"
)

var _ = time.Time{}
var _ = net.IP{}
var _ = eval.NewContext

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
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}

// GetProcessFilePathLength returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePathLength() int {
	if ev.BaseEvent.ProcessContext == nil {
		return 0
	}
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
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

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev, &ev.BaseEvent)
}
