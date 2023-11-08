// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix && ebpfless

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
	resolvedField := ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
	fieldCopy := make([]string, len(resolvedField))
	copy(fieldCopy, resolvedField)
	return fieldCopy
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

// GetOpenFileDestinationMode returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFileDestinationMode() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.Mode
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

// GetOpenFlags returns the value of the field, resolving if necessary
func (ev *Event) GetOpenFlags() uint32 {
	zeroValue := uint32(0)
	if ev.GetEventType().String() != "open" {
		return zeroValue
	}
	return ev.Open.Flags
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

// GetProcessForkTime returns the value of the field, resolving if necessary
func (ev *Event) GetProcessForkTime() time.Time {
	zeroValue := time.Time{}
	if ev.BaseEvent.ProcessContext == nil {
		return zeroValue
	}
	return ev.BaseEvent.ProcessContext.Process.ForkTime
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
	return ev.FieldHandlers.ResolveEventTime(ev)
}
