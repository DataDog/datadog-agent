// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"path"
	"sort"
	"strings"
	"syscall"
	"time"

	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/args"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type FieldHandlers struct {
	resolvers *resolvers.Resolvers
}

// ResolveFilePath resolves the inode to a full path
func (fh *FieldHandlers) ResolveFilePath(ev *model.Event, f *model.FileEvent) string {
	if !f.IsPathnameStrResolved && len(f.PathnameStr) == 0 {
		path, err := fh.resolvers.PathResolver.ResolveFileFieldsPath(&f.FileFields, &ev.PIDContext, &ev.ContainerContext)
		if err != nil {
			ev.SetPathResolutionError(f, err)
		}
		f.SetPathnameStr(path)
	}

	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (fh *FieldHandlers) ResolveFileBasename(ev *model.Event, f *model.FileEvent) string {
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
func (fh *FieldHandlers) ResolveFileFilesystem(ev *model.Event, f *model.FileEvent) string {
	if f.Filesystem == "" && !f.IsFileless() {
		fs, err := fh.resolvers.MountResolver.ResolveFilesystem(f.FileFields.MountID, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			ev.SetPathResolutionError(f, err)
		}
		f.Filesystem = fs
	}
	return f.Filesystem
}

// ResolveFileFieldsInUpperLayer resolves whether the file is in an upper layer
func (fh *FieldHandlers) ResolveFileFieldsInUpperLayer(ev *model.Event, f *model.FileFields) bool {
	return f.GetInUpperLayer()
}

// ResolveXAttrName returns the string representation of the extended attribute name
func (fh *FieldHandlers) ResolveXAttrName(ev *model.Event, e *model.SetXAttrEvent) string {
	if len(e.Name) == 0 {
		e.Name, _ = model.UnmarshalString(e.NameRaw[:], 200)
	}
	return e.Name
}

// ResolveXAttrNamespace returns the string representation of the extended attribute namespace
func (fh *FieldHandlers) ResolveXAttrNamespace(ev *model.Event, e *model.SetXAttrEvent) string {
	if len(e.Namespace) == 0 {
		ns, _, found := strings.Cut(fh.ResolveXAttrName(ev, e), ".")
		if found {
			e.Namespace = ns
		}
	}
	return e.Namespace
}

func (fh *FieldHandlers) ResolveMountPointPath(ev *model.Event, e *model.MountEvent) string {
	if len(e.MountPointPath) == 0 {
		mountPointPath, err := fh.resolvers.MountResolver.ResolveMountPath(e.MountID, ev.PIDContext.Pid, ev.ContainerContext.ID)
		if err != nil {
			e.MountPointPathResolutionError = err
			return ""
		}
		e.MountPointPath = mountPointPath
	}
	return e.MountPointPath
}

func (fh *FieldHandlers) ResolveMountSourcePath(ev *model.Event, e *model.MountEvent) string {
	if e.BindSrcMountID != 0 && len(e.MountSourcePath) == 0 {
		bindSourceMountPath, err := fh.resolvers.MountResolver.ResolveMountPath(e.BindSrcMountID, ev.PIDContext.Pid, ev.ContainerContext.ID)
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

// ResolveContainerID resolves the container ID of the event
func (fh *FieldHandlers) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev); entry != nil {
			e.ID = entry.ContainerID
		}
	}
	return e.ID
}

// ResolveContainerCreatedAt resolves the container creation time of the event
func (fh *FieldHandlers) ResolveContainerCreatedAt(ev *model.Event, e *model.ContainerContext) int {
	if e.CreatedAt == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev); entry != nil && entry.ContainerID != "" {
			if cgroup, _ := fh.resolvers.CGroupResolver.GetWorkload(entry.ContainerID); cgroup != nil {
				e.CreatedAt = cgroup.CreationTime
			}
		}
	}
	return int(e.CreatedAt)
}

// ResolveContainerTags resolves the container tags of the event
func (fh *FieldHandlers) ResolveContainerTags(ev *model.Event, e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ID != "" {
		e.Tags = fh.resolvers.TagsResolver.Resolve(e.ID)
	}
	return e.Tags
}

// ResolveRights resolves the rights of a file
func (fh *FieldHandlers) ResolveRights(ev *model.Event, e *model.FileFields) int {
	return int(e.Mode) & (syscall.S_ISUID | syscall.S_ISGID | syscall.S_ISVTX | syscall.S_IRWXU | syscall.S_IRWXG | syscall.S_IRWXO)
}

// ResolveChownUID resolves the ResolveProcessCacheEntry id of a chown event to a username
func (fh *FieldHandlers) ResolveChownUID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveChownGID resolves the group id of a chown event to a group name
func (fh *FieldHandlers) ResolveChownGID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
}

// ResolveProcessCreatedAt resolves process creation time
func (fh *FieldHandlers) ResolveProcessCreatedAt(ev *model.Event, e *model.Process) int {
	return int(e.ExecTime.UnixNano())
}

// ResolveProcessArgv0 resolves the first arg of the event
func (fh *FieldHandlers) ResolveProcessArgv0(ev *model.Event, process *model.Process) string {
	arg0, _ := fh.resolvers.ProcessResolver.GetProcessArgv0(process)
	return arg0
}

// ResolveProcessArgs resolves the args of the event
func (fh *FieldHandlers) ResolveProcessArgs(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgv(ev, process), " ")
}

// ResolveProcessArgv resolves the args of the event as an array
func (fh *FieldHandlers) ResolveProcessArgv(ev *model.Event, process *model.Process) []string {
	argv, _ := sprocess.GetProcessArgv(process)
	return argv
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (fh *FieldHandlers) ResolveProcessEnvp(ev *model.Event, process *model.Process) []string {
	envp, _ := fh.resolvers.ProcessResolver.GetProcessEnvp(process)
	return envp
}

// ResolveProcessArgsTruncated returns whether the args are truncated
func (fh *FieldHandlers) ResolveProcessArgsTruncated(ev *model.Event, process *model.Process) bool {
	_, truncated := sprocess.GetProcessArgv(process)
	return truncated
}

// ResolveProcessArgsFlags resolves the arguments flags of the event
func (fh *FieldHandlers) ResolveProcessArgsFlags(ev *model.Event, process *model.Process) (flags []string) {
	return args.ParseProcessFlags(fh.ResolveProcessArgv(ev, process))
}

// ResolveProcessArgsOptions resolves the arguments options of the event
func (fh *FieldHandlers) ResolveProcessArgsOptions(ev *model.Event, process *model.Process) (options []string) {
	return args.ParseProcessOptions(fh.ResolveProcessArgv(ev, process))
}

// ResolveProcessEnvsTruncated returns whether the envs are truncated
func (fh *FieldHandlers) ResolveProcessEnvsTruncated(ev *model.Event, process *model.Process) bool {
	_, truncated := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return truncated
}

// ResolveProcessEnvs resolves the envs of the event
func (fh *FieldHandlers) ResolveProcessEnvs(ev *model.Event, process *model.Process) []string {
	envs, _ := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return envs
}

// ResolveSetuidUser resolves the user of the Setuid event
func (fh *FieldHandlers) ResolveSetuidUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveSetuidEUser resolves the effective user of the Setuid event
func (fh *FieldHandlers) ResolveSetuidEUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.EUser) == 0 {
		e.EUser, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.EUID))
	}
	return e.EUser
}

// ResolveSetuidFSUser resolves the file-system user of the Setuid event
func (fh *FieldHandlers) ResolveSetuidFSUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.FSUser) == 0 {
		e.FSUser, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.FSUID))
	}
	return e.FSUser
}

// ResolveSetgidGroup resolves the group of the Setgid event
func (fh *FieldHandlers) ResolveSetgidGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.GID))
	}
	return e.Group
}

// ResolveSetgidEGroup resolves the effective group of the Setgid event
func (fh *FieldHandlers) ResolveSetgidEGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.EGroup) == 0 {
		e.EGroup, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.EGID))
	}
	return e.EGroup
}

// ResolveSetgidFSGroup resolves the file-system group of the Setgid event
func (fh *FieldHandlers) ResolveSetgidFSGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.FSGroup) == 0 {
		e.FSGroup, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.FSGID))
	}
	return e.FSGroup
}

// ResolveSELinuxBoolName resolves the boolean name of the SELinux event
func (fh *FieldHandlers) ResolveSELinuxBoolName(ev *model.Event, e *model.SELinuxEvent) string {
	if e.EventKind != model.SELinuxBoolChangeEventKind {
		return ""
	}

	if len(e.BoolName) == 0 {
		e.BoolName = fh.resolvers.PathResolver.ResolveBasename(&e.File.FileFields)
	}
	return e.BoolName
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *FieldHandlers) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.PIDContext.IsKworker {
		return model.NewEmptyProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, true), false
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid, ev.PIDContext.Inode)
	}

	if ev.ProcessCacheEntry == nil {
		// keep the original PIDContext
		ev.ProcessCacheEntry = model.NewProcessCacheEntry(nil)
		ev.ProcessCacheEntry.PIDContext = ev.PIDContext

		ev.ProcessCacheEntry.FileEvent.SetPathnameStr("")
		ev.ProcessCacheEntry.FileEvent.SetBasenameStr("")

		// mark interpreter as resolved too
		ev.ProcessCacheEntry.LinuxBinprm.FileEvent.SetPathnameStr("")
		ev.ProcessCacheEntry.LinuxBinprm.FileEvent.SetBasenameStr("")

		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// GetProcessServiceTag returns the service tag based on the process context
func (fh *FieldHandlers) GetProcessServiceTag(ev *model.Event) string {
	entry, _ := fh.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}

	var serviceValues []string

	// first search in the process context itself
	if entry.EnvsEntry != nil {
		if service := entry.EnvsEntry.Get(ServiceEnvVar); service != "" {
			serviceValues = append(serviceValues, service)
		}
	}

	inContainer := entry.ContainerID != ""

	// while in container check for each ancestor
	for ancestor := entry.Ancestor; ancestor != nil; ancestor = ancestor.Ancestor {
		if inContainer && ancestor.ContainerID == "" {
			break
		}

		if ancestor.EnvsEntry != nil {
			if service := ancestor.EnvsEntry.Get(ServiceEnvVar); service != "" {
				serviceValues = append(serviceValues, service)
			}
		}
	}

	return bestGuessServiceTag(serviceValues)
}

// ResolveFileFieldsGroup resolves the group id of the file to a group name
func (fh *FieldHandlers) ResolveFileFieldsGroup(ev *model.Event, e *model.FileFields) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveGroup(int(e.GID))
	}
	return e.Group
}

func bestGuessServiceTag(serviceValues []string) string {
	if len(serviceValues) == 0 {
		return ""
	}

	firstGuess := serviceValues[0]

	// first we sort base on len, biggest len first
	sort.Slice(serviceValues, func(i, j int) bool {
		return len(serviceValues[j]) < len(serviceValues[i]) // reverse
	})

	// we then compare [i] and [i + 1] to check if [i + 1] is a prefix of [i]
	for i := 0; i < len(serviceValues)-1; i++ {
		if !strings.HasPrefix(serviceValues[i], serviceValues[i+1]) {
			// if it's not a prefix it means we have multiple disjoints services
			// we then return the first guess, closest in the process tree
			return firstGuess
		}
	}

	// we have a prefix chain, let's return the biggest one
	return serviceValues[0]
}

// ResolveNetworkDeviceIfName returns the network iterface name from the network context
func (fh *FieldHandlers) ResolveNetworkDeviceIfName(ev *model.Event, device *model.NetworkDeviceContext) string {
	if len(device.IfName) == 0 && fh.resolvers.TCResolver != nil {
		ifName, ok := fh.resolvers.TCResolver.ResolveNetworkDeviceIfName(device.IfIndex, device.NetNS)
		if ok {
			device.IfName = ifName
		}
	}

	return device.IfName
}

// ResolveFileFieldsUser resolves the user id of the file to a username
func (fh *FieldHandlers) ResolveFileFieldsUser(ev *model.Event, e *model.FileFields) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID))
	}
	return e.User
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTimestamp(ev *model.Event) time.Time {
	if ev.Timestamp.IsZero() {
		fh := ev.FieldHandlers.(*FieldHandlers)

		ev.Timestamp = fh.resolvers.TimeResolver.ResolveMonotonicTimestamp(ev.TimestampRaw)
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now()
		}
	}
	return ev.Timestamp
}

// ResolveAsync resolves the async flag
func (fh *FieldHandlers) ResolveAsync(ev *model.Event) bool {
	ev.Async = ev.Flags&model.EventFlagsAsync > 0
	return ev.Async
}

// ResolvePackageName resolves the name of the package providing this file
func (fh *FieldHandlers) ResolvePackageName(ev *model.Event, f *model.FileEvent) string {
	if f.PkgName == "" {
		// Force the resolution of file path to be able to map to a package provided file
		if fh.ResolveFilePath(ev, f) == "" {
			return ""
		}

		if fh.resolvers.SBOMResolver == nil {
			return ""
		}

		if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ProcessCacheEntry.ContainerID, f); pkg != nil {
			f.PkgName = pkg.Name
		}
	}
	return f.PkgName
}

// ResolvePackageVersion resolves the version of the package providing this file
func (fh *FieldHandlers) ResolvePackageVersion(ev *model.Event, f *model.FileEvent) string {
	if f.PkgVersion == "" {
		// Force the resolution of file path to be able to map to a package provided file
		if fh.ResolveFilePath(ev, f) == "" {
			return ""
		}

		if fh.resolvers.SBOMResolver == nil {
			return ""
		}

		if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ProcessCacheEntry.ContainerID, f); pkg != nil {
			f.PkgVersion = pkg.Version
		}
	}
	return f.PkgVersion
}

// ResolvePackageSourceVersion resolves the version of the source package of the package providing this file
func (fh *FieldHandlers) ResolvePackageSourceVersion(ev *model.Event, f *model.FileEvent) string {
	if f.PkgSrcVersion == "" {
		// Force the resolution of file path to be able to map to a package provided file
		if fh.ResolveFilePath(ev, f) == "" {
			return ""
		}

		if fh.resolvers.SBOMResolver == nil {
			return ""
		}

		if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ProcessCacheEntry.ContainerID, f); pkg != nil {
			f.PkgSrcVersion = pkg.SrcVersion
		}
	}
	return f.PkgSrcVersion
}

// ResolveModuleArgv resolves the args of the event as an array
func (fh *FieldHandlers) ResolveModuleArgv(ev *model.Event, module *model.LoadModuleEvent) []string {
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
func (fh *FieldHandlers) ResolveModuleArgs(ev *model.Event, module *model.LoadModuleEvent) string {
	if module.ArgsTruncated {
		argsTmp := strings.Split(module.Args, " ")
		argsTmp = argsTmp[:len(argsTmp)-1]
		return strings.Join(argsTmp, " ")
	}
	return module.Args
}
