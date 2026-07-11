// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

package model

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"net"
	"time"
)

var _ = time.Time{}
var _ = net.IP{}
var _ = eval.NewContext

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

// ValidateFileField validates that GetFileField would return a valid FileEvent
func (e *Event) ValidateFileField(field string) error {
	switch field {
	case "process.file":
		return nil
	case "process.interpreter.file":
		return nil
	case "process.parent.file":
		return nil
	case "process.parent.interpreter.file":
		return nil
	case "chmod.file":
		return nil
	case "chown.file":
		return nil
	case "open.file":
		return nil
	case "mkdir.file":
		return nil
	case "rmdir.file":
		return nil
	case "rename.file":
		return nil
	case "rename.file.destination":
		return nil
	case "unlink.file":
		return nil
	case "utimes.file":
		return nil
	case "link.file":
		return nil
	case "link.file.destination":
		return nil
	case "setxattr.file":
		return nil
	case "removexattr.file":
		return nil
	case "splice.file":
		return nil
	case "chdir.file":
		return nil
	case "exec.file":
		return nil
	case "exec.interpreter.file":
		return nil
	case "signal.target.file":
		return nil
	case "signal.target.interpreter.file":
		return nil
	case "signal.target.parent.file":
		return nil
	case "signal.target.parent.interpreter.file":
		return nil
	case "exit.file":
		return nil
	case "exit.interpreter.file":
		return nil
	case "setrlimit.target.file":
		return nil
	case "setrlimit.target.interpreter.file":
		return nil
	case "setrlimit.target.parent.file":
		return nil
	case "setrlimit.target.parent.interpreter.file":
		return nil
	case "ptrace.tracee.file":
		return nil
	case "ptrace.tracee.interpreter.file":
		return nil
	case "ptrace.tracee.parent.file":
		return nil
	case "ptrace.tracee.parent.interpreter.file":
		return nil
	case "mmap.file":
		return nil
	case "load_module.file":
		return nil
	case "cgroup_write.file":
		return nil
	default:
		return fmt.Errorf("invalid field %s on event %s", field, e.GetEventType())
	}
}

// GetFileField returns the FileEvent associated with a field name
func (e *Event) GetFileField(field string) (*FileEvent, error) {
	switch field {
	case "process.file":
		if !e.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.BaseEvent.ProcessContext.Process.FileEvent, nil
	case "process.interpreter.file":
		if !e.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent, nil
	case "process.parent.file":
		if !e.BaseEvent.ProcessContext.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.BaseEvent.ProcessContext.Parent.FileEvent, nil
	case "process.parent.interpreter.file":
		if !e.BaseEvent.ProcessContext.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent, nil
	case "chmod.file":
		return &e.Chmod.File, nil
	case "chown.file":
		return &e.Chown.File, nil
	case "open.file":
		return &e.Open.File, nil
	case "mkdir.file":
		return &e.Mkdir.File, nil
	case "rmdir.file":
		return &e.Rmdir.File, nil
	case "rename.file":
		return &e.Rename.Old, nil
	case "rename.file.destination":
		return &e.Rename.New, nil
	case "unlink.file":
		return &e.Unlink.File, nil
	case "utimes.file":
		return &e.Utimes.File, nil
	case "link.file":
		return &e.Link.Source, nil
	case "link.file.destination":
		return &e.Link.Target, nil
	case "setxattr.file":
		return &e.SetXAttr.File, nil
	case "removexattr.file":
		return &e.RemoveXAttr.File, nil
	case "splice.file":
		return &e.Splice.File, nil
	case "chdir.file":
		return &e.Chdir.File, nil
	case "exec.file":
		if !e.Exec.Process.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Exec.Process.FileEvent, nil
	case "exec.interpreter.file":
		if !e.Exec.Process.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Exec.Process.LinuxBinprm.FileEvent, nil
	case "signal.target.file":
		if !e.Signal.Target.Process.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Signal.Target.Process.FileEvent, nil
	case "signal.target.interpreter.file":
		if !e.Signal.Target.Process.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Signal.Target.Process.LinuxBinprm.FileEvent, nil
	case "signal.target.parent.file":
		if !e.Signal.Target.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.Signal.Target.Parent.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Signal.Target.Parent.FileEvent, nil
	case "signal.target.parent.interpreter.file":
		if !e.Signal.Target.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.Signal.Target.Parent.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Signal.Target.Parent.LinuxBinprm.FileEvent, nil
	case "exit.file":
		if !e.Exit.Process.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Exit.Process.FileEvent, nil
	case "exit.interpreter.file":
		if !e.Exit.Process.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Exit.Process.LinuxBinprm.FileEvent, nil
	case "setrlimit.target.file":
		if !e.Setrlimit.Target.Process.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Setrlimit.Target.Process.FileEvent, nil
	case "setrlimit.target.interpreter.file":
		if !e.Setrlimit.Target.Process.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Setrlimit.Target.Process.LinuxBinprm.FileEvent, nil
	case "setrlimit.target.parent.file":
		if !e.Setrlimit.Target.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.Setrlimit.Target.Parent.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Setrlimit.Target.Parent.FileEvent, nil
	case "setrlimit.target.parent.interpreter.file":
		if !e.Setrlimit.Target.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.Setrlimit.Target.Parent.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.Setrlimit.Target.Parent.LinuxBinprm.FileEvent, nil
	case "ptrace.tracee.file":
		if !e.PTrace.Tracee.Process.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.PTrace.Tracee.Process.FileEvent, nil
	case "ptrace.tracee.interpreter.file":
		if !e.PTrace.Tracee.Process.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.PTrace.Tracee.Process.LinuxBinprm.FileEvent, nil
	case "ptrace.tracee.parent.file":
		if !e.PTrace.Tracee.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.PTrace.Tracee.Parent.IsNotKworker() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.PTrace.Tracee.Parent.FileEvent, nil
	case "ptrace.tracee.parent.interpreter.file":
		if !e.PTrace.Tracee.HasParent() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		if !e.PTrace.Tracee.Parent.HasInterpreter() {
			return nil, fmt.Errorf("no file event on this event %s", e.GetEventType())
		}
		return &e.PTrace.Tracee.Parent.LinuxBinprm.FileEvent, nil
	case "mmap.file":
		return &e.MMap.File, nil
	case "load_module.file":
		return &e.LoadModule.File, nil
	case "cgroup_write.file":
		return &e.CgroupWrite.File, nil
	default:
		return nil, fmt.Errorf("invalid field %s on event %s", field, e.GetEventType())
	}
}
