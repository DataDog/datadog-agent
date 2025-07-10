// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"path"
	"strings"
	"syscall"
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/security/secl/args"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// EBPFFieldHandlers defines a field handlers
type EBPFFieldHandlers struct {
	*BaseFieldHandlers
	resolvers *resolvers.EBPFResolvers
	onDemand  *OnDemandProbesManager
}

// NewEBPFFieldHandlers returns a new EBPFFieldHandlers
func NewEBPFFieldHandlers(config *config.Config, resolvers *resolvers.EBPFResolvers, hostname string, onDemand *OnDemandProbesManager) (*EBPFFieldHandlers, error) {
	bfh, err := NewBaseFieldHandlers(config, hostname)
	if err != nil {
		return nil, err
	}

	return &EBPFFieldHandlers{
		BaseFieldHandlers: bfh,
		resolvers:         resolvers,
		onDemand:          onDemand,
	}, nil
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFFieldHandlers) ResolveProcessCacheEntry(ev *model.Event, newEntryCb func(*model.ProcessCacheEntry, error)) (*model.ProcessCacheEntry, bool) {
	if ev.PIDContext.IsKworker {
		return model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, true), false
	}

	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid, ev.PIDContext.ExecInode, true, newEntryCb)
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, false)
		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// ResolveProcessCacheEntryFromPID queries the ProcessResolver to retrieve the ProcessContext of the provided PID
func (fh *EBPFFieldHandlers) ResolveProcessCacheEntryFromPID(pid uint32) *model.ProcessCacheEntry {
	return fh.resolvers.ProcessResolver.Resolve(pid, pid, 0, true, nil)
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
			fs, err := fh.resolvers.MountResolver.ResolveFilesystem(f.FileFields.MountID, f.FileFields.Device, ev.PIDContext.Pid, ev.ContainerContext.ContainerID)
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
	return f.IsInUpperLayer()
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
		mountPointPath, _, _, err := fh.resolvers.MountResolver.ResolveMountPath(e.MountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ContainerID)
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
		bindSourceMountPath, _, _, err := fh.resolvers.MountResolver.ResolveMountPath(e.BindSrcMountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ContainerID)
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
		mountRootPath, _, _, err := fh.resolvers.MountResolver.ResolveMountRoot(e.MountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ContainerID)
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
	if ev.ContainerContext.ContainerID != "" && !ev.ContainerContext.Resolved {
		if containerContext, _ := fh.resolvers.CGroupResolver.GetWorkload(ev.ContainerContext.ContainerID); containerContext != nil {
			if containerContext.CGroupFlags.IsContainer() {
				ev.ContainerContext = &containerContext.ContainerContext
			}

			ev.ContainerContext.Resolved = true
		}
	}
	return ev.ContainerContext, ev.ContainerContext.Resolved
}

// ResolveContainerRuntime retrieves the container runtime managing the container
func (fh *EBPFFieldHandlers) ResolveContainerRuntime(ev *model.Event, _ *model.ContainerContext) string {
	if ev.CGroupContext.CGroupFlags != 0 && ev.ContainerContext.ContainerID != "" {
		return getContainerRuntime(ev.CGroupContext.CGroupFlags)
	}

	return ""
}

// getContainerRuntime returns the container runtime managing the cgroup
func getContainerRuntime(flags containerutils.CGroupFlags) string {
	switch flags.GetCGroupManager() {
	case containerutils.CGroupManagerCRI:
		return string(workloadmeta.ContainerRuntimeContainerd)
	case containerutils.CGroupManagerCRIO:
		return string(workloadmeta.ContainerRuntimeCRIO)
	case containerutils.CGroupManagerDocker:
		return string(workloadmeta.ContainerRuntimeDocker)
	case containerutils.CGroupManagerPodman:
		return string(workloadmeta.ContainerRuntimePodman)
	default:
		return ""
	}
}

// ResolveRights resolves the rights of a file
func (fh *EBPFFieldHandlers) ResolveRights(_ *model.Event, e *model.FileFields) int {
	return int(e.Mode) & (syscall.S_ISUID | syscall.S_ISGID | syscall.S_ISVTX | syscall.S_IRWXU | syscall.S_IRWXG | syscall.S_IRWXO)
}

// ResolveChownUID resolves the user id of a chown event to a username
func (fh *EBPFFieldHandlers) ResolveChownUID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID), ev.ContainerContext.ContainerID)
	}
	return e.User
}

// ResolveChownGID resolves the group id of a chown event to a group name
func (fh *EBPFFieldHandlers) ResolveChownGID(ev *model.Event, e *model.ChownEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveGroup(int(e.GID), ev.ContainerContext.ContainerID)
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
	if process.Args == "" {
		process.Args = strings.Join(fh.ResolveProcessArgv(ev, process), " ")
	}
	return process.Args
}

// ResolveProcessArgsScrubbed resolves the args of the event
func (fh *EBPFFieldHandlers) ResolveProcessArgsScrubbed(ev *model.Event, process *model.Process) string {
	if process.ArgsScrubbed == "" {
		process.ArgsScrubbed = strings.Join(fh.ResolveProcessArgvScrubbed(ev, process), " ")
	}
	return process.ArgsScrubbed
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

// ResolveProcessIsThread returns true is the process is a thread
func (fh *EBPFFieldHandlers) ResolveProcessIsThread(_ *model.Event, process *model.Process) bool {
	return !process.IsExec
}

// ResolveSetuidUser resolves the user of the Setuid event
func (fh *EBPFFieldHandlers) ResolveSetuidUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.User) == 0 {
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID), ev.ContainerContext.ContainerID)
	}
	return e.User
}

// ResolveSetuidEUser resolves the effective user of the Setuid event
func (fh *EBPFFieldHandlers) ResolveSetuidEUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.EUser) == 0 {
		e.EUser, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.EUID), ev.ContainerContext.ContainerID)
	}
	return e.EUser
}

// ResolveSetuidFSUser resolves the file-system user of the Setuid event
func (fh *EBPFFieldHandlers) ResolveSetuidFSUser(ev *model.Event, e *model.SetuidEvent) string {
	if len(e.FSUser) == 0 {
		e.FSUser, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.FSUID), ev.ContainerContext.ContainerID)
	}
	return e.FSUser
}

// ResolveSetgidGroup resolves the group of the Setgid event
func (fh *EBPFFieldHandlers) ResolveSetgidGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.GID), ev.ContainerContext.ContainerID)
	}
	return e.Group
}

// ResolveSetgidEGroup resolves the effective group of the Setgid event
func (fh *EBPFFieldHandlers) ResolveSetgidEGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.EGroup) == 0 {
		e.EGroup, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.EGID), ev.ContainerContext.ContainerID)
	}
	return e.EGroup
}

// ResolveSetgidFSGroup resolves the file-system group of the Setgid event
func (fh *EBPFFieldHandlers) ResolveSetgidFSGroup(ev *model.Event, e *model.SetgidEvent) string {
	if len(e.FSGroup) == 0 {
		e.FSGroup, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.FSGID), ev.ContainerContext.ContainerID)
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
func (fh *EBPFFieldHandlers) GetProcessCacheEntry(ev *model.Event, newEntryCb func(*model.ProcessCacheEntry, error)) (*model.ProcessCacheEntry, bool) {
	ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(ev.PIDContext.Pid, ev.PIDContext.Tid, ev.PIDContext.ExecInode, false, newEntryCb)
	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Tid, false)
		return ev.ProcessCacheEntry, false
	}
	return ev.ProcessCacheEntry, true
}

// ResolveFileFieldsGroup resolves the group id of the file to a group name
func (fh *EBPFFieldHandlers) ResolveFileFieldsGroup(ev *model.Event, e *model.FileFields) string {
	if len(e.Group) == 0 {
		e.Group, _ = fh.resolvers.UserGroupResolver.ResolveGroup(int(e.GID), ev.ContainerContext.ContainerID)
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
		e.User, _ = fh.resolvers.UserGroupResolver.ResolveUser(int(e.UID), ev.ContainerContext.ContainerID)
	}
	return e.User
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *EBPFFieldHandlers) ResolveEventTimestamp(ev *model.Event, e *model.BaseEvent) int {
	return int(fh.ResolveEventTime(ev, e).UnixNano())
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

func (fh *EBPFFieldHandlers) resolveSBOMFields(ev *model.Event, f *model.FileEvent) {
	// Force the resolution of file path to be able to map to a package provided file
	if fh.ResolveFilePath(ev, f) == "" {
		return
	}

	if fh.resolvers.SBOMResolver == nil {
		return
	}

	if pkg := fh.resolvers.SBOMResolver.ResolvePackage(ev.ContainerContext.ContainerID, f); pkg != nil {
		f.PkgName = pkg.Name
		f.PkgVersion = pkg.Version
		f.PkgSrcVersion = pkg.SrcVersion
	}
}

// ResolvePackageName resolves the name of the package providing this file
func (fh *EBPFFieldHandlers) ResolvePackageName(ev *model.Event, f *model.FileEvent) string {
	if f.PkgName == "" {
		fh.resolveSBOMFields(ev, f)
	}
	return f.PkgName
}

// ResolvePackageVersion resolves the version of the package providing this file
func (fh *EBPFFieldHandlers) ResolvePackageVersion(ev *model.Event, f *model.FileEvent) string {
	if f.PkgVersion == "" {
		fh.resolveSBOMFields(ev, f)
	}
	return f.PkgVersion
}

// ResolvePackageSourceVersion resolves the version of the source package of the package providing this file
func (fh *EBPFFieldHandlers) ResolvePackageSourceVersion(ev *model.Event, f *model.FileEvent) string {
	if f.PkgSrcVersion == "" {
		fh.resolveSBOMFields(ev, f)
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

// ResolveCGroupID resolves the cgroup ID of the event
func (fh *EBPFFieldHandlers) ResolveCGroupID(ev *model.Event, e *model.CGroupContext) string {
	if len(e.CGroupID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev, nil); entry != nil {
			if entry.CGroup.CGroupID != "" && entry.CGroup.CGroupID != "/" {
				return string(entry.CGroup.CGroupID)
			}

			if cgroupContext, _, err := fh.resolvers.ResolveCGroupContext(e.CGroupFile, e.CGroupFlags); err == nil {
				ev.CGroupContext = cgroupContext
			}
		}
	}

	return string(e.CGroupID)
}

// ResolveCGroupManager resolves the manager of the cgroup
func (fh *EBPFFieldHandlers) ResolveCGroupManager(ev *model.Event, _ *model.CGroupContext) string {
	if entry, _ := fh.ResolveProcessCacheEntry(ev, nil); entry != nil {
		if manager := entry.CGroup.CGroupFlags.GetCGroupManager(); manager != 0 {
			return manager.String()
		}
	}

	return ""
}

// ResolveCGroupVersion resolves the version of the cgroup API
func (fh *EBPFFieldHandlers) ResolveCGroupVersion(ev *model.Event, e *model.CGroupContext) int {
	if e.CGroupVersion == 0 {
		if filesystem, _ := fh.resolvers.MountResolver.ResolveFilesystem(e.CGroupFile.MountID, 0, ev.PIDContext.Pid, ev.ContainerContext.ContainerID); filesystem == "cgroup2" {
			e.CGroupVersion = 2
		} else {
			e.CGroupVersion = 1
		}
	}
	return e.CGroupVersion
}

// ResolveContainerID resolves the container ID of the event
func (fh *EBPFFieldHandlers) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ContainerID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev, nil); entry != nil {
			if entry.CGroup.CGroupFlags.IsContainer() {
				e.ContainerID = containerutils.ContainerID(entry.ContainerID)
			} else {
				e.ContainerID = ""
			}
			return string(e.ContainerID)
		}
	}
	return string(e.ContainerID)
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
	if len(e.Tags) == 0 && e.ContainerID != "" {
		e.Tags = fh.resolvers.TagsResolver.Resolve(e.ContainerID)
	}
	return e.Tags
}

// ResolveProcessContainerID resolves the container ID of the event
func (fh *EBPFFieldHandlers) ResolveProcessContainerID(ev *model.Event, _ *model.Process) string {
	return fh.ResolveContainerID(ev, ev.ContainerContext)
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

// ResolveFileMetadata resolves file metadata
func (fh *EBPFFieldHandlers) ResolveFileMetadata(event *model.Event) *model.FileMetadata {
	if !fh.resolvers.FileMetadataResolver.Enabled {
		return nil
	}
	if event.Type == uint32(model.ExecEventType) {
		if event.Exec.FileMetadata.Resolved {
			return &event.Exec.FileMetadata
		}
		metadata, err := fh.resolvers.FileMetadataResolver.ResolveFileMetadata(event, &event.Exec.Process.FileEvent)
		if err != nil || metadata == nil {
			seclog.Errorf("failed to resolve exec binary metadata: %s", err)
			return nil
		}
		event.Exec.FileMetadata = *metadata
		event.Exec.FileMetadata.Resolved = true
		return metadata
	}
	return nil
}

// ResolveFileMetadataSize resolves file metadata size
func (fh *EBPFFieldHandlers) ResolveFileMetadataSize(event *model.Event, _ *model.FileMetadata) int {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return int(fm.Size)
	}
	return 0
}

// ResolveFileMetadataType resolves file metadata type
func (fh *EBPFFieldHandlers) ResolveFileMetadataType(event *model.Event, _ *model.FileMetadata) int {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return int(fm.Type)
	}
	return 0
}

// ResolveFileMetadataIsExecutable resolves file metadata is_executable
func (fh *EBPFFieldHandlers) ResolveFileMetadataIsExecutable(event *model.Event, _ *model.FileMetadata) bool {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return fm.IsExecutable
	}
	return false
}

// ResolveFileMetadataArchitecture resolves file metadata architecture
func (fh *EBPFFieldHandlers) ResolveFileMetadataArchitecture(event *model.Event, _ *model.FileMetadata) int {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return int(fm.Architecture)
	}
	return 0
}

// ResolveFileMetadataABI resolves file metadata ABI
func (fh *EBPFFieldHandlers) ResolveFileMetadataABI(event *model.Event, _ *model.FileMetadata) int {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return int(fm.ABI)
	}
	return 0
}

// ResolveFileMetadataIsUPXPacked resolves file metadata is_upx_packed
func (fh *EBPFFieldHandlers) ResolveFileMetadataIsUPXPacked(event *model.Event, _ *model.FileMetadata) bool {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return fm.IsUPXPacked
	}
	return false
}

// ResolveFileMetadataCompression resolves file metadata compression
func (fh *EBPFFieldHandlers) ResolveFileMetadataCompression(event *model.Event, _ *model.FileMetadata) int {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return int(fm.Compression)
	}
	return 0
}

// ResolveFileMetadataIsGarbleObfuscated resolves file metadata is_garble_obfuscated
func (fh *EBPFFieldHandlers) ResolveFileMetadataIsGarbleObfuscated(event *model.Event, _ *model.FileMetadata) bool {
	fm := fh.ResolveFileMetadata(event)
	if fm != nil {
		return fm.IsGarbleObfuscated
	}
	return false
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
		err := fh.resolvers.SyscallCtxResolver.Resolve(e.ID, e)
		if err != nil {
			return
		}
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

// ResolveOnDemandName resolves the on-demand event name
func (fh *EBPFFieldHandlers) ResolveOnDemandName(_ *model.Event, e *model.OnDemandEvent) string {
	if fh.onDemand == nil {
		return ""
	}
	return fh.onDemand.getHookNameFromID(int(e.ID))
}

func resolveOnDemandArgStr(e *model.OnDemandEvent, index int) string {
	if !(1 <= index && index <= model.OnDemandParsedArgsCount) {
		panic(fmt.Sprintf("index must be between 1 and %d", model.OnDemandParsedArgsCount))
	}

	start := (index - 1) * model.OnDemandPerArgSize
	data := e.Data[start : start+model.OnDemandPerArgSize]
	return model.NullTerminatedString(data)
}

func resolveOnDemandArgUint(e *model.OnDemandEvent, index int) int {
	if !(1 <= index && index <= model.OnDemandParsedArgsCount) {
		panic(fmt.Sprintf("index must be between 1 and %d", model.OnDemandParsedArgsCount))
	}

	start := (index - 1) * model.OnDemandPerArgSize
	return int(binary.NativeEndian.Uint64(e.Data[start : start+8]))
}

// ResolveOnDemandArg1Str resolves the string value of the first argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg1Str(_ *model.Event, e *model.OnDemandEvent) string {
	return resolveOnDemandArgStr(e, 1)
}

// ResolveOnDemandArg1Uint resolves the uint value of the first argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg1Uint(_ *model.Event, e *model.OnDemandEvent) int {
	return resolveOnDemandArgUint(e, 1)
}

// ResolveOnDemandArg2Str resolves the string value of the second argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg2Str(_ *model.Event, e *model.OnDemandEvent) string {
	return resolveOnDemandArgStr(e, 2)
}

// ResolveOnDemandArg2Uint resolves the uint value of the second argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg2Uint(_ *model.Event, e *model.OnDemandEvent) int {
	return resolveOnDemandArgUint(e, 2)
}

// ResolveOnDemandArg3Str resolves the string value of the third argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg3Str(_ *model.Event, e *model.OnDemandEvent) string {
	return resolveOnDemandArgStr(e, 3)
}

// ResolveOnDemandArg3Uint resolves the uint value of the third argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg3Uint(_ *model.Event, e *model.OnDemandEvent) int {
	return resolveOnDemandArgUint(e, 3)
}

// ResolveOnDemandArg4Str resolves the string value of the fourth argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg4Str(_ *model.Event, e *model.OnDemandEvent) string {
	return resolveOnDemandArgStr(e, 4)
}

// ResolveOnDemandArg4Uint resolves the uint value of the fourth argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg4Uint(_ *model.Event, e *model.OnDemandEvent) int {
	return resolveOnDemandArgUint(e, 4)
}

// ResolveOnDemandArg5Str resolves the string value of the fifth argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg5Str(_ *model.Event, e *model.OnDemandEvent) string {
	return resolveOnDemandArgStr(e, 5)
}

// ResolveOnDemandArg5Uint resolves the uint value of the fifth argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg5Uint(_ *model.Event, e *model.OnDemandEvent) int {
	return resolveOnDemandArgUint(e, 5)
}

// ResolveOnDemandArg6Str resolves the string value of the sixth argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg6Str(_ *model.Event, e *model.OnDemandEvent) string {
	return resolveOnDemandArgStr(e, 6)
}

// ResolveOnDemandArg6Uint resolves the uint value of the sixth argument of hooked function
func (fh *EBPFFieldHandlers) ResolveOnDemandArg6Uint(_ *model.Event, e *model.OnDemandEvent) int {
	return resolveOnDemandArgUint(e, 6)
}

// ResolveProcessNSID resolves the process namespace ID
func (fh *EBPFFieldHandlers) ResolveProcessNSID(e *model.Event) (uint64, error) {
	if e.ProcessCacheEntry.Process.NSID != 0 {
		return e.ProcessCacheEntry.Process.NSID, nil
	}

	nsid, err := utils.GetProcessPidNamespace(e.ProcessCacheEntry.Process.Pid)
	if err != nil {
		return 0, err
	}
	e.ProcessCacheEntry.Process.NSID = nsid
	return nsid, nil
}

func (fh *EBPFFieldHandlers) resolveHostnames(ip net.IP) []string {
	if !fh.config.Probe.DNSResolutionEnabled {
		return nil
	}

	nip, ok := netip.AddrFromSlice(ip)
	if ok {
		return fh.resolvers.DNSResolver.HostListFromIP(nip)
	}
	return nil
}

// ResolveConnectHostnames resolves the hostnames of a connect event
func (fh *EBPFFieldHandlers) ResolveConnectHostnames(_ *model.Event, e *model.ConnectEvent) []string {
	if len(e.Hostnames) == 0 {
		e.Hostnames = fh.resolveHostnames(e.Addr.IPNet.IP)
	}

	return e.Hostnames
}

// ResolveAcceptHostnames resolves the hostnames of an accept event
func (fh *EBPFFieldHandlers) ResolveAcceptHostnames(_ *model.Event, e *model.AcceptEvent) []string {
	if len(e.Hostnames) == 0 {
		e.Hostnames = fh.resolveHostnames(e.Addr.IPNet.IP)
	}

	return e.Hostnames
}

// ResolveSetSockOptFilterHash resolves the filter hash of a setsockopt event
func (fh *EBPFFieldHandlers) ResolveSetSockOptFilterHash(_ *model.Event, e *model.SetSockOptEvent) string {
	if len(e.FilterHash) == 0 {
		h := sha256.New()
		h.Write(e.RawFilter)
		bs := h.Sum(nil)
		e.FilterHash = fmt.Sprintf("%x", bs)
		return e.FilterHash
	}
	return e.FilterHash
}

// ResolveSetSockOptFilterInstructions resolves the filter instructions of a setsockopt event
func (fh *EBPFFieldHandlers) ResolveSetSockOptFilterInstructions(_ *model.Event, e *model.SetSockOptEvent) string {
	if len(e.FilterInstructions) == 0 {
		raw := []bpf.RawInstruction{}
		filterSize := 8
		sizeToRead := int(e.SizeToRead)
		actualNumberOfFilters := sizeToRead / filterSize
		rawFilter := e.RawFilter
		for i := 0; i < actualNumberOfFilters; i++ {
			offset := i * filterSize

			Code := binary.NativeEndian.Uint16(rawFilter[offset : offset+2])
			Jt := rawFilter[offset+2]
			Jf := rawFilter[offset+3]
			K := binary.NativeEndian.Uint32(rawFilter[offset+4 : offset+8])

			raw = append(raw, bpf.RawInstruction{
				Op: Code,
				Jt: Jt,
				Jf: Jf,
				K:  K,
			})
		}

		instructions, _ := bpf.Disassemble(raw)

		for i, inst := range instructions {
			e.FilterInstructions += fmt.Sprintf("%03d: %s\n", i, inst)
		}

		return e.FilterInstructions
	}
	return e.FilterInstructions
}
