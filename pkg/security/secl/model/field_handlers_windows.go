// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"time"
)

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
	_ = ev.FieldHandlers.ResolveContainerRuntime(ev, ev.BaseEvent.ContainerContext)
	if !forADs {
		_ = ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
	}
	_ = ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent)
	if !forADs {
		_ = ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
	}
	_ = ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)
	_ = ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent)
	}
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
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	_ = ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process)
	// resolve event specific fields
	switch ev.GetEventType().String() {
	case "change_permission":
		_ = ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission)
		_ = ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission)
	case "create":
		_ = ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File)
		_ = ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File)
		_ = ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File)
	case "create_key":
	case "delete":
		_ = ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File)
		_ = ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File)
		_ = ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File)
	case "delete_key":
	case "exec":
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
	case "exit":
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
	case "open_key":
	case "rename":
		_ = ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old)
		_ = ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old)
		_ = ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old)
		_ = ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New)
		_ = ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New)
		_ = ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New)
	case "set_key_value":
	case "write":
		_ = ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File)
		_ = ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File)
		_ = ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File)
	}
}

type FieldHandlers interface {
	ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int
	ResolveContainerID(ev *Event, e *ContainerContext) string
	ResolveContainerRuntime(ev *Event, e *ContainerContext) string
	ResolveContainerTags(ev *Event, e *ContainerContext) []string
	ResolveEventTime(ev *Event, e *BaseEvent) time.Time
	ResolveEventTimestamp(ev *Event, e *BaseEvent) int
	ResolveFileBasename(ev *Event, e *FileEvent) string
	ResolveFilePath(ev *Event, e *FileEvent) string
	ResolveFileUserPath(ev *Event, e *FimFileEvent) string
	ResolveFimFileBasename(ev *Event, e *FimFileEvent) string
	ResolveFimFilePath(ev *Event, e *FimFileEvent) string
	ResolveHostname(ev *Event, e *BaseEvent) string
	ResolveNewSecurityDescriptor(ev *Event, e *ChangePermissionEvent) string
	ResolveOldSecurityDescriptor(ev *Event, e *ChangePermissionEvent) string
	ResolveProcessCmdLine(ev *Event, e *Process) string
	ResolveProcessCmdLineScrubbed(ev *Event, e *Process) string
	ResolveProcessCreatedAt(ev *Event, e *Process) int
	ResolveProcessEnvp(ev *Event, e *Process) []string
	ResolveProcessEnvs(ev *Event, e *Process) []string
	ResolveService(ev *Event, e *BaseEvent) string
	ResolveUser(ev *Event, e *Process) string
	// custom handlers not tied to any fields
	ExtraFieldHandlers
}
type FakeFieldHandlers struct{}

func (dfh *FakeFieldHandlers) ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int {
	return int(e.CreatedAt)
}
func (dfh *FakeFieldHandlers) ResolveContainerID(ev *Event, e *ContainerContext) string {
	return string(e.ContainerID)
}
func (dfh *FakeFieldHandlers) ResolveContainerRuntime(ev *Event, e *ContainerContext) string {
	return string(e.Runtime)
}
func (dfh *FakeFieldHandlers) ResolveContainerTags(ev *Event, e *ContainerContext) []string {
	return []string(e.Tags)
}
func (dfh *FakeFieldHandlers) ResolveEventTime(ev *Event, e *BaseEvent) time.Time {
	return time.Time(e.Timestamp)
}
func (dfh *FakeFieldHandlers) ResolveEventTimestamp(ev *Event, e *BaseEvent) int {
	return int(e.TimestampRaw)
}
func (dfh *FakeFieldHandlers) ResolveFileBasename(ev *Event, e *FileEvent) string {
	return string(e.BasenameStr)
}
func (dfh *FakeFieldHandlers) ResolveFilePath(ev *Event, e *FileEvent) string {
	return string(e.PathnameStr)
}
func (dfh *FakeFieldHandlers) ResolveFileUserPath(ev *Event, e *FimFileEvent) string {
	return string(e.UserPathnameStr)
}
func (dfh *FakeFieldHandlers) ResolveFimFileBasename(ev *Event, e *FimFileEvent) string {
	return string(e.BasenameStr)
}
func (dfh *FakeFieldHandlers) ResolveFimFilePath(ev *Event, e *FimFileEvent) string {
	return string(e.PathnameStr)
}
func (dfh *FakeFieldHandlers) ResolveHostname(ev *Event, e *BaseEvent) string {
	return string(e.Hostname)
}
func (dfh *FakeFieldHandlers) ResolveNewSecurityDescriptor(ev *Event, e *ChangePermissionEvent) string {
	return string(e.NewSd)
}
func (dfh *FakeFieldHandlers) ResolveOldSecurityDescriptor(ev *Event, e *ChangePermissionEvent) string {
	return string(e.OldSd)
}
func (dfh *FakeFieldHandlers) ResolveProcessCmdLine(ev *Event, e *Process) string {
	return string(e.CmdLine)
}
func (dfh *FakeFieldHandlers) ResolveProcessCmdLineScrubbed(ev *Event, e *Process) string {
	return string(e.CmdLineScrubbed)
}
func (dfh *FakeFieldHandlers) ResolveProcessCreatedAt(ev *Event, e *Process) int {
	return int(e.CreatedAt)
}
func (dfh *FakeFieldHandlers) ResolveProcessEnvp(ev *Event, e *Process) []string {
	return []string(e.Envp)
}
func (dfh *FakeFieldHandlers) ResolveProcessEnvs(ev *Event, e *Process) []string {
	return []string(e.Envs)
}
func (dfh *FakeFieldHandlers) ResolveService(ev *Event, e *BaseEvent) string {
	return string(e.Service)
}
func (dfh *FakeFieldHandlers) ResolveUser(ev *Event, e *Process) string { return string(e.User) }
