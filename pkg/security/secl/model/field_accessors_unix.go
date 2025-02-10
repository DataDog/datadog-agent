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

// GetExitCode returns the value of the field, resolving if necessary
func (ev *Event) GetExitCode() uint32 {
	if ev.GetEventType().String() != "exit" {
		return uint32(0)
	}
	return ev.Exit.Code
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

// GetTimestamp returns the value of the field, resolving if necessary
func (ev *Event) GetTimestamp() time.Time {
	return ev.FieldHandlers.ResolveEventTime(ev, &ev.BaseEvent)
}
