// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows
// +build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"time"
)

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
	return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
}

// GetEventTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetEventTimestamp() int {
	return ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)
}

// GetExecCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetExecCmdline() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
}

// GetExecCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExecCmdlineScrubbed() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, ev.Exec.Process)
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

// GetExecEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvp() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
}

// GetExecEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExecEnvs() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
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

// GetExecFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExecFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
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

// GetExecFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExecFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exec" {
		return zeroValue
	}
	if ev.Exec.Process == nil {
		return zeroValue
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

// GetExitCause returns the value of the field, resolving if necessary
func (ev *Event) GetExitCause() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	return ev.Exit.Cause
}

// GetExitCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetExitCmdline() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process)
}

// GetExitCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetExitCmdlineScrubbed() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, ev.Exit.Process)
}

// GetExitCode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCode() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	return ev.Exit.Code
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

// GetExitEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvp() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
}

// GetExitEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetExitEnvs() []string {
	zeroValue := []string{}
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
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

// GetExitFileName returns the value of the field, resolving if necessary
func (ev *Event) GetExitFileName() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
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

// GetExitFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetExitFilePath() string {
	zeroValue := ""
	if ev.GetEventType().String() != "exit" {
		return zeroValue
	}
	if ev.Exit.Process == nil {
		return zeroValue
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

// GetProcessAncestorsCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCmdline() []string {
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
		result := ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process)
		values = append(values, result)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsCmdlineScrubbed() []string {
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
		result := ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, &element.ProcessContext.Process)
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

// GetProcessAncestorsEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvp() []string {
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
		values = append(values, result...)
		ptr = iterator.Next()
	}
	return values
}

// GetProcessAncestorsEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessAncestorsEnvs() []string {
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
		values = append(values, result...)
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

// GetProcessCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCmdline() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessCmdlineScrubbed() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, &ev.BaseEvent.ProcessContext.Process)
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

// GetProcessEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvp() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
}

// GetProcessEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessEnvs() []string {
	zeroValue := []string{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
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

// GetProcessFileName returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFileName() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
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

// GetProcessFilePath returns the value of the field, resolving if necessary
func (ev *Event) GetProcessFilePath() string {
	zeroValue := ""
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
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

// GetProcessParentCmdline returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCmdline() string {
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
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentCmdlineScrubbed returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentCmdlineScrubbed() string {
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
	return ev.FieldHandlers.ResolveProcessCmdLineScrubbed(ev, ev.BaseEvent.ProcessContext.Parent)
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

// GetProcessParentEnvp returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvp() []string {
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
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
}

// GetProcessParentEnvs returns the value of the field, resolving if necessary
func (ev *Event) GetProcessParentEnvs() []string {
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
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
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

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev, &ev.BaseEvent)
}
