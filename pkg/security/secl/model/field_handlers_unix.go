// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

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
	_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.CGroupContext)
	_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.CGroupContext)
	_ = ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext)
	_ = ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
	_ = ev.FieldHandlers.ResolveContainerRuntime(ev, ev.BaseEvent.ContainerContext)
	if !forADs {
		_ = ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
	}
	_ = ev.FieldHandlers.ResolveAsync(ev)
	_ = ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent)
	_ = ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
	_ = ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)
	_ = ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkContext.Device)
	if !forADs {
		_ = ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process)
	}
	_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessArgv(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.BaseEvent.ProcessContext.Process.CGroup)
	_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.BaseEvent.ProcessContext.Process.CGroup)
	_ = ev.FieldHandlers.ResolveProcessContainerID(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
	_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.BaseEvent.ProcessContext.Process)
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
		}
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
		}
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)
	}
	_ = ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.BaseEvent.ProcessContext.Process)
	if ev.BaseEvent.ProcessContext.HasParent() {
		if !forADs {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent)
		}
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.BaseEvent.ProcessContext.Parent.CGroup)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.BaseEvent.ProcessContext.Parent.CGroup)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessContainerID(ev, ev.BaseEvent.ProcessContext.Parent)
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
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
		}
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
		}
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() && ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveProcessIsThread(ev, ev.BaseEvent.ProcessContext.Parent)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession)
	}
	if ev.BaseEvent.ProcessContext.HasParent() {
		_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession)
	}
	_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Process.UserSession)
	_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Process.UserSession)
	_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Process.UserSession)
	// resolve event specific fields
	switch ev.GetEventType().String() {
	case "bind":
	case "bpf":
	case "capset":
	case "chdir":
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chdir.File.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Chdir.File)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chdir.File)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chdir.File)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Chdir.File)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chdir.File)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chdir.File)
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chdir.File)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chdir.SyscallContext)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chmod.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chmod.SyscallContext)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File)
		}
		_ = ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown)
		_ = ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown)
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chown.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chown.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chown.SyscallContext)
		}
	case "connect":
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
		if ev.Exec.Process.IsNotKworker() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.Exec.Process.CGroup)
		_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Exec.Process.CGroup)
		_ = ev.FieldHandlers.ResolveProcessContainerID(ev, ev.Exec.Process)
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
		if ev.Exec.Process.HasInterpreter() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exec.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exec.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exec.Process.UserSession)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process)
		if !forADs {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process)
		}
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process)
		_ = ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exec.Process)
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Exec.SyscallContext)
		}
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
		if ev.Exit.Process.IsNotKworker() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.Exit.Process.CGroup)
		_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Exit.Process.CGroup)
		_ = ev.FieldHandlers.ResolveProcessContainerID(ev, ev.Exit.Process)
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
		if ev.Exit.Process.HasInterpreter() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exit.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exit.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exit.Process.UserSession)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process)
		if !forADs {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process)
		}
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process)
		_ = ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exit.Process)
	case "imds":
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source)
		}
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target)
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Link.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Link.SyscallContext)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File)
		}
	case "mount":
		_ = ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount)
		_ = ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount)
		_ = ev.FieldHandlers.ResolveMountRootPath(ev, &ev.Mount)
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mount.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Mount.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Mount.SyscallContext)
		}
	case "mprotect":
	case "ondemand":
		_ = ev.FieldHandlers.ResolveOnDemandName(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg1Str(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg1Uint(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg2Str(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg2Uint(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg3Str(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg3Uint(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg4Str(ev, &ev.OnDemand)
		_ = ev.FieldHandlers.ResolveOnDemandArg4Uint(ev, &ev.OnDemand)
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Open.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Open.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Open.SyscallContext)
		}
	case "packet":
		_ = ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.RawPacket.NetworkContext.Device)
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
		if ev.PTrace.Tracee.Process.IsNotKworker() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.PTrace.Tracee.Process.CGroup)
		_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.PTrace.Tracee.Process.CGroup)
		_ = ev.FieldHandlers.ResolveProcessContainerID(ev, &ev.PTrace.Tracee.Process)
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
		if ev.PTrace.Tracee.Process.HasInterpreter() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Process.UserSession)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process)
		if !forADs {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process)
		}
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process)
		_ = ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.PTrace.Tracee.Process)
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
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.IsNotKworker() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.FileEvent)
			}
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.PTrace.Tracee.Parent.CGroup)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.PTrace.Tracee.Parent.CGroup)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessContainerID(ev, ev.PTrace.Tracee.Parent)
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
		if ev.PTrace.Tracee.HasParent() && ev.PTrace.Tracee.Parent.HasInterpreter() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent)
			}
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Parent.UserSession)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Parent.UserSession)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Parent.UserSession)
		}
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent)
		}
		if ev.PTrace.Tracee.HasParent() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent)
			}
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
		if ev.PTrace.Tracee.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessIsThread(ev, ev.PTrace.Tracee.Parent)
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old)
		}
		_ = ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields)
		_ = ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields)
		_ = ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New)
		_ = ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New)
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rename.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Rename.SyscallContext)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File)
		}
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
		if ev.Signal.Target.Process.IsNotKworker() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.Signal.Target.Process.CGroup)
		_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Signal.Target.Process.CGroup)
		_ = ev.FieldHandlers.ResolveProcessContainerID(ev, &ev.Signal.Target.Process)
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
		if ev.Signal.Target.Process.HasInterpreter() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent)
			}
		}
		_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Process.UserSession)
		_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Process.UserSession)
		_ = ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process)
		if !forADs {
			_ = ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process)
		}
		_ = ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process)
		_ = ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.Signal.Target.Process)
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
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.IsNotKworker() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.FileEvent)
			}
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveCGroupID(ev, &ev.Signal.Target.Parent.CGroup)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveCGroupManager(ev, &ev.Signal.Target.Parent.CGroup)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessContainerID(ev, ev.Signal.Target.Parent)
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
		if ev.Signal.Target.HasParent() && ev.Signal.Target.Parent.HasInterpreter() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent)
			}
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Parent.UserSession)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Parent.UserSession)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Parent.UserSession)
		}
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent)
		}
		if ev.Signal.Target.HasParent() {
			if !forADs {
				_ = ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent)
			}
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
		if ev.Signal.Target.HasParent() {
			_ = ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Signal.Target.Parent)
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Unlink.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Unlink.SyscallContext)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Unlink.SyscallContext)
		}
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
		if !forADs {
			_ = ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File)
		}
		if !forADs {
			_ = ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Utimes.SyscallContext)
		}
	}
}

type FieldHandlers interface {
	ResolveAsync(ev *Event) bool
	ResolveCGroupID(ev *Event, e *CGroupContext) string
	ResolveCGroupManager(ev *Event, e *CGroupContext) string
	ResolveChownGID(ev *Event, e *ChownEvent) string
	ResolveChownUID(ev *Event, e *ChownEvent) string
	ResolveContainerCreatedAt(ev *Event, e *ContainerContext) int
	ResolveContainerID(ev *Event, e *ContainerContext) string
	ResolveContainerRuntime(ev *Event, e *ContainerContext) string
	ResolveContainerTags(ev *Event, e *ContainerContext) []string
	ResolveEventTime(ev *Event, e *BaseEvent) time.Time
	ResolveEventTimestamp(ev *Event, e *BaseEvent) int
	ResolveFileBasename(ev *Event, e *FileEvent) string
	ResolveFileFieldsGroup(ev *Event, e *FileFields) string
	ResolveFileFieldsInUpperLayer(ev *Event, e *FileFields) bool
	ResolveFileFieldsUser(ev *Event, e *FileFields) string
	ResolveFileFilesystem(ev *Event, e *FileEvent) string
	ResolveFilePath(ev *Event, e *FileEvent) string
	ResolveHashesFromEvent(ev *Event, e *FileEvent) []string
	ResolveHostname(ev *Event, e *BaseEvent) string
	ResolveK8SGroups(ev *Event, e *UserSessionContext) []string
	ResolveK8SUID(ev *Event, e *UserSessionContext) string
	ResolveK8SUsername(ev *Event, e *UserSessionContext) string
	ResolveModuleArgs(ev *Event, e *LoadModuleEvent) string
	ResolveModuleArgv(ev *Event, e *LoadModuleEvent) []string
	ResolveMountPointPath(ev *Event, e *MountEvent) string
	ResolveMountRootPath(ev *Event, e *MountEvent) string
	ResolveMountSourcePath(ev *Event, e *MountEvent) string
	ResolveNetworkDeviceIfName(ev *Event, e *NetworkDeviceContext) string
	ResolveOnDemandArg1Str(ev *Event, e *OnDemandEvent) string
	ResolveOnDemandArg1Uint(ev *Event, e *OnDemandEvent) int
	ResolveOnDemandArg2Str(ev *Event, e *OnDemandEvent) string
	ResolveOnDemandArg2Uint(ev *Event, e *OnDemandEvent) int
	ResolveOnDemandArg3Str(ev *Event, e *OnDemandEvent) string
	ResolveOnDemandArg3Uint(ev *Event, e *OnDemandEvent) int
	ResolveOnDemandArg4Str(ev *Event, e *OnDemandEvent) string
	ResolveOnDemandArg4Uint(ev *Event, e *OnDemandEvent) int
	ResolveOnDemandName(ev *Event, e *OnDemandEvent) string
	ResolvePackageName(ev *Event, e *FileEvent) string
	ResolvePackageSourceVersion(ev *Event, e *FileEvent) string
	ResolvePackageVersion(ev *Event, e *FileEvent) string
	ResolveProcessArgs(ev *Event, e *Process) string
	ResolveProcessArgsFlags(ev *Event, e *Process) []string
	ResolveProcessArgsOptions(ev *Event, e *Process) []string
	ResolveProcessArgsScrubbed(ev *Event, e *Process) string
	ResolveProcessArgsTruncated(ev *Event, e *Process) bool
	ResolveProcessArgv(ev *Event, e *Process) []string
	ResolveProcessArgv0(ev *Event, e *Process) string
	ResolveProcessArgvScrubbed(ev *Event, e *Process) []string
	ResolveProcessCmdArgv(ev *Event, e *Process) []string
	ResolveProcessContainerID(ev *Event, e *Process) string
	ResolveProcessCreatedAt(ev *Event, e *Process) int
	ResolveProcessEnvp(ev *Event, e *Process) []string
	ResolveProcessEnvs(ev *Event, e *Process) []string
	ResolveProcessEnvsTruncated(ev *Event, e *Process) bool
	ResolveProcessIsThread(ev *Event, e *Process) bool
	ResolveRights(ev *Event, e *FileFields) int
	ResolveSELinuxBoolName(ev *Event, e *SELinuxEvent) string
	ResolveService(ev *Event, e *BaseEvent) string
	ResolveSetgidEGroup(ev *Event, e *SetgidEvent) string
	ResolveSetgidFSGroup(ev *Event, e *SetgidEvent) string
	ResolveSetgidGroup(ev *Event, e *SetgidEvent) string
	ResolveSetuidEUser(ev *Event, e *SetuidEvent) string
	ResolveSetuidFSUser(ev *Event, e *SetuidEvent) string
	ResolveSetuidUser(ev *Event, e *SetuidEvent) string
	ResolveSyscallCtxArgsInt1(ev *Event, e *SyscallContext) int
	ResolveSyscallCtxArgsInt2(ev *Event, e *SyscallContext) int
	ResolveSyscallCtxArgsInt3(ev *Event, e *SyscallContext) int
	ResolveSyscallCtxArgsStr1(ev *Event, e *SyscallContext) string
	ResolveSyscallCtxArgsStr2(ev *Event, e *SyscallContext) string
	ResolveSyscallCtxArgsStr3(ev *Event, e *SyscallContext) string
	ResolveXAttrName(ev *Event, e *SetXAttrEvent) string
	ResolveXAttrNamespace(ev *Event, e *SetXAttrEvent) string
	// custom handlers not tied to any fields
	ExtraFieldHandlers
}
type FakeFieldHandlers struct{}

func (dfh *FakeFieldHandlers) ResolveAsync(ev *Event) bool { return bool(ev.Async) }
func (dfh *FakeFieldHandlers) ResolveCGroupID(ev *Event, e *CGroupContext) string {
	return string(e.CGroupID)
}
func (dfh *FakeFieldHandlers) ResolveCGroupManager(ev *Event, e *CGroupContext) string {
	return string(e.CGroupManager)
}
func (dfh *FakeFieldHandlers) ResolveChownGID(ev *Event, e *ChownEvent) string {
	return string(e.Group)
}
func (dfh *FakeFieldHandlers) ResolveChownUID(ev *Event, e *ChownEvent) string { return string(e.User) }
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
func (dfh *FakeFieldHandlers) ResolveFileFieldsGroup(ev *Event, e *FileFields) string {
	return string(e.Group)
}
func (dfh *FakeFieldHandlers) ResolveFileFieldsInUpperLayer(ev *Event, e *FileFields) bool {
	return bool(e.InUpperLayer)
}
func (dfh *FakeFieldHandlers) ResolveFileFieldsUser(ev *Event, e *FileFields) string {
	return string(e.User)
}
func (dfh *FakeFieldHandlers) ResolveFileFilesystem(ev *Event, e *FileEvent) string {
	return string(e.Filesystem)
}
func (dfh *FakeFieldHandlers) ResolveFilePath(ev *Event, e *FileEvent) string {
	return string(e.PathnameStr)
}
func (dfh *FakeFieldHandlers) ResolveHashesFromEvent(ev *Event, e *FileEvent) []string {
	return []string(e.Hashes)
}
func (dfh *FakeFieldHandlers) ResolveHostname(ev *Event, e *BaseEvent) string {
	return string(e.Hostname)
}
func (dfh *FakeFieldHandlers) ResolveK8SGroups(ev *Event, e *UserSessionContext) []string {
	return []string(e.K8SGroups)
}
func (dfh *FakeFieldHandlers) ResolveK8SUID(ev *Event, e *UserSessionContext) string {
	return string(e.K8SUID)
}
func (dfh *FakeFieldHandlers) ResolveK8SUsername(ev *Event, e *UserSessionContext) string {
	return string(e.K8SUsername)
}
func (dfh *FakeFieldHandlers) ResolveModuleArgs(ev *Event, e *LoadModuleEvent) string {
	return string(e.Args)
}
func (dfh *FakeFieldHandlers) ResolveModuleArgv(ev *Event, e *LoadModuleEvent) []string {
	return []string(e.Argv)
}
func (dfh *FakeFieldHandlers) ResolveMountPointPath(ev *Event, e *MountEvent) string {
	return string(e.MountPointPath)
}
func (dfh *FakeFieldHandlers) ResolveMountRootPath(ev *Event, e *MountEvent) string {
	return string(e.MountRootPath)
}
func (dfh *FakeFieldHandlers) ResolveMountSourcePath(ev *Event, e *MountEvent) string {
	return string(e.MountSourcePath)
}
func (dfh *FakeFieldHandlers) ResolveNetworkDeviceIfName(ev *Event, e *NetworkDeviceContext) string {
	return string(e.IfName)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg1Str(ev *Event, e *OnDemandEvent) string {
	return string(e.Arg1Str)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg1Uint(ev *Event, e *OnDemandEvent) int {
	return int(e.Arg1Uint)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg2Str(ev *Event, e *OnDemandEvent) string {
	return string(e.Arg2Str)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg2Uint(ev *Event, e *OnDemandEvent) int {
	return int(e.Arg2Uint)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg3Str(ev *Event, e *OnDemandEvent) string {
	return string(e.Arg3Str)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg3Uint(ev *Event, e *OnDemandEvent) int {
	return int(e.Arg3Uint)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg4Str(ev *Event, e *OnDemandEvent) string {
	return string(e.Arg4Str)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandArg4Uint(ev *Event, e *OnDemandEvent) int {
	return int(e.Arg4Uint)
}
func (dfh *FakeFieldHandlers) ResolveOnDemandName(ev *Event, e *OnDemandEvent) string {
	return string(e.Name)
}
func (dfh *FakeFieldHandlers) ResolvePackageName(ev *Event, e *FileEvent) string {
	return string(e.PkgName)
}
func (dfh *FakeFieldHandlers) ResolvePackageSourceVersion(ev *Event, e *FileEvent) string {
	return string(e.PkgSrcVersion)
}
func (dfh *FakeFieldHandlers) ResolvePackageVersion(ev *Event, e *FileEvent) string {
	return string(e.PkgVersion)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgs(ev *Event, e *Process) string { return string(e.Args) }
func (dfh *FakeFieldHandlers) ResolveProcessArgsFlags(ev *Event, e *Process) []string {
	return []string(e.Argv)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgsOptions(ev *Event, e *Process) []string {
	return []string(e.Argv)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgsScrubbed(ev *Event, e *Process) string {
	return string(e.ArgsScrubbed)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgsTruncated(ev *Event, e *Process) bool {
	return bool(e.ArgsTruncated)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgv(ev *Event, e *Process) []string {
	return []string(e.Argv)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgv0(ev *Event, e *Process) string {
	return string(e.Argv0)
}
func (dfh *FakeFieldHandlers) ResolveProcessArgvScrubbed(ev *Event, e *Process) []string {
	return []string(e.ArgvScrubbed)
}
func (dfh *FakeFieldHandlers) ResolveProcessCmdArgv(ev *Event, e *Process) []string {
	return []string(e.Argv)
}
func (dfh *FakeFieldHandlers) ResolveProcessContainerID(ev *Event, e *Process) string {
	return string(e.ContainerID)
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
func (dfh *FakeFieldHandlers) ResolveProcessEnvsTruncated(ev *Event, e *Process) bool {
	return bool(e.EnvsTruncated)
}
func (dfh *FakeFieldHandlers) ResolveProcessIsThread(ev *Event, e *Process) bool {
	return bool(e.IsThread)
}
func (dfh *FakeFieldHandlers) ResolveRights(ev *Event, e *FileFields) int { return int(e.Mode) }
func (dfh *FakeFieldHandlers) ResolveSELinuxBoolName(ev *Event, e *SELinuxEvent) string {
	return string(e.BoolName)
}
func (dfh *FakeFieldHandlers) ResolveService(ev *Event, e *BaseEvent) string {
	return string(e.Service)
}
func (dfh *FakeFieldHandlers) ResolveSetgidEGroup(ev *Event, e *SetgidEvent) string {
	return string(e.EGroup)
}
func (dfh *FakeFieldHandlers) ResolveSetgidFSGroup(ev *Event, e *SetgidEvent) string {
	return string(e.FSGroup)
}
func (dfh *FakeFieldHandlers) ResolveSetgidGroup(ev *Event, e *SetgidEvent) string {
	return string(e.Group)
}
func (dfh *FakeFieldHandlers) ResolveSetuidEUser(ev *Event, e *SetuidEvent) string {
	return string(e.EUser)
}
func (dfh *FakeFieldHandlers) ResolveSetuidFSUser(ev *Event, e *SetuidEvent) string {
	return string(e.FSUser)
}
func (dfh *FakeFieldHandlers) ResolveSetuidUser(ev *Event, e *SetuidEvent) string {
	return string(e.User)
}
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgsInt1(ev *Event, e *SyscallContext) int {
	return int(e.IntArg1)
}
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgsInt2(ev *Event, e *SyscallContext) int {
	return int(e.IntArg2)
}
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgsInt3(ev *Event, e *SyscallContext) int {
	return int(e.IntArg3)
}
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgsStr1(ev *Event, e *SyscallContext) string {
	return string(e.StrArg1)
}
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgsStr2(ev *Event, e *SyscallContext) string {
	return string(e.StrArg2)
}
func (dfh *FakeFieldHandlers) ResolveSyscallCtxArgsStr3(ev *Event, e *SyscallContext) string {
	return string(e.StrArg3)
}
func (dfh *FakeFieldHandlers) ResolveXAttrName(ev *Event, e *SetXAttrEvent) string {
	return string(e.Name)
}
func (dfh *FakeFieldHandlers) ResolveXAttrNamespace(ev *Event, e *SetXAttrEvent) string {
	return string(e.Namespace)
}
