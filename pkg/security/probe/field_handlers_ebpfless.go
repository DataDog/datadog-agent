// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	sprocess "github.com/DataDog/datadog-agent/pkg/security/resolvers/process"

	"github.com/DataDog/datadog-agent/pkg/security/secl/args"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// EBPFLessFieldHandlers defines a field handlers
type EBPFLessFieldHandlers struct {
	config    *config.Config
	resolvers *resolvers.EBPFLessResolvers
	hostname  string
}

// ResolveService returns the service tag based on the process context
func (fh *EBPFLessFieldHandlers) ResolveService(ev *model.Event, _ *model.BaseEvent) string {
	entry, _ := fh.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}
	return getProcessService(fh.config, entry)
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry == nil && ev.PIDContext.Pid != 0 {
		ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(sprocess.CacheResolverKey{
			Pid:  ev.PIDContext.Pid,
			NSID: ev.PIDContext.NSID,
		})
	}

	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Pid, false)
		return ev.ProcessCacheEntry, false
	}

	return ev.ProcessCacheEntry, true
}

// ResolveFilePath resolves the inode to a full path
func (fh *EBPFLessFieldHandlers) ResolveFilePath(_ *model.Event, f *model.FileEvent) string {
	return f.PathnameStr
}

// ResolveFileBasename resolves the inode to a full path
func (fh *EBPFLessFieldHandlers) ResolveFileBasename(_ *model.Event, f *model.FileEvent) string {
	return f.BasenameStr
}

// ResolveProcessArgsFlags resolves the arguments flags of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsFlags(ev *model.Event, process *model.Process) (flags []string) {
	return args.ParseProcessFlags(fh.ResolveProcessArgv(ev, process))
}

// ResolveProcessArgsOptions resolves the arguments options of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsOptions(ev *model.Event, process *model.Process) (options []string) {
	return args.ParseProcessOptions(fh.ResolveProcessArgv(ev, process))
}

// ResolveProcessArgv0 resolves the first arg of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgv0(_ *model.Event, process *model.Process) string {
	arg0, _ := sprocess.GetProcessArgv0(process)
	return arg0
}

// ResolveProcessArgs resolves the args of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgs(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgv(ev, process), " ")
}

// ResolveProcessArgv resolves the unscrubbed args of the process as an array. Use with caution.
func (fh *EBPFLessFieldHandlers) ResolveProcessArgv(_ *model.Event, process *model.Process) []string {
	argv, _ := sprocess.GetProcessArgv(process)
	return argv
}

// ResolveProcessArgvScrubbed resolves the args of the process as an array
func (fh *EBPFLessFieldHandlers) ResolveProcessArgvScrubbed(_ *model.Event, process *model.Process) []string {
	argv, _ := fh.resolvers.ProcessResolver.GetProcessArgvScrubbed(process)
	return argv
}

// ResolveProcessArgsScrubbed resolves the args of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsScrubbed(ev *model.Event, process *model.Process) string {
	return strings.Join(fh.ResolveProcessArgvScrubbed(ev, process), " ")
}

// ResolveProcessEnvp resolves the envp of the event as an array
func (fh *EBPFLessFieldHandlers) ResolveProcessEnvp(_ *model.Event, process *model.Process) []string {
	envp, _ := fh.resolvers.ProcessResolver.GetProcessEnvp(process)
	return envp
}

// ResolveProcessArgsTruncated returns whether the args are truncated
func (fh *EBPFLessFieldHandlers) ResolveProcessArgsTruncated(_ *model.Event, process *model.Process) bool {
	_, truncated := sprocess.GetProcessArgv(process)
	return truncated
}

// ResolveProcessEnvsTruncated returns whether the envs are truncated
func (fh *EBPFLessFieldHandlers) ResolveProcessEnvsTruncated(_ *model.Event, process *model.Process) bool {
	_, truncated := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return truncated
}

// ResolveProcessEnvs resolves the unscrubbed envs of the event. Use with caution.
func (fh *EBPFLessFieldHandlers) ResolveProcessEnvs(_ *model.Event, process *model.Process) []string {
	envs, _ := fh.resolvers.ProcessResolver.GetProcessEnvs(process)
	return envs
}

// ResolveProcessIsThread returns true is the process is a thread
func (fh *EBPFLessFieldHandlers) ResolveProcessIsThread(_ *model.Event, process *model.Process) bool {
	return !process.IsExec
}

// GetProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *EBPFLessFieldHandlers) GetProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	ev.ProcessCacheEntry = fh.resolvers.ProcessResolver.Resolve(sprocess.CacheResolverKey{
		Pid:  ev.PIDContext.Pid,
		NSID: ev.PIDContext.NSID,
	})
	if ev.ProcessCacheEntry == nil {
		ev.ProcessCacheEntry = model.GetPlaceholderProcessCacheEntry(ev.PIDContext.Pid, ev.PIDContext.Pid, false)
		return ev.ProcessCacheEntry, false
	}
	return ev.ProcessCacheEntry, true
}

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *EBPFLessFieldHandlers) ResolveEventTime(ev *model.Event, _ *model.BaseEvent) time.Time {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	return ev.Timestamp
}

// ResolveCGroupID resolves the cgroup ID of the event
func (fh *EBPFLessFieldHandlers) ResolveCGroupID(_ *model.Event, _ *model.CGroupContext) string {
	return ""
}

// ResolveCGroupManager resolves the manager of the cgroup
func (fh *EBPFLessFieldHandlers) ResolveCGroupManager(_ *model.Event, _ *model.CGroupContext) string {
	return ""
}

// ResolveContainerContext retrieve the ContainerContext of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	return ev.ContainerContext, ev.ContainerContext != nil
}

// ResolveContainerRuntime retrieves the container runtime managing the container
func (fh *EBPFLessFieldHandlers) ResolveContainerRuntime(_ *model.Event, _ *model.ContainerContext) string {
	return ""
}

// ResolveContainerID resolves the container ID of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ContainerID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev); entry != nil {
			e.ContainerID = containerutils.ContainerID(entry.ContainerID)
		}
	}
	return string(e.ContainerID)
}

// ResolveContainerCreatedAt resolves the container creation time of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerCreatedAt(ev *model.Event, e *model.ContainerContext) int {
	if e.CreatedAt == 0 {
		if containerContext, _ := fh.ResolveContainerContext(ev); containerContext != nil {
			e.CreatedAt = containerContext.CreatedAt
		}
	}
	return int(e.CreatedAt)
}

// ResolveContainerTags resolves the container tags of the event
func (fh *EBPFLessFieldHandlers) ResolveContainerTags(_ *model.Event, e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ContainerID != "" {
		e.Tags = fh.resolvers.TagsResolver.Resolve(string(e.ContainerID))
	}
	return e.Tags
}

// ResolveProcessContainerID resolves the container ID of the event
func (fh *EBPFLessFieldHandlers) ResolveProcessContainerID(ev *model.Event, _ *model.Process) string {
	return fh.ResolveContainerID(ev, ev.ContainerContext)
}

// ResolveProcessCreatedAt resolves process creation time
func (fh *EBPFLessFieldHandlers) ResolveProcessCreatedAt(_ *model.Event, e *model.Process) int {
	return int(e.ExecTime.UnixNano())
}

// ResolveAsync resolves the async flag
func (fh *EBPFLessFieldHandlers) ResolveAsync(ev *model.Event) bool { return ev.Async }

// ResolveChownGID resolves the ResolveProcessCacheEntry group id of a chown event to a username
func (fh *EBPFLessFieldHandlers) ResolveChownGID(_ *model.Event, e *model.ChownEvent) string {
	return e.Group
}

// ResolveChownUID resolves the ResolveProcessCacheEntry id of a chown event to a username
func (fh *EBPFLessFieldHandlers) ResolveChownUID(_ *model.Event, e *model.ChownEvent) string {
	return e.User
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *EBPFLessFieldHandlers) ResolveEventTimestamp(_ *model.Event, e *model.BaseEvent) int {
	return int(e.TimestampRaw)
}

// ResolveFileFieldsGroup resolves the group id of the file to a group name
func (fh *EBPFLessFieldHandlers) ResolveFileFieldsGroup(_ *model.Event, e *model.FileFields) string {
	return e.Group
}

// ResolveFileFieldsInUpperLayer resolves whether the file is in an upper layer
func (fh *EBPFLessFieldHandlers) ResolveFileFieldsInUpperLayer(_ *model.Event, e *model.FileFields) bool {
	return e.InUpperLayer
}

// ResolveFileFieldsUser resolves the user id of the file to a username
func (fh *EBPFLessFieldHandlers) ResolveFileFieldsUser(_ *model.Event, e *model.FileFields) string {
	return e.User
}

// ResolveFileFilesystem resolves the filesystem a file resides in
func (fh *EBPFLessFieldHandlers) ResolveFileFilesystem(_ *model.Event, e *model.FileEvent) string {
	return e.Filesystem
}

// ResolveK8SGroups resolves the k8s groups of the event
func (fh *EBPFLessFieldHandlers) ResolveK8SGroups(_ *model.Event, e *model.UserSessionContext) []string {
	return e.K8SGroups
}

// ResolveK8SUID resolves the k8s UID of the event
func (fh *EBPFLessFieldHandlers) ResolveK8SUID(_ *model.Event, e *model.UserSessionContext) string {
	return e.K8SUID
}

// ResolveK8SUsername resolves the k8s username of the event
func (fh *EBPFLessFieldHandlers) ResolveK8SUsername(_ *model.Event, e *model.UserSessionContext) string {
	return e.K8SUsername
}

// ResolveModuleArgs resolves the correct args if the arguments were truncated, if not return module.Args
func (fh *EBPFLessFieldHandlers) ResolveModuleArgs(_ *model.Event, e *model.LoadModuleEvent) string {
	return e.Args
}

// ResolveModuleArgv resolves the unscrubbed args of the module as an array. Use with caution.
func (fh *EBPFLessFieldHandlers) ResolveModuleArgv(_ *model.Event, e *model.LoadModuleEvent) []string {
	return e.Argv
}

// ResolveMountPointPath resolves a mount point path
func (fh *EBPFLessFieldHandlers) ResolveMountPointPath(_ *model.Event, e *model.MountEvent) string {
	return e.MountPointPath
}

// ResolveMountRootPath resolves a mount root path
func (fh *EBPFLessFieldHandlers) ResolveMountRootPath(_ *model.Event, e *model.MountEvent) string {
	return e.MountRootPath
}

// ResolveMountSourcePath resolves a mount source path
func (fh *EBPFLessFieldHandlers) ResolveMountSourcePath(_ *model.Event, e *model.MountEvent) string {
	return e.MountSourcePath
}

// ResolveNetworkDeviceIfName returns the network iterface name from the network context
func (fh *EBPFLessFieldHandlers) ResolveNetworkDeviceIfName(_ *model.Event, e *model.NetworkDeviceContext) string {
	return e.IfName
}

// ResolvePackageName resolves the name of the package providing this file
func (fh *EBPFLessFieldHandlers) ResolvePackageName(_ *model.Event, e *model.FileEvent) string {
	return e.PkgName
}

// ResolvePackageSourceVersion resolves the version of the source package of the package providing this file
func (fh *EBPFLessFieldHandlers) ResolvePackageSourceVersion(_ *model.Event, e *model.FileEvent) string {
	return e.PkgSrcVersion
}

// ResolvePackageVersion resolves the version of the package providing this file
func (fh *EBPFLessFieldHandlers) ResolvePackageVersion(_ *model.Event, e *model.FileEvent) string {
	return e.PkgVersion
}

// ResolveRights resolves the rights of a file
func (fh *EBPFLessFieldHandlers) ResolveRights(_ *model.Event, e *model.FileFields) int {
	return int(e.Mode)
}

// ResolveSELinuxBoolName resolves the boolean name of the SELinux event
func (fh *EBPFLessFieldHandlers) ResolveSELinuxBoolName(_ *model.Event, e *model.SELinuxEvent) string {
	return e.BoolName
}

// ResolveSetgidEGroup resolves the effective group of the Setgid event
func (fh *EBPFLessFieldHandlers) ResolveSetgidEGroup(_ *model.Event, e *model.SetgidEvent) string {
	return e.EGroup
}

// ResolveSetgidFSGroup resolves the file-system group of the Setgid event
func (fh *EBPFLessFieldHandlers) ResolveSetgidFSGroup(_ *model.Event, e *model.SetgidEvent) string {
	return e.FSGroup
}

// ResolveSetgidGroup resolves the group of the Setgid event
func (fh *EBPFLessFieldHandlers) ResolveSetgidGroup(_ *model.Event, e *model.SetgidEvent) string {
	return e.Group
}

// ResolveSetuidEUser resolves the effective user of the Setuid event
func (fh *EBPFLessFieldHandlers) ResolveSetuidEUser(_ *model.Event, e *model.SetuidEvent) string {
	return e.EUser
}

// ResolveSetuidFSUser resolves the file-system user of the Setuid event
func (fh *EBPFLessFieldHandlers) ResolveSetuidFSUser(_ *model.Event, e *model.SetuidEvent) string {
	return e.FSUser
}

// ResolveSetuidUser resolves the user of the Setuid event
func (fh *EBPFLessFieldHandlers) ResolveSetuidUser(_ *model.Event, e *model.SetuidEvent) string {
	return e.User
}

// ResolveXAttrName returns the string representation of the extended attribute name
func (fh *EBPFLessFieldHandlers) ResolveXAttrName(_ *model.Event, e *model.SetXAttrEvent) string {
	return e.Name
}

// ResolveXAttrNamespace returns the string representation of the extended attribute namespace
func (fh *EBPFLessFieldHandlers) ResolveXAttrNamespace(_ *model.Event, e *model.SetXAttrEvent) string {
	return e.Namespace
}

// ResolveHashes resolves the hash of the provided file
func (fh *EBPFLessFieldHandlers) ResolveHashes(eventType model.EventType, process *model.Process, file *model.FileEvent) []string {
	return fh.resolvers.HashResolver.ComputeHashes(eventType, process, file)
}

// ResolveHashesFromEvent resolves the hashes of the requested event
func (fh *EBPFLessFieldHandlers) ResolveHashesFromEvent(ev *model.Event, f *model.FileEvent) []string {
	return fh.resolvers.HashResolver.ComputeHashesFromEvent(ev, f)
}

// ResolveUserSessionContext resolves and updates the provided user session context
func (fh *EBPFLessFieldHandlers) ResolveUserSessionContext(_ *model.UserSessionContext) {}

// ResolveProcessCmdArgv resolves the command line
func (fh *EBPFLessFieldHandlers) ResolveProcessCmdArgv(ev *model.Event, process *model.Process) []string {
	cmdline := []string{fh.ResolveProcessArgv0(ev, process)}
	return append(cmdline, fh.ResolveProcessArgv(ev, process)...)
}

// ResolveAWSSecurityCredentials resolves and updates the AWS security credentials of the input process entry
func (fh *EBPFLessFieldHandlers) ResolveAWSSecurityCredentials(_ *model.Event) []model.AWSSecurityCredentials {
	return nil
}

// ResolveSyscallCtxArgs resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgs(_ *model.Event, e *model.SyscallContext) {
	e.Resolved = true
}

// ResolveSyscallCtxArgsStr1 resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgsStr1(_ *model.Event, e *model.SyscallContext) string {
	return e.StrArg1
}

// ResolveSyscallCtxArgsStr2 resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgsStr2(_ *model.Event, e *model.SyscallContext) string {
	return e.StrArg2
}

// ResolveSyscallCtxArgsStr3 resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgsStr3(_ *model.Event, e *model.SyscallContext) string {
	return e.StrArg3
}

// ResolveSyscallCtxArgsInt1 resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgsInt1(_ *model.Event, e *model.SyscallContext) int {
	return int(e.IntArg1)
}

// ResolveSyscallCtxArgsInt2 resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgsInt2(_ *model.Event, e *model.SyscallContext) int {
	return int(e.IntArg2)
}

// ResolveSyscallCtxArgsInt3 resolve syscall ctx
func (fh *EBPFLessFieldHandlers) ResolveSyscallCtxArgsInt3(_ *model.Event, e *model.SyscallContext) int {
	return int(e.IntArg3)
}

// ResolveHostname resolve the hostname
func (fh *EBPFLessFieldHandlers) ResolveHostname(_ *model.Event, _ *model.BaseEvent) string {
	return fh.hostname
}

// ResolveOnDemandName resolves the on-demand event name
func (fh *EBPFLessFieldHandlers) ResolveOnDemandName(_ *model.Event, _ *model.OnDemandEvent) string {
	return ""
}

// ResolveOnDemandArg1Str resolves the string value of the first argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg1Str(_ *model.Event, _ *model.OnDemandEvent) string {
	return ""
}

// ResolveOnDemandArg1Uint resolves the uint value of the first argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg1Uint(_ *model.Event, _ *model.OnDemandEvent) int {
	return 0
}

// ResolveOnDemandArg2Str resolves the string value of the second argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg2Str(_ *model.Event, _ *model.OnDemandEvent) string {
	return ""
}

// ResolveOnDemandArg2Uint resolves the uint value of the second argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg2Uint(_ *model.Event, _ *model.OnDemandEvent) int {
	return 0
}

// ResolveOnDemandArg3Str resolves the string value of the third argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg3Str(_ *model.Event, _ *model.OnDemandEvent) string {
	return ""
}

// ResolveOnDemandArg3Uint resolves the uint value of the third argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg3Uint(_ *model.Event, _ *model.OnDemandEvent) int {
	return 0
}

// ResolveOnDemandArg4Str resolves the string value of the fourth argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg4Str(_ *model.Event, _ *model.OnDemandEvent) string {
	return ""
}

// ResolveOnDemandArg4Uint resolves the uint value of the fourth argument of hooked function
func (fh *EBPFLessFieldHandlers) ResolveOnDemandArg4Uint(_ *model.Event, _ *model.OnDemandEvent) int {
	return 0
}
