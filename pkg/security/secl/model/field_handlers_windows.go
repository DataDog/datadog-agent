//go:build windows && npm
// +build windows,npm

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.
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
	_ = ev.FieldHandlers.ResolveProcessArgs(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessArgv(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.ProcessContext.Process)
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Process.FileEvent)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Process.FileEvent.FileFields)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Process.FileEvent.FileFields)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.FileEvent)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.FileEvent)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Process.FileEvent.FileFields)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.ProcessContext.Parent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Parent.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.FileEvent.FileFields)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	// resolve event specific fields
	switch ev.GetEventType().String() {
	case "exec":
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process)
	case "exit":
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
	}
}

type FieldHandlers interface {
	ResolveFileBasename(ev *Event, e *FileEvent) string
	ResolveFileFieldsGroup(ev *Event, e *FileFields) string
	ResolveFileFieldsInUpperLayer(ev *Event, e *FileFields) bool
	ResolveFileFieldsUser(ev *Event, e *FileFields) string
	ResolveFileFilesystem(ev *Event, e *FileEvent) string
	ResolveFilePath(ev *Event, e *FileEvent) string
	ResolveProcessArgs(ev *Event, e *Process) string
	ResolveProcessArgsFlags(ev *Event, e *Process) []string
	ResolveProcessArgsOptions(ev *Event, e *Process) []string
	ResolveProcessArgsTruncated(ev *Event, e *Process) bool
	ResolveProcessArgv(ev *Event, e *Process) []string
	ResolveProcessArgv0(ev *Event, e *Process) string
	ResolveProcessCreatedAt(ev *Event, e *Process) int
	ResolveProcessEnvp(ev *Event, e *Process) []string
	ResolveProcessEnvs(ev *Event, e *Process) []string
	ResolveProcessEnvsTruncated(ev *Event, e *Process) bool
	ResolveRights(ev *Event, e *FileFields) int
	// custom handlers not tied to any fields
	ExtraFieldHandlers
}
type DefaultFieldHandlers struct{}

func (dfh *DefaultFieldHandlers) ResolveFileBasename(ev *Event, e *FileEvent) string {
	return e.BasenameStr
}
func (dfh *DefaultFieldHandlers) ResolveFileFieldsGroup(ev *Event, e *FileFields) string {
	return e.Group
}
func (dfh *DefaultFieldHandlers) ResolveFileFieldsInUpperLayer(ev *Event, e *FileFields) bool {
	return e.InUpperLayer
}
func (dfh *DefaultFieldHandlers) ResolveFileFieldsUser(ev *Event, e *FileFields) string {
	return e.User
}
func (dfh *DefaultFieldHandlers) ResolveFileFilesystem(ev *Event, e *FileEvent) string {
	return e.Filesystem
}
func (dfh *DefaultFieldHandlers) ResolveFilePath(ev *Event, e *FileEvent) string {
	return e.PathnameStr
}
func (dfh *DefaultFieldHandlers) ResolveProcessArgs(ev *Event, e *Process) string { return e.Args }
func (dfh *DefaultFieldHandlers) ResolveProcessArgsFlags(ev *Event, e *Process) []string {
	return e.Argv
}
func (dfh *DefaultFieldHandlers) ResolveProcessArgsOptions(ev *Event, e *Process) []string {
	return e.Argv
}
func (dfh *DefaultFieldHandlers) ResolveProcessArgsTruncated(ev *Event, e *Process) bool {
	return e.ArgsTruncated
}
func (dfh *DefaultFieldHandlers) ResolveProcessArgv(ev *Event, e *Process) []string { return e.Argv }
func (dfh *DefaultFieldHandlers) ResolveProcessArgv0(ev *Event, e *Process) string  { return e.Argv0 }
func (dfh *DefaultFieldHandlers) ResolveProcessCreatedAt(ev *Event, e *Process) int {
	return int(e.CreatedAt)
}
func (dfh *DefaultFieldHandlers) ResolveProcessEnvp(ev *Event, e *Process) []string { return e.Envp }
func (dfh *DefaultFieldHandlers) ResolveProcessEnvs(ev *Event, e *Process) []string { return e.Envs }
func (dfh *DefaultFieldHandlers) ResolveProcessEnvsTruncated(ev *Event, e *Process) bool {
	return e.EnvsTruncated
}
func (dfh *DefaultFieldHandlers) ResolveRights(ev *Event, e *FileFields) int { return int(e.Mode) }
