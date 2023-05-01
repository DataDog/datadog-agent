// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix
// +build unix

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
	_ = ev.FieldHandlers.ResolveAsync(ev)
	_ = ev.FieldHandlers.ResolveContainerCreatedAt(ev, &ev.ContainerContext)
	_ = ev.FieldHandlers.ResolveContainerID(ev, &ev.ContainerContext)
	if !forADs {
		_ = ev.FieldHandlers.ResolveContainerTags(ev, &ev.ContainerContext)
	}
	_ = ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkContext.Device)
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
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.ProcessContext.Process.FileEvent)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.ProcessContext.Process.FileEvent)
	}
	if ev.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.ProcessContext.Process.FileEvent)
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
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.ProcessContext.Process.LinuxBinprm.FileEvent)
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
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.ProcessContext.Parent.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.ProcessContext.Parent.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.ProcessContext.Parent.FileEvent)
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
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.ProcessContext.HasParent() && ev.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	// resolve event specific fields
	switch ev.GetEventType().String() {
	case "bind":
	case "bpf":
	case "capset":
	case "chmod":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chmod.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File)
	case "chown":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chown.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File)
		_ = ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown)
		_ = ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown)
	case "dns":
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
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent)
		}
		if ev.Exec.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent)
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
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exec.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
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
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent)
		}
		if ev.Exit.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent)
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
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
		}
		if ev.Exit.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
	case "link":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Source.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source)
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target)
	case "load_module":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.LoadModule.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File)
		_ = ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule)
		_ = ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule)
	case "mkdir":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Mkdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File)
	case "mmap":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.MMap.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File)
	case "mount":
		_ = ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount)
		_ = ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount)
	case "mprotect":
	case "open":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Open.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File)
	case "ptrace":
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
		}
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process)
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.PTrace.Tracee.Parent)
		}
	case "removexattr":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.RemoveXAttr.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File)
		_ = ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr)
		_ = ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr)
	case "rename":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.Old.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old)
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New)
	case "rmdir":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rmdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File)
	case "selinux":
		_ = ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux)
	case "setgid":
		_ = ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID)
		_ = ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID)
		_ = ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID)
	case "setuid":
		_ = ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID)
		_ = ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID)
		_ = ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID)
	case "setxattr":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.SetXAttr.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File)
		_ = ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr)
		_ = ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr)
	case "signal":
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.FileEvent.FileFields)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent)
		}
		if ev.Signal.Target.Process.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.Process.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process)
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Signal.Target.Parent)
		}
	case "splice":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Splice.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File)
	case "unlink":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Unlink.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File)
	case "unload_module":
	case "utimes":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Utimes.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File)
	}
}

type FieldHandlers interface {
	ResolveAsync(ev *Event) bool
	ResolveChownGID(ev *Event, e *ChownEvent) string
	ResolveChownUID(ev *Event, e *ChownEvent) string
	ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int
	ResolveContainerID(ev *Event, e *ContainerContext) string
	ResolveContainerTags(ev *Event, e *ContainerContext) []string
	ResolveFileBasename(ev *Event, e *FileEvent) string
	ResolveFileFieldsGroup(ev *Event, e *FileFields) string
	ResolveFileFieldsInUpperLayer(ev *Event, e *FileFields) bool
	ResolveFileFieldsUser(ev *Event, e *FileFields) string
	ResolveFileFilesystem(ev *Event, e *FileEvent) string
	ResolveFilePath(ev *Event, e *FileEvent) string
	ResolveModuleArgs(ev *Event, e *LoadModuleEvent) string
	ResolveModuleArgv(ev *Event, e *LoadModuleEvent) []string
	ResolveMountPointPath(ev *Event, e *MountEvent) string
	ResolveMountSourcePath(ev *Event, e *MountEvent) string
	ResolveNetworkDeviceIfName(ev *Event, e *NetworkDeviceContext) string
	ResolvePackageName(ev *Event, e *FileEvent) string
	ResolvePackageSourceVersion(ev *Event, e *FileEvent) string
	ResolvePackageVersion(ev *Event, e *FileEvent) string
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
	ResolveSELinuxBoolName(ev *Event, e *SELinuxEvent) string
	ResolveSetgidEGroup(ev *Event, e *SetgidEvent) string
	ResolveSetgidFSGroup(ev *Event, e *SetgidEvent) string
	ResolveSetgidGroup(ev *Event, e *SetgidEvent) string
	ResolveSetuidEUser(ev *Event, e *SetuidEvent) string
	ResolveSetuidFSUser(ev *Event, e *SetuidEvent) string
	ResolveSetuidUser(ev *Event, e *SetuidEvent) string
	ResolveXAttrName(ev *Event, e *SetXAttrEvent) string
	ResolveXAttrNamespace(ev *Event, e *SetXAttrEvent) string
	// custom handlers not tied to any fields
	ExtraFieldHandlers
}
type DefaultFieldHandlers struct{}

func (dfh *DefaultFieldHandlers) ResolveAsync(ev *Event) bool                     { return ev.Async }
func (dfh *DefaultFieldHandlers) ResolveChownGID(ev *Event, e *ChownEvent) string { return e.Group }
func (dfh *DefaultFieldHandlers) ResolveChownUID(ev *Event, e *ChownEvent) string { return e.User }
func (dfh *DefaultFieldHandlers) ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int {
	return int(e.CreatedAt)
}
func (dfh *DefaultFieldHandlers) ResolveContainerID(ev *Event, e *ContainerContext) string {
	return e.ID
}
func (dfh *DefaultFieldHandlers) ResolveContainerTags(ev *Event, e *ContainerContext) []string {
	return e.Tags
}
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
func (dfh *DefaultFieldHandlers) ResolveModuleArgs(ev *Event, e *LoadModuleEvent) string {
	return e.Args
}
func (dfh *DefaultFieldHandlers) ResolveModuleArgv(ev *Event, e *LoadModuleEvent) []string {
	return e.Argv
}
func (dfh *DefaultFieldHandlers) ResolveMountPointPath(ev *Event, e *MountEvent) string {
	return e.MountPointPath
}
func (dfh *DefaultFieldHandlers) ResolveMountSourcePath(ev *Event, e *MountEvent) string {
	return e.MountSourcePath
}
func (dfh *DefaultFieldHandlers) ResolveNetworkDeviceIfName(ev *Event, e *NetworkDeviceContext) string {
	return e.IfName
}
func (dfh *DefaultFieldHandlers) ResolvePackageName(ev *Event, e *FileEvent) string { return e.PkgName }
func (dfh *DefaultFieldHandlers) ResolvePackageSourceVersion(ev *Event, e *FileEvent) string {
	return e.PkgSrcVersion
}
func (dfh *DefaultFieldHandlers) ResolvePackageVersion(ev *Event, e *FileEvent) string {
	return e.PkgVersion
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
func (dfh *DefaultFieldHandlers) ResolveSELinuxBoolName(ev *Event, e *SELinuxEvent) string {
	return e.BoolName
}
func (dfh *DefaultFieldHandlers) ResolveSetgidEGroup(ev *Event, e *SetgidEvent) string {
	return e.EGroup
}
func (dfh *DefaultFieldHandlers) ResolveSetgidFSGroup(ev *Event, e *SetgidEvent) string {
	return e.FSGroup
}
func (dfh *DefaultFieldHandlers) ResolveSetgidGroup(ev *Event, e *SetgidEvent) string { return e.Group }
func (dfh *DefaultFieldHandlers) ResolveSetuidEUser(ev *Event, e *SetuidEvent) string { return e.EUser }
func (dfh *DefaultFieldHandlers) ResolveSetuidFSUser(ev *Event, e *SetuidEvent) string {
	return e.FSUser
}
func (dfh *DefaultFieldHandlers) ResolveSetuidUser(ev *Event, e *SetuidEvent) string  { return e.User }
func (dfh *DefaultFieldHandlers) ResolveXAttrName(ev *Event, e *SetXAttrEvent) string { return e.Name }
func (dfh *DefaultFieldHandlers) ResolveXAttrNamespace(ev *Event, e *SetXAttrEvent) string {
	return e.Namespace
}
