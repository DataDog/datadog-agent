// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/args"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// EBPFFieldHandlers defines a field handlers
type EBPFFieldHandlers struct {
	config    *config.Config
	resolvers *resolvers.EBPFResolvers
	hostname  string
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFFieldHandlers) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.PIDContext.IsKworker {
		return model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, true), false
	}

	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid, ev.PIDContext.ExecInode, true)
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, false)
		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// ResolveFilePath resolves the inode to a full path
func (fh *EBPFFieldHandlers) ResolveFilePath(ev *model.Event, f *model.FileEvent) string {
	if !f.IsPathnameStrResolved && len(f.PathnameStr) == 0 {
		path, mountPath, source, origin, err := fh.resolvers.PathResolver.ResolveFileFieldsPath(&f.FileFields, &ev.PIDContext, ev.ContainerContext)
		if err != nil {
			ev.SetPathResolutionError(f, err)
		}
		f.SetPathnameStr(path)
		f.MountPath = mountPath
		f.MountSource = source
		f.MountOrigin = origin
	}

	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (fh *EBPFFieldHandlers) ResolveFileBasename(_ *model.Event, f *model.FileEvent) string {
	if !f.IsBasenameStrResolved && len(f.BasenameStr) == 0 {
		if f.PathnameStr != "" {
			f.SetBasenameStr(path.Base(f.PathnameStr))
		} else {
			f.SetBasenameStr(fh.resolvers.PathResolver.ResolveBasename(&f.FileFields))
		}
	}
	return f.BasenameStr
}

// ResolveFileFilesystem resolves the filesystem a file resides in
func (fh *EBPFFieldHandlers) ResolveFileFilesystem(ev *model.Event, f *model.FileEvent) string {
	if f.Filesystem == "" {
		if f.IsFileless() {
			f.Filesystem = model.TmpFS
		} else {
			fs, err := fh.resolvers.MountResolver.ResolveFilesystem(f.FileFields.MountID, f.FileFields.Device, ev.PIDContext.Pid, ev.ContainerContext.ID)
			if err != nil {
				ev.SetPathResolutionError(f, err)
			}
			f.Filesystem = fs
		}
	}
	return f.Filesystem
}

// ResolveProcessArgsFlags resolves the arguments flags of the event
func (fh *EBPFFieldHandlers) ResolveProcessArgsFlags(ev *model.Event, process *model.Process) (flags []string) {
	return args.ParseProcessFlags(fh.ResolveProcessArgv(ev, process))
}

// ResolveProcessArgsOptions resolves the arguments options of the event
func (fh *EBPFFieldHandlers) ResolveProcessArgsOptions(ev *model.Event, process *model.Process) (options []string) {
	return args.ParseProcessOptions(fh.ResolveProcessArgv(ev, process))
}

// ResolveFileFieldsInUpperLayer resolves whether the file is in an upper layer
func (fh *EBPFFieldHandlers) ResolveFileFieldsInUpperLayer(_ *model.Event, f *model.FileFields) bool {
	return f.GetInUpperLayer()
}

// ResolveXAttrName returns the string representation of the extended attribute name
func (fh *EBPFFieldHandlers) ResolveXAttrName(_ *model.Event, e *model.SetXAttrEvent) string {
	if len(e.Name) == 0 {
		e.Name, _ = model.UnmarshalString(e.NameRaw[:], 200)
	}
	return e.Name
}

// ResolveXAttrNamespace returns the string representation of the extended attribute namespace
func (fh *EBPFFieldHandlers) ResolveXAttrNamespace(ev *model.Event, e *model.SetXAttrEvent) string {
	if len(e.Namespace) == 0 {
		ns, _, found := strings.Cut(fh.ResolveXAttrName(ev, e), ".")
		if found {
			e.Namespace = ns
		}
	}
	return e.Namespace
}

// ResolveMountPointPath resolves a mount point path
func (fh *EBPFFieldHandlers) ResolveMountPointPath(ev *model.Event, e *model.MountEvent) string {
	if len(e.MountPointPath) == 0 {
		mountPointPath, _, _, err := fh.resolvers.MountResolver.ResolveMountPath(e.MountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			e.MountPointPathResolutionError = err
			return ""
		}
		e.MountPointPath = mountPointPath
	}
	return e.MountPointPath
}

// ResolveMountSourcePath resolves a mount source path
func (fh *EBPFFieldHandlers) ResolveMountSourcePath(ev *model.Event, e *model.MountEvent) string {
	if e.BindSrcMountID != 0 && len(e.MountSourcePath) == 0 {
		bindSourceMountPath, _, _, err := fh.resolvers.MountResolver.ResolveMountPath(e.BindSrcMountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			e.MountSourcePathResolutionError = err
			return ""
		}
		rootStr, err := fh.resolvers.PathResolver.ResolveMountRoot(ev, &e.Mount)
		if err != nil {
			e.MountSourcePathResolutionError = err
			return ""
		}
		e.MountSourcePath = path.Join(bindSourceMountPath, rootStr)
	}
	return e.MountSourcePath
}

// ResolveMountRootPath resolves a mount root path
func (fh *EBPFFieldHandlers) ResolveMountRootPath(ev *model.Event, e *model.MountEvent) string {
	if len(e.MountRootPath) == 0 {
		mountRootPath, _, _, err := fh.resolvers.MountResolver.ResolveMountRoot(e.MountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			e.MountRootPathResolutionError = err
			return ""
		}
		e.MountRootPath = mountRootPath
	}
	return e.MountRootPath
}

// ResolveContainerContext queries the cgroup resolver to retrieve the ContainerContext of the event
func (fh *EBPFFieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	if ev.ContainerContext.ID != "" && !ev.ContainerContext.Resolved {
		if containerContext, _ := fh.resolvers.CGroupResolver.GetWorkload(ev.ContainerContext.ID); containerContext != nil {
			ev.ContainerContext = &containerContext.ContainerContext
			ev.ContainerContext.Resolved = true
		}
	}
	return ev.ContainerContext, ev.ContainerContext != nil
}

// ResolveRights resolves the rights of a file
func (fh *EBPFFieldHandlers) ResolveRights(_ *model.Event, e *model.FileFields) int {
	return int(e.Mode) & (syscall.S_ISUID | syscall.S_ISGID | syscall.S_ISVTX | syscall.S_IRWXU | syscall.S_IRWXG | syscall.S_IRWXO)
}

// ResolveChownUID resolves the ResolveProcessCacheEntry id of a chown event to a username
func (fh *EBPFFieldHandlers) ResolveChownUID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID), ev.ContainerContext.ID)
	}
	return e.User
}

// ResolveChownGID resolves the group id of a chown event to a group name
func (fh *EBPFFieldHandlers) ResolveChownGID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveGroup(int(e.GID), ev.ContainerContext.ID)
	}
	return e.Group
}

// ResolveProcessArgv0 resolves the first arg of the event
func (fh *EBPFFieldHandlers) ResolveProcessArgv0(_ *model.Event, process *model.Process) string {
	arg0, _ := sprocess.GetProcessArgv0(process)
	return arg0
}

// ResolveProcessArgs resolves the args of the event
func (fh *EBPFFieldHandlers) ResolveProcessArgs(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgv(ev, process), " ")
}

// ResolveProcessArgsScrubbed resolves the args of the event
func (fh *EBPFFieldHandlers) ResolveProcessArgsScrubbed(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgvScrubbed(ev, process), " ")
}

// ResolveProcessArgv resolves the unscrubbed args of the process as an array. Use with caution.
func (fh *EBPFFieldHandlers) ResolveProcessArgv(_ *model.Event, process *model.Process) []string {
	argv, _ := sprocess.GetProcessArgv(process)
	return argv
}

// ResolveProcessArgvScrubbed resolves the args of the process as an array
func (fh *EBPFFieldHandlers) ResolveProcessArgvScrubbed(_ *model.Event, process *model.Process) []string {
	argv, _ := fh.resolvers.ProcessResolver.GetProcessArgvScrubbed(process)
	return argv
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (fh *EBPFFieldHandlers) ResolveProcessEnvp(_ *model.Event, process *model.Process) []string {
	envp, _ := fh.resolvers.ProcessResolver.GetProcessEnvp(process)
	return envp
}

// ResolveProcessArgsTruncated returns whether the args are truncated
func (fh *EBPFFieldHandlers) ResolveProcessArgsTruncated(_ *model.Event, process *model.Process) bool {
	_, truncated := sprocess.GetProcessArgv(process)
	return truncated
}

// ResolveProcessEnvsTruncated returns whether the envs are truncated
func (fh *EBPFFieldHandlers) ResolveProcessEnvsTruncated(_ *model.Event, process *model.Process) bool {
	_, truncated := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return truncated
}

// ResolveProcessEnvs resolves the unscrubbed envs of the event. Use with caution.
func (fh *EBPFFieldHandlers) ResolveProcessEnvs(_ *model.Event, process *model.Process) []string {
	envs, _ := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return envs
}

// ResolveSetuidUser resolves the user of the Setuid event
func (fh *EBPFFieldHandlers) ResolveSetuidUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID), ev.ContainerContext.ID)
	}
	return e.User
}

// ResolveSetuidEUser resolves the effective user of the Setuid event
func (fh *EBPFFieldHandlers) ResolveSetuidEUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.EUser) == 0 {
		e.EUser, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.EUID), ev.ContainerContext.ID)
	}
	return e.EUser
}

// ResolveSetuidFSUser resolves the file-system user of the Setuid event
func (fh *EBPFFieldHandlers) ResolveSetuidFSUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.FSUser) == 0 {
		e.FSUser, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.FSUID), ev.ContainerContext.ID)
	}
	return e.FSUser
}

// ResolveSetgidGroup resolves the group of the Setgid event
func (fh *EBPFFieldHandlers) ResolveSetgidGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.GID), ev.ContainerContext.ID)
	}
	return e.Group
}

// ResolveSetgidEGroup resolves the effective group of the Setgid event
func (fh *EBPFFieldHandlers) ResolveSetgidEGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.EGroup) == 0 {
		e.EGroup, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.EGID), ev.ContainerContext.ID)
	}
	return e.EGroup
}

// ResolveSetgidFSGroup resolves the file-system group of the Setgid event
func (fh *EBPFFieldHandlers) ResolveSetgidFSGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.FSGroup) == 0 {
		e.FSGroup, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.FSGID), ev.ContainerContext.ID)
	}
	return e.FSGroup
}

// ResolveSELinuxBoolName resolves the boolean name of the SELinux event
func (fh *EBPFFieldHandlers) ResolveSELinuxBoolName(_ *model.Event, e *model.SELinuxEvent) string {
	if e.EventKind != model.SELinuxBoolChangeEventKind {
		return ""
	}

	if len(e.BoolName) == 0 {
		e.BoolName = fh.resolvers.PathResolver.ResolveBasename(&e.File.FileFields)
	}
	return e.BoolName
}

// GetProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFFieldHandlers) GetProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid, ev.PIDContext.ExecInode, false)
	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, false)
		return ev.ProcessCacheEntry, false
	}
	return ev.ProcessCacheEntry, true
}

// ResolveFileFieldsGroup resolves the group id of the file to a group name
func (fh *EBPFFieldHandlers) ResolveFileFieldsGroup(ev *model.Event, e *model.FileFields) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveGroup(int(e.GID), ev.ContainerContext.ID)
	}
	return e.Group
}

// ResolveNetworkDeviceIfName returns the network iterface name from the network context
func (fh *EBPFFieldHandlers) ResolveNetworkDeviceIfName(_ *model.Event, device *model.NetworkDeviceContext) string {
	if len(device.IfName) == 0 && fh.resolvers.TCResolver != nil {
		ifName, ok := fh.resolvers.TCResolver.ResolveNetworkDeviceIfName(device.IfIndex, device.NetNS)
		if ok {
			device.IfName = ifName
		}
	}

	return device.IfName
}

// ResolveFileFieldsUser resolves the user id of the file to a username
func (fh *EBPFFieldHandlers) ResolveFileFieldsUser(ev *model.Event, e *model.FileFields) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID), ev.ContainerContext.ID)
	}
	return e.User
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *EBPFFieldHandlers) ResolveEventTimestamp(ev *model.Event, e *model.BaseEvent) int {
	return int(fh.ResolveEventTime(ev, e).UnixNano())
}

// ResolveService returns the service tag based on the process context
func (fh *EBPFFieldHandlers) ResolveService(ev *model.Event, _ *model.BaseEvent) string {
	entry, _ := fh.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}
	return getProcessService(fh.config, entry)
}

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *EBPFFieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		fh := ev.FieldHandlers.(*EBPFFieldHandlers)

		ev.Timestamp = fh.resolvers.TimeResolver.ResolveMonotonicTimestamp(ev.TimestampRaw)
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}
	}
	return ev.Timestamp
}

// ResolveAsync resolves the async flag
func (fh *EBPFFieldHandlers) ResolveAsync(ev *model.Event) bool {
	ev.Async = ev.Flags&model.EventFlagsAsync > 0
	return ev.Async
}

// ResolvePackageName resolves the name of the package providing this file
func (fh *EBPFFieldHandlers) ResolvePackageName(ev *model.Event, f *model.FileEvent) string {
	if f.PkgName == "" {
		// Force the resolution of file path to be able to map to a package provided file
		if fh.ResolveFilePath(ev, f) == "" {
			return ""
		}

		if fh.resolvers.SBOMResolver == nil {
			return ""
		}

		if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ContainerContext.ID, f); pkg != nil {
			f.PkgName = pkg.Name
		}
	}
	return f.PkgName
}

// ResolvePackageVersion resolves the version of the package providing this file
func (fh *EBPFFieldHandlers) ResolvePackageVersion(ev *model.Event, f *model.FileEvent) string {
	if f.PkgVersion == "" {
		// Force the resolution of file path to be able to map to a package provided file
		if fh.ResolveFilePath(ev, f) == "" {
			return ""
		}

		if fh.resolvers.SBOMResolver == nil {
			return ""
		}

		if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ContainerContext.ID, f); pkg != nil {
			f.PkgVersion = pkg.Version
		}
	}
	return f.PkgVersion
}

// ResolvePackageSourceVersion resolves the version of the source package of the package providing this file
func (fh *EBPFFieldHandlers) ResolvePackageSourceVersion(ev *model.Event, f *model.FileEvent) string {
	if f.PkgSrcVersion == "" {
		// Force the resolution of file path to be able to map to a package provided file
		if fh.ResolveFilePath(ev, f) == "" {
			return ""
		}

		if fh.resolvers.SBOMResolver == nil {
			return ""
		}

		if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ContainerContext.ID, f); pkg != nil {
			f.PkgSrcVersion = pkg.SrcVersion
		}
	}
	return f.PkgSrcVersion
}

// ResolveModuleArgv resolves the unscrubbed args of the module as an array. Use with caution.
func (fh *EBPFFieldHandlers) ResolveModuleArgv(_ *model.Event, module *model.LoadModuleEvent) []string {
	// strings.Split return [""] if args is empty, so we do a manual check before
	if len(module.Args) == 0 {
		module.Argv = nil
		return module.Argv
	}

	module.Argv = strings.Split(module.Args, " ")
	if module.ArgsTruncated {
		module.Argv = module.Argv[:len(module.Argv)-1]
	}
	return module.Argv
}

// ResolveModuleArgs resolves the correct args if the arguments were truncated, if not return module.Args
func (fh *EBPFFieldHandlers) ResolveModuleArgs(_ *model.Event, module *model.LoadModuleEvent) string {
	if module.ArgsTruncated {
		argsTmp := strings.Split(module.Args, " ")
		argsTmp = argsTmp[:len(argsTmp)-1]
		return strings.Join(argsTmp, " ")
	}
	return module.Args
}

// ResolveHashesFromEvent resolves the hashes of the requested event
func (fh *EBPFFieldHandlers) ResolveHashesFromEvent(ev *model.Event, f *model.FileEvent) []string {
	return fh.resolvers.HashResolver.ComputeHashesFromEvent(ev, f)
}

// ResolveHashes resolves the hashes of the requested file event
func (fh *EBPFFieldHandlers) ResolveHashes(eventType model.EventType, process *model.Process, file *model.FileEvent) []string {
	return fh.resolvers.HashResolver.ComputeHashes(eventType, process, file)
}

// ResolveContainerID resolves the container ID of the event
func (fh *EBPFFieldHandlers) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev); entry != nil {
			e.ID = entry.ContainerID
		}
	}
	return e.ID
}

// ResolveContainerCreatedAt resolves the container creation time of the event
func (fh *EBPFFieldHandlers) ResolveContainerCreatedAt(ev *model.Event, e *model.ContainerContext) int {
	if e.CreatedAt == 0 {
		if containerContext, _ := fh.ResolveContainerContext(ev); containerContext != nil {
			e.CreatedAt = containerContext.CreatedAt
		}
	}
	return int(e.CreatedAt)
}

// ResolveContainerTags resolves the container tags of the event
func (fh *EBPFFieldHandlers) ResolveContainerTags(_ *model.Event, e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ID != "" {
		e.Tags = fh.resolvers.TagsResolver.Resolve(e.ID)
	}
	return e.Tags
}

// ResolveProcessCreatedAt resolves process creation time
func (fh *EBPFFieldHandlers) ResolveProcessCreatedAt(_ *model.Event, e *model.Process) int {
	return int(e.ExecTime.UnixNano())
}

// ResolveUserSessionContext resolves and updates the provided user session context
func (fh *EBPFFieldHandlers) ResolveUserSessionContext(evtCtx *model.UserSessionContext) {
	if !evtCtx.Resolved {
		ctx := fh.resolvers.UserSessionsResolver.ResolveUserSession(evtCtx.ID)
		if ctx != nil {
			*evtCtx = *ctx
		}
	}
}

// ResolveK8SUsername resolves the k8s username of the event
func (fh *EBPFFieldHandlers) ResolveK8SUsername(_ *model.Event, evtCtx *model.UserSessionContext) string {
	fh.ResolveUserSessionContext(evtCtx)
	return evtCtx.K8SUsername
}

// ResolveK8SUID resolves the k8s UID of the event
func (fh *EBPFFieldHandlers) ResolveK8SUID(_ *model.Event, evtCtx *model.UserSessionContext) string {
	fh.ResolveUserSessionContext(evtCtx)
	return evtCtx.K8SUID
}

// ResolveK8SGroups resolves the k8s groups of the event
func (fh *EBPFFieldHandlers) ResolveK8SGroups(_ *model.Event, evtCtx *model.UserSessionContext) []string {
	fh.ResolveUserSessionContext(evtCtx)
	return evtCtx.K8SGroups
}

// ResolveProcessCmdArgv resolves the command line
func (fh *EBPFFieldHandlers) ResolveProcessCmdArgv(ev *model.Event, process *model.Process) []string {
	cmdline := []string{fh.ResolveProcessArgv0(ev, process)}
	return append(cmdline, fh.ResolveProcessArgv(ev, process)...)
}

// ResolveAWSSecurityCredentials resolves and updates the AWS security credentials of the input process entry
func (fh *EBPFFieldHandlers) ResolveAWSSecurityCredentials(e *model.Event) []model.AWSSecurityCredentials {
	return fh.resolvers.ProcessResolver.FetchAWSSecurityCredentials(e)
}

// ResolveSyscallCtxArgs resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgs(_ *model.Event, e *model.SyscallContext) {
	if !e.Resolved {
		_ = fh.resolvers.SyscallCtxResolver.Resolve(e.ID, e)
		e.Resolved = true
	}
}

// ResolveSyscallCtxArgsStr1 resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgsStr1(ev *model.Event, e *model.SyscallContext) string {
	fh.ResolveSyscallCtxArgs(ev, e)
	return e.StrArg1
}

// ResolveSyscallCtxArgsStr2 resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgsStr2(ev *model.Event, e *model.SyscallContext) string {
	fh.ResolveSyscallCtxArgs(ev, e)
	return e.StrArg2
}

// ResolveSyscallCtxArgsStr3 resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgsStr3(ev *model.Event, e *model.SyscallContext) string {
	fh.ResolveSyscallCtxArgs(ev, e)
	return e.StrArg3
}

// ResolveSyscallCtxArgsInt1 resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgsInt1(ev *model.Event, e *model.SyscallContext) int {
	fh.ResolveSyscallCtxArgs(ev, e)
	return int(e.IntArg1)
}

// ResolveSyscallCtxArgsInt2 resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgsInt2(ev *model.Event, e *model.SyscallContext) int {
	fh.ResolveSyscallCtxArgs(ev, e)
	return int(e.IntArg2)
}

// ResolveSyscallCtxArgsInt3 resolve syscall ctx
func (fh *EBPFFieldHandlers) ResolveSyscallCtxArgsInt3(ev *model.Event, e *model.SyscallContext) int {
	fh.ResolveSyscallCtxArgs(ev, e)
	return int(e.IntArg3)
}

// ResolveHostname resolve the hostname
func (fh *EBPFFieldHandlers) ResolveHostname(_ *model.Event, _ *model.BaseEvent) string {
	return fh.hostname
}
