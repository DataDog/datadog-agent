// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows
// +build windows

package model

// ResolveFields resolves all the fields associate to the event type. Context fields are automatically resolved.
func (ev *Event) ResolveFields() {
	ev.resolveFields(false)
}

// ResolveFieldsForAD resolves all the fields associate to the event type. Context fields are automatically resolved.
func (ev *Event) ResolveFieldsForAD() {
	ev.resolveFields(true)
}
func (ev *Event) resolveFields(forADs bool) {
	// resolve context fields that are not related to any event type
	_ = ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext)
	_ = ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
	if !forADs {
		_ = ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
	}
	_ = ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)
	_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	// resolve event specific fields
	switch ev.GetEventType().String() {
	case "dns":
	case "exec":
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
	case "exit":
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
	}
}

type FieldHandlers interface {
	ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int
	ResolveContainerID(ev *Event, e *ContainerContext) string
	ResolveContainerTags(ev *Event, e *ContainerContext) []string
	ResolveEventTimestamp(ev *Event, e *BaseEvent) int
	ResolveFileBasename(ev *Event, e *FileEvent) string
	ResolveFilePath(ev *Event, e *FileEvent) string
	ResolveProcessArgsFlags(ev *Event, e *Process) []string
	ResolveProcessArgsOptions(ev *Event, e *Process) []string
	ResolveProcessCreatedAt(ev *Event, e *Process) int
	ResolveProcessEnvp(ev *Event, e *Process) []string
	ResolveProcessEnvs(ev *Event, e *Process) []string
	// custom handlers not tied to any fields
	ExtraFieldHandlers
}
type DefaultFieldHandlers struct{}

func (dfh *DefaultFieldHandlers) ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int {
	return int(e.CreatedAt)
}
func (dfh *DefaultFieldHandlers) ResolveContainerID(ev *Event, e *ContainerContext) string {
	return e.ID
}
func (dfh *DefaultFieldHandlers) ResolveContainerTags(ev *Event, e *ContainerContext) []string {
	return e.Tags
}
func (dfh *DefaultFieldHandlers) ResolveEventTimestamp(ev *Event, e *BaseEvent) int {
	return int(e.TimestampRaw)
}
func (dfh *DefaultFieldHandlers) ResolveFileBasename(ev *Event, e *FileEvent) string {
	return e.BasenameStr
}
func (dfh *DefaultFieldHandlers) ResolveFilePath(ev *Event, e *FileEvent) string {
	return e.PathnameStr
}
func (dfh *DefaultFieldHandlers) ResolveProcessArgsFlags(ev *Event, e *Process) []string {
	return e.Argv
}
func (dfh *DefaultFieldHandlers) ResolveProcessArgsOptions(ev *Event, e *Process) []string {
	return e.Argv
}
func (dfh *DefaultFieldHandlers) ResolveProcessCreatedAt(ev *Event, e *Process) int {
	return int(e.CreatedAt)
}
func (dfh *DefaultFieldHandlers) ResolveProcessEnvp(ev *Event, e *Process) []string { return e.Envp }
func (dfh *DefaultFieldHandlers) ResolveProcessEnvs(ev *Event, e *Process) []string { return e.Envs }
