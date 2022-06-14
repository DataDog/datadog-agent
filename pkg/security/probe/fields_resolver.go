//go:build linux
// +build linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.
package probe

// ResolveFields resolves all the fields associate to the event type. Context fields are automatically resolved.
func (ev *Event) ResolveFields() {
	// resolve context fields that are not related to any event type
	_ = ev.ResolveContainerID(&ev.ContainerContext)
	_ = ev.ResolveContainerTags(&ev.ContainerContext)
	_ = ev.ResolveNetworkDeviceIfName(&ev.NetworkContext.Device)
	_ = ev.ResolveProcessArgs(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessArgsTruncated(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessArgv(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessArgv0(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessCreatedAt(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessEnvp(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessEnvs(&ev.ProcessContext.Process)
	_ = ev.ResolveProcessEnvsTruncated(&ev.ProcessContext.Process)
	_ = ev.ResolveFileFilesystem(&ev.ProcessContext.Process.FileEvent)
	_ = ev.ResolveFileFieldsGroup(&ev.ProcessContext.Process.FileEvent.FileFields)
	_ = ev.ResolveFileFieldsInUpperLayer(&ev.ProcessContext.Process.FileEvent.FileFields)
	_ = ev.ResolveFileBasename(&ev.ProcessContext.Process.FileEvent)
	_ = ev.ResolveFilePath(&ev.ProcessContext.Process.FileEvent)
	_ = ev.ResolveFileFieldsUser(&ev.ProcessContext.Process.FileEvent.FileFields)
	// resolve event specific fields
	switch ev.GetEventType().String() {
	case "bind":
	case "bpf":
		_ = ev.ResolveHelpers(&ev.BPF.Program)
	case "capset":
	case "chmod":
		_ = ev.ResolveFileFieldsUser(&ev.Chmod.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Chmod.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Chmod.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Chmod.File)
		_ = ev.ResolveFileBasename(&ev.Chmod.File)
		_ = ev.ResolveFileFilesystem(&ev.Chmod.File)
	case "chown":
		_ = ev.ResolveFileFieldsUser(&ev.Chown.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Chown.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Chown.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Chown.File)
		_ = ev.ResolveFileBasename(&ev.Chown.File)
		_ = ev.ResolveFileFilesystem(&ev.Chown.File)
		_ = ev.ResolveChownUID(&ev.Chown)
		_ = ev.ResolveChownGID(&ev.Chown)
	case "dns":
	case "exec":
		_ = ev.ResolveFileFieldsUser(&ev.Exec.Process.FileEvent.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Exec.Process.FileEvent.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Exec.Process.FileEvent.FileFields)
		_ = ev.ResolveFilePath(&ev.Exec.Process.FileEvent)
		_ = ev.ResolveFileBasename(&ev.Exec.Process.FileEvent)
		_ = ev.ResolveFileFilesystem(&ev.Exec.Process.FileEvent)
		_ = ev.ResolveProcessCreatedAt(ev.Exec.Process)
		_ = ev.ResolveProcessArgv0(ev.Exec.Process)
		_ = ev.ResolveProcessArgs(ev.Exec.Process)
		_ = ev.ResolveProcessArgv(ev.Exec.Process)
		_ = ev.ResolveProcessArgsTruncated(ev.Exec.Process)
		_ = ev.ResolveProcessEnvs(ev.Exec.Process)
		_ = ev.ResolveProcessEnvp(ev.Exec.Process)
		_ = ev.ResolveProcessEnvsTruncated(ev.Exec.Process)
	case "link":
		_ = ev.ResolveFileFieldsUser(&ev.Link.Source.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Link.Source.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Link.Source.FileFields)
		_ = ev.ResolveFilePath(&ev.Link.Source)
		_ = ev.ResolveFileBasename(&ev.Link.Source)
		_ = ev.ResolveFileFilesystem(&ev.Link.Source)
		_ = ev.ResolveFileFieldsUser(&ev.Link.Target.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Link.Target.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Link.Target.FileFields)
		_ = ev.ResolveFilePath(&ev.Link.Target)
		_ = ev.ResolveFileBasename(&ev.Link.Target)
		_ = ev.ResolveFileFilesystem(&ev.Link.Target)
	case "load_module":
		_ = ev.ResolveFileFieldsUser(&ev.LoadModule.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.LoadModule.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.LoadModule.File.FileFields)
		_ = ev.ResolveFilePath(&ev.LoadModule.File)
		_ = ev.ResolveFileBasename(&ev.LoadModule.File)
		_ = ev.ResolveFileFilesystem(&ev.LoadModule.File)
	case "mkdir":
		_ = ev.ResolveFileFieldsUser(&ev.Mkdir.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Mkdir.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Mkdir.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Mkdir.File)
		_ = ev.ResolveFileBasename(&ev.Mkdir.File)
		_ = ev.ResolveFileFilesystem(&ev.Mkdir.File)
	case "mmap":
		_ = ev.ResolveFileFieldsUser(&ev.MMap.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.MMap.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.MMap.File.FileFields)
		_ = ev.ResolveFilePath(&ev.MMap.File)
		_ = ev.ResolveFileBasename(&ev.MMap.File)
		_ = ev.ResolveFileFilesystem(&ev.MMap.File)
	case "mprotect":
	case "open":
		_ = ev.ResolveFileFieldsUser(&ev.Open.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Open.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Open.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Open.File)
		_ = ev.ResolveFileBasename(&ev.Open.File)
		_ = ev.ResolveFileFilesystem(&ev.Open.File)
	case "ptrace":
		_ = ev.ResolveFileFieldsUser(&ev.PTrace.Tracee.Process.FileEvent.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.PTrace.Tracee.Process.FileEvent.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.PTrace.Tracee.Process.FileEvent.FileFields)
		_ = ev.ResolveFilePath(&ev.PTrace.Tracee.Process.FileEvent)
		_ = ev.ResolveFileBasename(&ev.PTrace.Tracee.Process.FileEvent)
		_ = ev.ResolveFileFilesystem(&ev.PTrace.Tracee.Process.FileEvent)
		_ = ev.ResolveProcessCreatedAt(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessArgv0(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessArgs(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessArgv(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessArgsTruncated(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessEnvs(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessEnvp(&ev.PTrace.Tracee.Process)
		_ = ev.ResolveProcessEnvsTruncated(&ev.PTrace.Tracee.Process)
	case "removexattr":
		_ = ev.ResolveFileFieldsUser(&ev.RemoveXAttr.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.RemoveXAttr.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.RemoveXAttr.File.FileFields)
		_ = ev.ResolveFilePath(&ev.RemoveXAttr.File)
		_ = ev.ResolveFileBasename(&ev.RemoveXAttr.File)
		_ = ev.ResolveFileFilesystem(&ev.RemoveXAttr.File)
		_ = ev.ResolveXAttrNamespace(&ev.RemoveXAttr)
		_ = ev.ResolveXAttrName(&ev.RemoveXAttr)
	case "rename":
		_ = ev.ResolveFileFieldsUser(&ev.Rename.Old.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Rename.Old.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Rename.Old.FileFields)
		_ = ev.ResolveFilePath(&ev.Rename.Old)
		_ = ev.ResolveFileBasename(&ev.Rename.Old)
		_ = ev.ResolveFileFilesystem(&ev.Rename.Old)
		_ = ev.ResolveFileFieldsUser(&ev.Rename.New.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Rename.New.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Rename.New.FileFields)
		_ = ev.ResolveFilePath(&ev.Rename.New)
		_ = ev.ResolveFileBasename(&ev.Rename.New)
		_ = ev.ResolveFileFilesystem(&ev.Rename.New)
	case "rmdir":
		_ = ev.ResolveFileFieldsUser(&ev.Rmdir.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Rmdir.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Rmdir.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Rmdir.File)
		_ = ev.ResolveFileBasename(&ev.Rmdir.File)
		_ = ev.ResolveFileFilesystem(&ev.Rmdir.File)
	case "selinux":
		_ = ev.ResolveSELinuxBoolName(&ev.SELinux)
	case "setgid":
		_ = ev.ResolveSetgidGroup(&ev.SetGID)
		_ = ev.ResolveSetgidEGroup(&ev.SetGID)
		_ = ev.ResolveSetgidFSGroup(&ev.SetGID)
	case "setuid":
		_ = ev.ResolveSetuidUser(&ev.SetUID)
		_ = ev.ResolveSetuidEUser(&ev.SetUID)
		_ = ev.ResolveSetuidFSUser(&ev.SetUID)
	case "setxattr":
		_ = ev.ResolveFileFieldsUser(&ev.SetXAttr.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.SetXAttr.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.SetXAttr.File.FileFields)
		_ = ev.ResolveFilePath(&ev.SetXAttr.File)
		_ = ev.ResolveFileBasename(&ev.SetXAttr.File)
		_ = ev.ResolveFileFilesystem(&ev.SetXAttr.File)
		_ = ev.ResolveXAttrNamespace(&ev.SetXAttr)
		_ = ev.ResolveXAttrName(&ev.SetXAttr)
	case "signal":
		_ = ev.ResolveFileFieldsUser(&ev.Signal.Target.Process.FileEvent.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Signal.Target.Process.FileEvent.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Signal.Target.Process.FileEvent.FileFields)
		_ = ev.ResolveFilePath(&ev.Signal.Target.Process.FileEvent)
		_ = ev.ResolveFileBasename(&ev.Signal.Target.Process.FileEvent)
		_ = ev.ResolveFileFilesystem(&ev.Signal.Target.Process.FileEvent)
		_ = ev.ResolveProcessCreatedAt(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessArgv0(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessArgs(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessArgv(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessArgsTruncated(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessEnvs(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessEnvp(&ev.Signal.Target.Process)
		_ = ev.ResolveProcessEnvsTruncated(&ev.Signal.Target.Process)
	case "splice":
		_ = ev.ResolveFileFieldsUser(&ev.Splice.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Splice.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Splice.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Splice.File)
		_ = ev.ResolveFileBasename(&ev.Splice.File)
		_ = ev.ResolveFileFilesystem(&ev.Splice.File)
	case "unlink":
		_ = ev.ResolveFileFieldsUser(&ev.Unlink.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Unlink.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Unlink.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Unlink.File)
		_ = ev.ResolveFileBasename(&ev.Unlink.File)
		_ = ev.ResolveFileFilesystem(&ev.Unlink.File)
	case "unload_module":
	case "utimes":
		_ = ev.ResolveFileFieldsUser(&ev.Utimes.File.FileFields)
		_ = ev.ResolveFileFieldsGroup(&ev.Utimes.File.FileFields)
		_ = ev.ResolveFileFieldsInUpperLayer(&ev.Utimes.File.FileFields)
		_ = ev.ResolveFilePath(&ev.Utimes.File)
		_ = ev.ResolveFileBasename(&ev.Utimes.File)
		_ = ev.ResolveFileFilesystem(&ev.Utimes.File)
	}
}
