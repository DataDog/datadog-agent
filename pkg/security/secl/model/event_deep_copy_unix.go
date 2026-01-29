// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"github.com/google/gopacket"
)

// DeepCopy creates a deep copy of the Event where the copy shares nothing with the original
func (e *Event) DeepCopy() *Event {
	if e == nil {
		return nil
	}
	copied := &Event{}
	copied.Accept = deepCopyAcceptEvent(e.Accept)
	copied.ArgsEnvs = deepCopyArgsEnvsEvent(e.ArgsEnvs)
	copied.Async = e.Async
	copied.BPF = deepCopyBPFEvent(e.BPF)
	copied.BaseEvent = deepCopyBaseEvent(e.BaseEvent)
	copied.Bind = deepCopyBindEvent(e.Bind)
	copied.CapabilitiesUsage = deepCopyCapabilitiesEvent(e.CapabilitiesUsage)
	copied.Capset = deepCopyCapsetEvent(e.Capset)
	copied.CgroupTracing = deepCopyCgroupTracingEvent(e.CgroupTracing)
	copied.CgroupWrite = deepCopyCgroupWriteEvent(e.CgroupWrite)
	copied.Chdir = deepCopyChdirEvent(e.Chdir)
	copied.Chmod = deepCopyChmodEvent(e.Chmod)
	copied.Chown = deepCopyChownEvent(e.Chown)
	copied.Connect = deepCopyConnectEvent(e.Connect)
	copied.DNS = deepCopyDNSEvent(e.DNS)
	copied.Exec = deepCopyExecEvent(e.Exec)
	copied.Exit = deepCopyExitEvent(e.Exit)
	copied.FailedDNS = deepCopyFailedDNSEvent(e.FailedDNS)
	copied.IMDS = deepCopyIMDSEvent(e.IMDS)
	copied.InvalidateDentry = deepCopyInvalidateDentryEvent(e.InvalidateDentry)
	copied.Link = deepCopyLinkEvent(e.Link)
	copied.LoadModule = deepCopyLoadModuleEvent(e.LoadModule)
	copied.LoginUIDWrite = deepCopyLoginUIDWriteEvent(e.LoginUIDWrite)
	copied.MMap = deepCopyMMapEvent(e.MMap)
	copied.MProtect = deepCopyMProtectEvent(e.MProtect)
	copied.Mkdir = deepCopyMkdirEvent(e.Mkdir)
	copied.Mount = deepCopyMountEvent(e.Mount)
	copied.MountReleased = deepCopyMountReleasedEvent(e.MountReleased)
	copied.NetDevice = deepCopyNetDeviceEvent(e.NetDevice)
	copied.NetworkContext = deepCopyNetworkContext(e.NetworkContext)
	copied.NetworkFlowMonitor = deepCopyNetworkFlowMonitorEvent(e.NetworkFlowMonitor)
	copied.OnDemand = deepCopyOnDemandEvent(e.OnDemand)
	copied.Open = deepCopyOpenEvent(e.Open)
	copied.PTrace = deepCopyPTraceEvent(e.PTrace)
	copied.PrCtl = deepCopyPrCtlEvent(e.PrCtl)
	copied.RawPacket = deepCopyRawPacketEvent(e.RawPacket)
	copied.RemoveXAttr = deepCopySetXAttrEvent(e.RemoveXAttr)
	copied.Rename = deepCopyRenameEvent(e.Rename)
	copied.Rmdir = deepCopyRmdirEvent(e.Rmdir)
	copied.SELinux = deepCopySELinuxEvent(e.SELinux)
	copied.SetGID = deepCopySetgidEvent(e.SetGID)
	copied.SetSockOpt = deepCopySetSockOptEvent(e.SetSockOpt)
	copied.SetUID = deepCopySetuidEvent(e.SetUID)
	copied.SetXAttr = deepCopySetXAttrEvent(e.SetXAttr)
	copied.Setrlimit = deepCopySetrlimitEvent(e.Setrlimit)
	copied.Signal = deepCopySignalEvent(e.Signal)
	copied.Signature = e.Signature
	copied.SpanContext = deepCopySpanContext(e.SpanContext)
	copied.Splice = deepCopySpliceEvent(e.Splice)
	copied.StartTime = e.StartTime
	copied.SysCtl = deepCopySysCtlEvent(e.SysCtl)
	copied.Syscalls = deepCopySyscallsEvent(e.Syscalls)
	copied.TracerMemfdSeal = deepCopyTracerMemfdSealEvent(e.TracerMemfdSeal)
	copied.Umount = deepCopyUmountEvent(e.Umount)
	copied.Unlink = deepCopyUnlinkEvent(e.Unlink)
	copied.UnloadModule = deepCopyUnloadModuleEvent(e.UnloadModule)
	copied.UnshareMountNS = deepCopyUnshareMountNSEvent(e.UnshareMountNS)
	copied.Utimes = deepCopyUtimesEvent(e.Utimes)
	copied.VethPair = deepCopyVethPairEvent(e.VethPair)
	// FieldHandlers is an interface that must be copied by reference (not deep copied)
	// It provides access to shared resolvers needed for field resolution
	copied.FieldHandlers = e.FieldHandlers
	return copied
}
func deepCopyAcceptEvent(fieldToCopy AcceptEvent) AcceptEvent {
	copied := AcceptEvent{}
	copied.Addr = deepCopyIPPortContext(fieldToCopy.Addr)
	copied.AddrFamily = fieldToCopy.AddrFamily
	copied.Hostnames = deepCopystringArr(fieldToCopy.Hostnames)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyIPPortContext(fieldToCopy IPPortContext) IPPortContext {
	copied := IPPortContext{}
	copied.IPNet = fieldToCopy.IPNet
	copied.IsPublic = fieldToCopy.IsPublic
	copied.IsPublicResolved = fieldToCopy.IsPublicResolved
	copied.Port = fieldToCopy.Port
	return copied
}
func deepCopystringArr(fieldToCopy []string) []string {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]string, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopySyscallEvent(fieldToCopy SyscallEvent) SyscallEvent {
	copied := SyscallEvent{}
	copied.Retval = fieldToCopy.Retval
	return copied
}
func deepCopyArgsEnvsEvent(fieldToCopy ArgsEnvsEvent) ArgsEnvsEvent {
	copied := ArgsEnvsEvent{}
	copied.ArgsEnvs = deepCopyArgsEnvs(fieldToCopy.ArgsEnvs)
	return copied
}
func deepCopyArgsEnvs(fieldToCopy ArgsEnvs) ArgsEnvs {
	copied := ArgsEnvs{}
	copied.ID = fieldToCopy.ID
	copied.Size = fieldToCopy.Size
	copied.ValuesRaw = fieldToCopy.ValuesRaw
	return copied
}
func deepCopyBPFEvent(fieldToCopy BPFEvent) BPFEvent {
	copied := BPFEvent{}
	copied.Cmd = fieldToCopy.Cmd
	copied.Map = deepCopyBPFMap(fieldToCopy.Map)
	copied.Program = deepCopyBPFProgram(fieldToCopy.Program)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyBPFMap(fieldToCopy BPFMap) BPFMap {
	copied := BPFMap{}
	copied.ID = fieldToCopy.ID
	copied.Name = fieldToCopy.Name
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyBPFProgram(fieldToCopy BPFProgram) BPFProgram {
	copied := BPFProgram{}
	copied.AttachType = fieldToCopy.AttachType
	copied.Helpers = deepCopyuint32Arr(fieldToCopy.Helpers)
	copied.ID = fieldToCopy.ID
	copied.Name = fieldToCopy.Name
	copied.Tag = fieldToCopy.Tag
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyuint32Arr(fieldToCopy []uint32) []uint32 {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]uint32, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyBaseEvent(fieldToCopy BaseEvent) BaseEvent {
	copied := BaseEvent{}
	copied.Flags = fieldToCopy.Flags
	copied.Hostname = fieldToCopy.Hostname
	copied.ID = fieldToCopy.ID
	copied.Origin = fieldToCopy.Origin
	copied.Os = fieldToCopy.Os
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext)
	copied.ProcessCacheEntry = deepCopyProcessCacheEntryPtr(fieldToCopy.ProcessCacheEntry)
	copied.ProcessContext = deepCopyProcessContextPtr(fieldToCopy.ProcessContext)
	copied.RuleContext = deepCopyRuleContext(fieldToCopy.RuleContext)
	copied.RuleTags = deepCopystringArr(fieldToCopy.RuleTags)
	copied.Rules = deepCopyMatchedRulePtrArr(fieldToCopy.Rules)
	copied.SecurityProfileContext = deepCopySecurityProfileContext(fieldToCopy.SecurityProfileContext)
	copied.Service = fieldToCopy.Service
	copied.Source = fieldToCopy.Source
	copied.Timestamp = fieldToCopy.Timestamp
	copied.TimestampRaw = fieldToCopy.TimestampRaw
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyPIDContext(fieldToCopy PIDContext) PIDContext {
	copied := PIDContext{}
	copied.ExecInode = fieldToCopy.ExecInode
	copied.IsKworker = fieldToCopy.IsKworker
	copied.MntNS = fieldToCopy.MntNS
	copied.NSID = fieldToCopy.NSID
	copied.NetNS = fieldToCopy.NetNS
	copied.Pid = fieldToCopy.Pid
	copied.Tid = fieldToCopy.Tid
	copied.UserSessionID = fieldToCopy.UserSessionID
	return copied
}
func deepCopyProcessCacheEntryPtr(fieldToCopy *ProcessCacheEntry) *ProcessCacheEntry {
	if fieldToCopy == nil {
		return nil
	}
	copied := &ProcessCacheEntry{}
	copied.ProcessContext = deepCopyProcessContext(fieldToCopy.ProcessContext)
	return copied
}
func deepCopyProcessContext(fieldToCopy ProcessContext) ProcessContext {
	copied := ProcessContext{}
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent)
	copied.Process = deepCopyProcess(fieldToCopy.Process)
	return copied
}
func deepCopyProcessPtr(fieldToCopy *Process) *Process {
	if fieldToCopy == nil {
		return nil
	}
	copied := &Process{}
	copied.AWSSecurityCredentials = deepCopyAWSSecurityCredentialsArr(fieldToCopy.AWSSecurityCredentials)
	copied.Args = fieldToCopy.Args
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry)
	copied.ArgsID = fieldToCopy.ArgsID
	copied.ArgsScrubbed = fieldToCopy.ArgsScrubbed
	copied.ArgsTruncated = fieldToCopy.ArgsTruncated
	copied.Argv = deepCopystringArr(fieldToCopy.Argv)
	copied.Argv0 = fieldToCopy.Argv0
	copied.ArgvScrubbed = deepCopystringArr(fieldToCopy.ArgvScrubbed)
	copied.CGroup = deepCopyCGroupContext(fieldToCopy.CGroup)
	copied.CapsAttempted = fieldToCopy.CapsAttempted
	copied.CapsUsed = fieldToCopy.CapsUsed
	copied.Comm = fieldToCopy.Comm
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext)
	copied.Cookie = fieldToCopy.Cookie
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Credentials = deepCopyCredentials(fieldToCopy.Credentials)
	copied.Envp = deepCopystringArr(fieldToCopy.Envp)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry)
	copied.EnvsID = fieldToCopy.EnvsID
	copied.EnvsTruncated = fieldToCopy.EnvsTruncated
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent)
	copied.ForkTime = fieldToCopy.ForkTime
	copied.IsExec = fieldToCopy.IsExec
	copied.IsExecExec = fieldToCopy.IsExecExec
	copied.IsParentMissing = fieldToCopy.IsParentMissing
	copied.IsThread = fieldToCopy.IsThread
	copied.IsThroughSymLink = fieldToCopy.IsThroughSymLink
	copied.LinuxBinprm = deepCopyLinuxBinprm(fieldToCopy.LinuxBinprm)
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext)
	copied.PPid = fieldToCopy.PPid
	copied.Source = fieldToCopy.Source
	copied.SpanID = fieldToCopy.SpanID
	copied.SymlinkBasenameStr = fieldToCopy.SymlinkBasenameStr
	copied.SymlinkPathnameStr = fieldToCopy.SymlinkPathnameStr
	copied.TTYName = fieldToCopy.TTYName
	copied.TraceID = deepCopyTraceID(fieldToCopy.TraceID)
	copied.TracerTags = deepCopystringArr(fieldToCopy.TracerTags)
	copied.UserSession = deepCopyUserSessionContext(fieldToCopy.UserSession)
	return copied
}
func deepCopyAWSSecurityCredentialsArr(fieldToCopy []AWSSecurityCredentials) []AWSSecurityCredentials {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]AWSSecurityCredentials, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = deepCopyAWSSecurityCredentials(fieldToCopy[i])
	}
	return copied
}
func deepCopyAWSSecurityCredentials(fieldToCopy AWSSecurityCredentials) AWSSecurityCredentials {
	copied := AWSSecurityCredentials{}
	copied.AccessKeyID = fieldToCopy.AccessKeyID
	copied.Code = fieldToCopy.Code
	copied.Expiration = fieldToCopy.Expiration
	copied.ExpirationRaw = fieldToCopy.ExpirationRaw
	copied.LastUpdated = fieldToCopy.LastUpdated
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyArgsEntryPtr(fieldToCopy *ArgsEntry) *ArgsEntry {
	if fieldToCopy == nil {
		return nil
	}
	copied := &ArgsEntry{}
	copied.ScrubbedResolved = fieldToCopy.ScrubbedResolved
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values)
	return copied
}
func deepCopyCGroupContext(fieldToCopy CGroupContext) CGroupContext {
	copied := CGroupContext{}
	copied.CGroupID = fieldToCopy.CGroupID
	copied.CGroupPathKey = deepCopyPathKey(fieldToCopy.CGroupPathKey)
	copied.CGroupVersion = fieldToCopy.CGroupVersion
	copied.Releasable = deepCopyReleasablePtr(fieldToCopy.Releasable)
	return copied
}
func deepCopyPathKey(fieldToCopy PathKey) PathKey {
	copied := PathKey{}
	copied.Inode = fieldToCopy.Inode
	copied.MountID = fieldToCopy.MountID
	copied.PathID = fieldToCopy.PathID
	return copied
}
func deepCopyReleasablePtr(fieldToCopy *Releasable) *Releasable {
	if fieldToCopy == nil {
		return nil
	}
	copied := &Releasable{}
	return copied
}
func deepCopyContainerContext(fieldToCopy ContainerContext) ContainerContext {
	copied := ContainerContext{}
	copied.ContainerID = fieldToCopy.ContainerID
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Releasable = deepCopyReleasablePtr(fieldToCopy.Releasable)
	copied.Tags = deepCopystringArr(fieldToCopy.Tags)
	return copied
}
func deepCopyCredentials(fieldToCopy Credentials) Credentials {
	copied := Credentials{}
	copied.AUID = fieldToCopy.AUID
	copied.CapEffective = fieldToCopy.CapEffective
	copied.CapPermitted = fieldToCopy.CapPermitted
	copied.EGID = fieldToCopy.EGID
	copied.EGroup = fieldToCopy.EGroup
	copied.EUID = fieldToCopy.EUID
	copied.EUser = fieldToCopy.EUser
	copied.FSGID = fieldToCopy.FSGID
	copied.FSGroup = fieldToCopy.FSGroup
	copied.FSUID = fieldToCopy.FSUID
	copied.FSUser = fieldToCopy.FSUser
	copied.GID = fieldToCopy.GID
	copied.Group = fieldToCopy.Group
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyEnvsEntryPtr(fieldToCopy *EnvsEntry) *EnvsEntry {
	if fieldToCopy == nil {
		return nil
	}
	copied := &EnvsEntry{}
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values)
	return copied
}
func deepCopyFileEvent(fieldToCopy FileEvent) FileEvent {
	copied := FileEvent{}
	copied.BasenameStr = fieldToCopy.BasenameStr
	copied.Extension = fieldToCopy.Extension
	copied.FileFields = deepCopyFileFields(fieldToCopy.FileFields)
	copied.Filesystem = fieldToCopy.Filesystem
	copied.HashState = fieldToCopy.HashState
	copied.Hashes = deepCopystringArr(fieldToCopy.Hashes)
	copied.IsBasenameStrResolved = fieldToCopy.IsBasenameStrResolved
	copied.IsPathnameStrResolved = fieldToCopy.IsPathnameStrResolved
	copied.MountDetached = fieldToCopy.MountDetached
	copied.MountOrigin = fieldToCopy.MountOrigin
	copied.MountPath = fieldToCopy.MountPath
	copied.MountSource = fieldToCopy.MountSource
	copied.MountVisibilityResolved = fieldToCopy.MountVisibilityResolved
	copied.MountVisible = fieldToCopy.MountVisible
	copied.PathnameStr = fieldToCopy.PathnameStr
	copied.PkgEpoch = fieldToCopy.PkgEpoch
	copied.PkgName = fieldToCopy.PkgName
	copied.PkgRelease = fieldToCopy.PkgRelease
	copied.PkgSrcEpoch = fieldToCopy.PkgSrcEpoch
	copied.PkgSrcRelease = fieldToCopy.PkgSrcRelease
	copied.PkgSrcVersion = fieldToCopy.PkgSrcVersion
	copied.PkgVersion = fieldToCopy.PkgVersion
	return copied
}
func deepCopyFileFields(fieldToCopy FileFields) FileFields {
	copied := FileFields{}
	copied.CTime = fieldToCopy.CTime
	copied.Device = fieldToCopy.Device
	copied.Flags = fieldToCopy.Flags
	copied.GID = fieldToCopy.GID
	copied.Group = fieldToCopy.Group
	copied.InUpperLayer = fieldToCopy.InUpperLayer
	copied.MTime = fieldToCopy.MTime
	copied.Mode = fieldToCopy.Mode
	copied.NLink = fieldToCopy.NLink
	copied.PathKey = deepCopyPathKey(fieldToCopy.PathKey)
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyLinuxBinprm(fieldToCopy LinuxBinprm) LinuxBinprm {
	copied := LinuxBinprm{}
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent)
	return copied
}
func deepCopyTraceID(fieldToCopy utils.TraceID) utils.TraceID {
	copied := utils.TraceID{}
	copied.Hi = fieldToCopy.Hi
	copied.Lo = fieldToCopy.Lo
	return copied
}
func deepCopyUserSessionContext(fieldToCopy UserSessionContext) UserSessionContext {
	copied := UserSessionContext{}
	copied.ID = fieldToCopy.ID
	copied.Identity = fieldToCopy.Identity
	copied.K8SSessionContext = deepCopyK8SSessionContext(fieldToCopy.K8SSessionContext)
	copied.SSHSessionContext = deepCopySSHSessionContext(fieldToCopy.SSHSessionContext)
	copied.SessionType = fieldToCopy.SessionType
	return copied
}
func deepCopyK8SSessionContext(fieldToCopy K8SSessionContext) K8SSessionContext {
	copied := K8SSessionContext{}
	copied.K8SExtra = deepCopystringArrMap(fieldToCopy.K8SExtra)
	copied.K8SGroups = deepCopystringArr(fieldToCopy.K8SGroups)
	copied.K8SResolved = fieldToCopy.K8SResolved
	copied.K8SSessionID = fieldToCopy.K8SSessionID
	copied.K8SUID = fieldToCopy.K8SUID
	copied.K8SUsername = fieldToCopy.K8SUsername
	return copied
}
func deepCopystringArrMap(fieldToCopy map[string][]string) map[string][]string {
	if fieldToCopy == nil {
		return nil
	}
	copied := make(map[string][]string, len(fieldToCopy))
	for k, v := range fieldToCopy {
		copied[k] = v
	}
	return copied
}
func deepCopySSHSessionContext(fieldToCopy SSHSessionContext) SSHSessionContext {
	copied := SSHSessionContext{}
	copied.SSHAuthMethod = fieldToCopy.SSHAuthMethod
	copied.SSHClientIP = fieldToCopy.SSHClientIP
	copied.SSHClientPort = fieldToCopy.SSHClientPort
	copied.SSHDPid = fieldToCopy.SSHDPid
	copied.SSHPublicKey = fieldToCopy.SSHPublicKey
	copied.SSHSessionID = fieldToCopy.SSHSessionID
	return copied
}
func deepCopyProcess(fieldToCopy Process) Process {
	copied := Process{}
	copied.AWSSecurityCredentials = deepCopyAWSSecurityCredentialsArr(fieldToCopy.AWSSecurityCredentials)
	copied.Args = fieldToCopy.Args
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry)
	copied.ArgsID = fieldToCopy.ArgsID
	copied.ArgsScrubbed = fieldToCopy.ArgsScrubbed
	copied.ArgsTruncated = fieldToCopy.ArgsTruncated
	copied.Argv = deepCopystringArr(fieldToCopy.Argv)
	copied.Argv0 = fieldToCopy.Argv0
	copied.ArgvScrubbed = deepCopystringArr(fieldToCopy.ArgvScrubbed)
	copied.CGroup = deepCopyCGroupContext(fieldToCopy.CGroup)
	copied.CapsAttempted = fieldToCopy.CapsAttempted
	copied.CapsUsed = fieldToCopy.CapsUsed
	copied.Comm = fieldToCopy.Comm
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext)
	copied.Cookie = fieldToCopy.Cookie
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Credentials = deepCopyCredentials(fieldToCopy.Credentials)
	copied.Envp = deepCopystringArr(fieldToCopy.Envp)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry)
	copied.EnvsID = fieldToCopy.EnvsID
	copied.EnvsTruncated = fieldToCopy.EnvsTruncated
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent)
	copied.ForkTime = fieldToCopy.ForkTime
	copied.IsExec = fieldToCopy.IsExec
	copied.IsExecExec = fieldToCopy.IsExecExec
	copied.IsParentMissing = fieldToCopy.IsParentMissing
	copied.IsThread = fieldToCopy.IsThread
	copied.IsThroughSymLink = fieldToCopy.IsThroughSymLink
	copied.LinuxBinprm = deepCopyLinuxBinprm(fieldToCopy.LinuxBinprm)
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext)
	copied.PPid = fieldToCopy.PPid
	copied.Source = fieldToCopy.Source
	copied.SpanID = fieldToCopy.SpanID
	copied.SymlinkBasenameStr = fieldToCopy.SymlinkBasenameStr
	copied.SymlinkPathnameStr = fieldToCopy.SymlinkPathnameStr
	copied.TTYName = fieldToCopy.TTYName
	copied.TraceID = deepCopyTraceID(fieldToCopy.TraceID)
	copied.TracerTags = deepCopystringArr(fieldToCopy.TracerTags)
	copied.UserSession = deepCopyUserSessionContext(fieldToCopy.UserSession)
	return copied
}
func deepCopyProcessContextPtr(fieldToCopy *ProcessContext) *ProcessContext {
	if fieldToCopy == nil {
		return nil
	}
	copied := &ProcessContext{}
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent)
	copied.Process = deepCopyProcess(fieldToCopy.Process)
	return copied
}
func deepCopyRuleContext(fieldToCopy RuleContext) RuleContext {
	copied := RuleContext{}
	copied.Expression = fieldToCopy.Expression
	copied.MatchingSubExprs = deepCopyMatchingSubExprArr(fieldToCopy.MatchingSubExprs)
	return copied
}
func deepCopyMatchingSubExprArr(fieldToCopy []eval.MatchingSubExpr) []eval.MatchingSubExpr {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]eval.MatchingSubExpr, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchingSubExpr(fieldToCopy[i])
	}
	return copied
}
func deepCopyMatchingValue(fieldToCopy eval.MatchingValue) eval.MatchingValue {
	copied := eval.MatchingValue{}
	copied.Field = fieldToCopy.Field
	copied.Offset = fieldToCopy.Offset
	return copied
}
func deepCopyMatchingSubExpr(fieldToCopy eval.MatchingSubExpr) eval.MatchingSubExpr {
	copied := eval.MatchingSubExpr{}
	copied.Offset = fieldToCopy.Offset
	copied.ValueA = deepCopyMatchingValue(fieldToCopy.ValueA)
	copied.ValueB = deepCopyMatchingValue(fieldToCopy.ValueB)
	return copied
}
func deepCopyMatchedRulePtrArr(fieldToCopy []*MatchedRule) []*MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]*MatchedRule, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchedRulePtr(fieldToCopy[i])
	}
	return copied
}
func deepCopystringMap(fieldToCopy map[string]string) map[string]string {
	if fieldToCopy == nil {
		return nil
	}
	copied := make(map[string]string, len(fieldToCopy))
	for k, v := range fieldToCopy {
		copied[k] = v
	}
	return copied
}
func deepCopyMatchedRulePtr(fieldToCopy *MatchedRule) *MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	copied := &MatchedRule{}
	copied.PolicyName = fieldToCopy.PolicyName
	copied.PolicyVersion = fieldToCopy.PolicyVersion
	copied.RuleID = fieldToCopy.RuleID
	copied.RuleTags = deepCopystringMap(fieldToCopy.RuleTags)
	copied.RuleVersion = fieldToCopy.RuleVersion
	return copied
}
func deepCopySecurityProfileContext(fieldToCopy SecurityProfileContext) SecurityProfileContext {
	copied := SecurityProfileContext{}
	copied.EventTypeState = fieldToCopy.EventTypeState
	copied.EventTypes = deepCopyEventTypeArr(fieldToCopy.EventTypes)
	copied.Name = fieldToCopy.Name
	copied.Tags = deepCopystringArr(fieldToCopy.Tags)
	copied.Version = fieldToCopy.Version
	return copied
}
func deepCopyEventTypeArr(fieldToCopy []EventType) []EventType {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]EventType, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyBindEvent(fieldToCopy BindEvent) BindEvent {
	copied := BindEvent{}
	copied.Addr = deepCopyIPPortContext(fieldToCopy.Addr)
	copied.AddrFamily = fieldToCopy.AddrFamily
	copied.Protocol = fieldToCopy.Protocol
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyCapabilitiesEvent(fieldToCopy CapabilitiesEvent) CapabilitiesEvent {
	copied := CapabilitiesEvent{}
	copied.Attempted = fieldToCopy.Attempted
	copied.Used = fieldToCopy.Used
	return copied
}
func deepCopyCapsetEvent(fieldToCopy CapsetEvent) CapsetEvent {
	copied := CapsetEvent{}
	copied.CapEffective = fieldToCopy.CapEffective
	copied.CapPermitted = fieldToCopy.CapPermitted
	return copied
}
func deepCopyCgroupTracingEvent(fieldToCopy CgroupTracingEvent) CgroupTracingEvent {
	copied := CgroupTracingEvent{}
	copied.CGroupContext = deepCopyCGroupContext(fieldToCopy.CGroupContext)
	copied.Config = deepCopyActivityDumpLoadConfig(fieldToCopy.Config)
	copied.ConfigCookie = fieldToCopy.ConfigCookie
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext)
	copied.Pid = fieldToCopy.Pid
	return copied
}
func deepCopyActivityDumpLoadConfig(fieldToCopy ActivityDumpLoadConfig) ActivityDumpLoadConfig {
	copied := ActivityDumpLoadConfig{}
	copied.EndTimestampRaw = fieldToCopy.EndTimestampRaw
	copied.Paused = fieldToCopy.Paused
	copied.Rate = fieldToCopy.Rate
	copied.StartTimestampRaw = fieldToCopy.StartTimestampRaw
	copied.Timeout = fieldToCopy.Timeout
	copied.TracedEventTypes = deepCopyEventTypeArr(fieldToCopy.TracedEventTypes)
	copied.WaitListTimestampRaw = fieldToCopy.WaitListTimestampRaw
	return copied
}
func deepCopyCgroupWriteEvent(fieldToCopy CgroupWriteEvent) CgroupWriteEvent {
	copied := CgroupWriteEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Pid = fieldToCopy.Pid
	return copied
}
func deepCopyChdirEvent(fieldToCopy ChdirEvent) ChdirEvent {
	copied := ChdirEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopySyscallContext(fieldToCopy SyscallContext) SyscallContext {
	copied := SyscallContext{}
	copied.ID = fieldToCopy.ID
	copied.IntArg1 = fieldToCopy.IntArg1
	copied.IntArg2 = fieldToCopy.IntArg2
	copied.IntArg3 = fieldToCopy.IntArg3
	copied.Resolved = fieldToCopy.Resolved
	copied.StrArg1 = fieldToCopy.StrArg1
	copied.StrArg2 = fieldToCopy.StrArg2
	copied.StrArg3 = fieldToCopy.StrArg3
	return copied
}
func deepCopyChmodEvent(fieldToCopy ChmodEvent) ChmodEvent {
	copied := ChmodEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Mode = fieldToCopy.Mode
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallMode = fieldToCopy.SyscallMode
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyChownEvent(fieldToCopy ChownEvent) ChownEvent {
	copied := ChownEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.GID = fieldToCopy.GID
	copied.Group = fieldToCopy.Group
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallGID = fieldToCopy.SyscallGID
	copied.SyscallPath = fieldToCopy.SyscallPath
	copied.SyscallUID = fieldToCopy.SyscallUID
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyConnectEvent(fieldToCopy ConnectEvent) ConnectEvent {
	copied := ConnectEvent{}
	copied.Addr = deepCopyIPPortContext(fieldToCopy.Addr)
	copied.AddrFamily = fieldToCopy.AddrFamily
	copied.Hostnames = deepCopystringArr(fieldToCopy.Hostnames)
	copied.Protocol = fieldToCopy.Protocol
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyDNSEvent(fieldToCopy DNSEvent) DNSEvent {
	copied := DNSEvent{}
	copied.ID = fieldToCopy.ID
	copied.Question = deepCopyDNSQuestion(fieldToCopy.Question)
	copied.Response = deepCopyDNSResponsePtr(fieldToCopy.Response)
	return copied
}
func deepCopyDNSQuestion(fieldToCopy DNSQuestion) DNSQuestion {
	copied := DNSQuestion{}
	copied.Class = fieldToCopy.Class
	copied.Count = fieldToCopy.Count
	copied.Name = fieldToCopy.Name
	copied.Size = fieldToCopy.Size
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyDNSResponsePtr(fieldToCopy *DNSResponse) *DNSResponse {
	if fieldToCopy == nil {
		return nil
	}
	copied := &DNSResponse{}
	copied.ResponseCode = fieldToCopy.ResponseCode
	return copied
}
func deepCopyExecEvent(fieldToCopy ExecEvent) ExecEvent {
	copied := ExecEvent{}
	copied.FileMetadata = deepCopyFileMetadata(fieldToCopy.FileMetadata)
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyFileMetadata(fieldToCopy FileMetadata) FileMetadata {
	copied := FileMetadata{}
	copied.ABI = fieldToCopy.ABI
	copied.Architecture = fieldToCopy.Architecture
	copied.Compression = fieldToCopy.Compression
	copied.IsExecutable = fieldToCopy.IsExecutable
	copied.IsGarbleObfuscated = fieldToCopy.IsGarbleObfuscated
	copied.IsUPXPacked = fieldToCopy.IsUPXPacked
	copied.Linkage = fieldToCopy.Linkage
	copied.Resolved = fieldToCopy.Resolved
	copied.Size = fieldToCopy.Size
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyExitEvent(fieldToCopy ExitEvent) ExitEvent {
	copied := ExitEvent{}
	copied.Cause = fieldToCopy.Cause
	copied.Code = fieldToCopy.Code
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process)
	return copied
}
func deepCopyFailedDNSEvent(fieldToCopy FailedDNSEvent) FailedDNSEvent {
	copied := FailedDNSEvent{}
	copied.Payload = deepCopybyteArr(fieldToCopy.Payload)
	return copied
}
func deepCopybyteArr(fieldToCopy []byte) []byte {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]byte, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyIMDSEvent(fieldToCopy IMDSEvent) IMDSEvent {
	copied := IMDSEvent{}
	copied.AWS = deepCopyAWSIMDSEvent(fieldToCopy.AWS)
	copied.CloudProvider = fieldToCopy.CloudProvider
	copied.Host = fieldToCopy.Host
	copied.Server = fieldToCopy.Server
	copied.Type = fieldToCopy.Type
	copied.URL = fieldToCopy.URL
	copied.UserAgent = fieldToCopy.UserAgent
	return copied
}
func deepCopyAWSIMDSEvent(fieldToCopy AWSIMDSEvent) AWSIMDSEvent {
	copied := AWSIMDSEvent{}
	copied.IsIMDSv2 = fieldToCopy.IsIMDSv2
	copied.SecurityCredentials = deepCopyAWSSecurityCredentials(fieldToCopy.SecurityCredentials)
	return copied
}
func deepCopyInvalidateDentryEvent(fieldToCopy InvalidateDentryEvent) InvalidateDentryEvent {
	copied := InvalidateDentryEvent{}
	copied.Inode = fieldToCopy.Inode
	copied.MountID = fieldToCopy.MountID
	return copied
}
func deepCopyLinkEvent(fieldToCopy LinkEvent) LinkEvent {
	copied := LinkEvent{}
	copied.Source = deepCopyFileEvent(fieldToCopy.Source)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallDestinationPath = fieldToCopy.SyscallDestinationPath
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallPath = fieldToCopy.SyscallPath
	copied.Target = deepCopyFileEvent(fieldToCopy.Target)
	return copied
}
func deepCopyLoadModuleEvent(fieldToCopy LoadModuleEvent) LoadModuleEvent {
	copied := LoadModuleEvent{}
	copied.Args = fieldToCopy.Args
	copied.ArgsTruncated = fieldToCopy.ArgsTruncated
	copied.Argv = deepCopystringArr(fieldToCopy.Argv)
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.LoadedFromMemory = fieldToCopy.LoadedFromMemory
	copied.Name = fieldToCopy.Name
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyLoginUIDWriteEvent(fieldToCopy LoginUIDWriteEvent) LoginUIDWriteEvent {
	copied := LoginUIDWriteEvent{}
	copied.AUID = fieldToCopy.AUID
	return copied
}
func deepCopyMMapEvent(fieldToCopy MMapEvent) MMapEvent {
	copied := MMapEvent{}
	copied.Addr = fieldToCopy.Addr
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Flags = fieldToCopy.Flags
	copied.Len = fieldToCopy.Len
	copied.Offset = fieldToCopy.Offset
	copied.Protection = fieldToCopy.Protection
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyMProtectEvent(fieldToCopy MProtectEvent) MProtectEvent {
	copied := MProtectEvent{}
	copied.ReqProtection = fieldToCopy.ReqProtection
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.VMEnd = fieldToCopy.VMEnd
	copied.VMProtection = fieldToCopy.VMProtection
	copied.VMStart = fieldToCopy.VMStart
	return copied
}
func deepCopyMkdirEvent(fieldToCopy MkdirEvent) MkdirEvent {
	copied := MkdirEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Mode = fieldToCopy.Mode
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallMode = fieldToCopy.SyscallMode
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyMountEvent(fieldToCopy MountEvent) MountEvent {
	copied := MountEvent{}
	copied.Mount = deepCopyMount(fieldToCopy.Mount)
	copied.MountPointPath = fieldToCopy.MountPointPath
	copied.MountRootPath = fieldToCopy.MountRootPath
	copied.MountSourcePath = fieldToCopy.MountSourcePath
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallFSType = fieldToCopy.SyscallFSType
	copied.SyscallMountpointPath = fieldToCopy.SyscallMountpointPath
	copied.SyscallSourcePath = fieldToCopy.SyscallSourcePath
	return copied
}
func deepCopyMount(fieldToCopy Mount) Mount {
	copied := Mount{}
	copied.BindSrcMountID = fieldToCopy.BindSrcMountID
	copied.BindSrcMountIDUnique = fieldToCopy.BindSrcMountIDUnique
	copied.Children = deepCopyuint32Arr(fieldToCopy.Children)
	copied.Detached = fieldToCopy.Detached
	copied.Device = fieldToCopy.Device
	copied.FSType = fieldToCopy.FSType
	copied.MountID = fieldToCopy.MountID
	copied.MountIDUnique = fieldToCopy.MountIDUnique
	copied.MountPointStr = fieldToCopy.MountPointStr
	copied.NamespaceInode = fieldToCopy.NamespaceInode
	copied.Origin = fieldToCopy.Origin
	copied.ParentMountIDUnique = fieldToCopy.ParentMountIDUnique
	copied.ParentPathKey = deepCopyPathKey(fieldToCopy.ParentPathKey)
	copied.Path = fieldToCopy.Path
	copied.RootPathKey = deepCopyPathKey(fieldToCopy.RootPathKey)
	copied.RootStr = fieldToCopy.RootStr
	copied.Visible = fieldToCopy.Visible
	return copied
}
func deepCopyMountReleasedEvent(fieldToCopy MountReleasedEvent) MountReleasedEvent {
	copied := MountReleasedEvent{}
	copied.MountID = fieldToCopy.MountID
	copied.MountIDUnique = fieldToCopy.MountIDUnique
	return copied
}
func deepCopyNetDeviceEvent(fieldToCopy NetDeviceEvent) NetDeviceEvent {
	copied := NetDeviceEvent{}
	copied.Device = deepCopyNetDevice(fieldToCopy.Device)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyNetDevice(fieldToCopy NetDevice) NetDevice {
	copied := NetDevice{}
	copied.IfIndex = fieldToCopy.IfIndex
	copied.Name = fieldToCopy.Name
	copied.NetNS = fieldToCopy.NetNS
	copied.PeerIfIndex = fieldToCopy.PeerIfIndex
	copied.PeerNetNS = fieldToCopy.PeerNetNS
	return copied
}
func deepCopyNetworkContext(fieldToCopy NetworkContext) NetworkContext {
	copied := NetworkContext{}
	copied.Destination = deepCopyIPPortContext(fieldToCopy.Destination)
	copied.Device = deepCopyNetworkDeviceContext(fieldToCopy.Device)
	copied.L3Protocol = fieldToCopy.L3Protocol
	copied.L4Protocol = fieldToCopy.L4Protocol
	copied.NetworkDirection = fieldToCopy.NetworkDirection
	copied.Size = fieldToCopy.Size
	copied.Source = deepCopyIPPortContext(fieldToCopy.Source)
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyNetworkDeviceContext(fieldToCopy NetworkDeviceContext) NetworkDeviceContext {
	copied := NetworkDeviceContext{}
	copied.IfIndex = fieldToCopy.IfIndex
	copied.IfName = fieldToCopy.IfName
	copied.NetNS = fieldToCopy.NetNS
	return copied
}
func deepCopyNetworkFlowMonitorEvent(fieldToCopy NetworkFlowMonitorEvent) NetworkFlowMonitorEvent {
	copied := NetworkFlowMonitorEvent{}
	copied.Device = deepCopyNetworkDeviceContext(fieldToCopy.Device)
	copied.Flows = deepCopyFlowArr(fieldToCopy.Flows)
	copied.FlowsCount = fieldToCopy.FlowsCount
	return copied
}
func deepCopyFlowArr(fieldToCopy []Flow) []Flow {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]Flow, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = deepCopyFlow(fieldToCopy[i])
	}
	return copied
}
func deepCopyNetworkStats(fieldToCopy NetworkStats) NetworkStats {
	copied := NetworkStats{}
	copied.DataSize = fieldToCopy.DataSize
	copied.PacketCount = fieldToCopy.PacketCount
	return copied
}
func deepCopyFlow(fieldToCopy Flow) Flow {
	copied := Flow{}
	copied.Destination = deepCopyIPPortContext(fieldToCopy.Destination)
	copied.Egress = deepCopyNetworkStats(fieldToCopy.Egress)
	copied.Ingress = deepCopyNetworkStats(fieldToCopy.Ingress)
	copied.L3Protocol = fieldToCopy.L3Protocol
	copied.L4Protocol = fieldToCopy.L4Protocol
	copied.Source = deepCopyIPPortContext(fieldToCopy.Source)
	return copied
}
func deepCopyOnDemandEvent(fieldToCopy OnDemandEvent) OnDemandEvent {
	copied := OnDemandEvent{}
	copied.Arg1Str = fieldToCopy.Arg1Str
	copied.Arg1Uint = fieldToCopy.Arg1Uint
	copied.Arg2Str = fieldToCopy.Arg2Str
	copied.Arg2Uint = fieldToCopy.Arg2Uint
	copied.Arg3Str = fieldToCopy.Arg3Str
	copied.Arg3Uint = fieldToCopy.Arg3Uint
	copied.Arg4Str = fieldToCopy.Arg4Str
	copied.Arg4Uint = fieldToCopy.Arg4Uint
	copied.Arg5Str = fieldToCopy.Arg5Str
	copied.Arg5Uint = fieldToCopy.Arg5Uint
	copied.Arg6Str = fieldToCopy.Arg6Str
	copied.Arg6Uint = fieldToCopy.Arg6Uint
	copied.Data = fieldToCopy.Data
	copied.ID = fieldToCopy.ID
	copied.Name = fieldToCopy.Name
	return copied
}
func deepCopyOpenEvent(fieldToCopy OpenEvent) OpenEvent {
	copied := OpenEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Flags = fieldToCopy.Flags
	copied.Mode = fieldToCopy.Mode
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallFlags = fieldToCopy.SyscallFlags
	copied.SyscallMode = fieldToCopy.SyscallMode
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyPTraceEvent(fieldToCopy PTraceEvent) PTraceEvent {
	copied := PTraceEvent{}
	copied.Address = fieldToCopy.Address
	copied.NSPID = fieldToCopy.NSPID
	copied.PID = fieldToCopy.PID
	copied.Request = fieldToCopy.Request
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.Tracee = deepCopyProcessContextPtr(fieldToCopy.Tracee)
	return copied
}
func deepCopyPrCtlEvent(fieldToCopy PrCtlEvent) PrCtlEvent {
	copied := PrCtlEvent{}
	copied.IsNameTruncated = fieldToCopy.IsNameTruncated
	copied.NewName = fieldToCopy.NewName
	copied.Option = fieldToCopy.Option
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyRawPacketEvent(fieldToCopy RawPacketEvent) RawPacketEvent {
	copied := RawPacketEvent{}
	copied.CaptureInfo = deepCopyCaptureInfo(fieldToCopy.CaptureInfo)
	copied.Data = deepCopybyteArr(fieldToCopy.Data)
	copied.Filter = fieldToCopy.Filter
	copied.NetworkContext = deepCopyNetworkContext(fieldToCopy.NetworkContext)
	copied.TLSContext = deepCopyTLSContext(fieldToCopy.TLSContext)
	return copied
}
func deepCopyCaptureInfo(fieldToCopy gopacket.CaptureInfo) gopacket.CaptureInfo {
	copied := gopacket.CaptureInfo{}
	copied.CaptureLength = fieldToCopy.CaptureLength
	copied.InterfaceIndex = fieldToCopy.InterfaceIndex
	copied.Length = fieldToCopy.Length
	copied.Timestamp = fieldToCopy.Timestamp
	return copied
}
func deepCopyTLSContext(fieldToCopy TLSContext) TLSContext {
	copied := TLSContext{}
	copied.Version = fieldToCopy.Version
	return copied
}
func deepCopySetXAttrEvent(fieldToCopy SetXAttrEvent) SetXAttrEvent {
	copied := SetXAttrEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Name = fieldToCopy.Name
	copied.NameRaw = fieldToCopy.NameRaw
	copied.Namespace = fieldToCopy.Namespace
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyRenameEvent(fieldToCopy RenameEvent) RenameEvent {
	copied := RenameEvent{}
	copied.New = deepCopyFileEvent(fieldToCopy.New)
	copied.Old = deepCopyFileEvent(fieldToCopy.Old)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallDestinationPath = fieldToCopy.SyscallDestinationPath
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyRmdirEvent(fieldToCopy RmdirEvent) RmdirEvent {
	copied := RmdirEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopySELinuxEvent(fieldToCopy SELinuxEvent) SELinuxEvent {
	copied := SELinuxEvent{}
	copied.BoolChangeValue = fieldToCopy.BoolChangeValue
	copied.BoolCommitValue = fieldToCopy.BoolCommitValue
	copied.BoolName = fieldToCopy.BoolName
	copied.EnforceStatus = fieldToCopy.EnforceStatus
	copied.EventKind = fieldToCopy.EventKind
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	return copied
}
func deepCopySetgidEvent(fieldToCopy SetgidEvent) SetgidEvent {
	copied := SetgidEvent{}
	copied.EGID = fieldToCopy.EGID
	copied.EGroup = fieldToCopy.EGroup
	copied.FSGID = fieldToCopy.FSGID
	copied.FSGroup = fieldToCopy.FSGroup
	copied.GID = fieldToCopy.GID
	copied.Group = fieldToCopy.Group
	return copied
}
func deepCopySetSockOptEvent(fieldToCopy SetSockOptEvent) SetSockOptEvent {
	copied := SetSockOptEvent{}
	copied.FilterHash = fieldToCopy.FilterHash
	copied.FilterInstructions = fieldToCopy.FilterInstructions
	copied.FilterLen = fieldToCopy.FilterLen
	copied.IsFilterTruncated = fieldToCopy.IsFilterTruncated
	copied.Level = fieldToCopy.Level
	copied.OptName = fieldToCopy.OptName
	copied.RawFilter = deepCopybyteArr(fieldToCopy.RawFilter)
	copied.SizeToRead = fieldToCopy.SizeToRead
	copied.SocketFamily = fieldToCopy.SocketFamily
	copied.SocketProtocol = fieldToCopy.SocketProtocol
	copied.SocketType = fieldToCopy.SocketType
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.UsedImmediates = deepCopyintArr(fieldToCopy.UsedImmediates)
	return copied
}
func deepCopyintArr(fieldToCopy []int) []int {
	if fieldToCopy == nil {
		return nil
	}
	copied := make([]int, len(fieldToCopy))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopySetuidEvent(fieldToCopy SetuidEvent) SetuidEvent {
	copied := SetuidEvent{}
	copied.EUID = fieldToCopy.EUID
	copied.EUser = fieldToCopy.EUser
	copied.FSUID = fieldToCopy.FSUID
	copied.FSUser = fieldToCopy.FSUser
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopySetrlimitEvent(fieldToCopy SetrlimitEvent) SetrlimitEvent {
	copied := SetrlimitEvent{}
	copied.Resource = fieldToCopy.Resource
	copied.RlimCur = fieldToCopy.RlimCur
	copied.RlimMax = fieldToCopy.RlimMax
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.Target = deepCopyProcessContextPtr(fieldToCopy.Target)
	copied.TargetPid = fieldToCopy.TargetPid
	return copied
}
func deepCopySignalEvent(fieldToCopy SignalEvent) SignalEvent {
	copied := SignalEvent{}
	copied.PID = fieldToCopy.PID
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.Target = deepCopyProcessContextPtr(fieldToCopy.Target)
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopySpanContext(fieldToCopy SpanContext) SpanContext {
	copied := SpanContext{}
	copied.SpanID = fieldToCopy.SpanID
	copied.TraceID = deepCopyTraceID(fieldToCopy.TraceID)
	return copied
}
func deepCopySpliceEvent(fieldToCopy SpliceEvent) SpliceEvent {
	copied := SpliceEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.PipeEntryFlag = fieldToCopy.PipeEntryFlag
	copied.PipeExitFlag = fieldToCopy.PipeExitFlag
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopySysCtlEvent(fieldToCopy SysCtlEvent) SysCtlEvent {
	copied := SysCtlEvent{}
	copied.Action = fieldToCopy.Action
	copied.FilePosition = fieldToCopy.FilePosition
	copied.Name = fieldToCopy.Name
	copied.NameTruncated = fieldToCopy.NameTruncated
	copied.OldValue = fieldToCopy.OldValue
	copied.OldValueTruncated = fieldToCopy.OldValueTruncated
	copied.Value = fieldToCopy.Value
	copied.ValueTruncated = fieldToCopy.ValueTruncated
	return copied
}
func deepCopySyscallsEvent(fieldToCopy SyscallsEvent) SyscallsEvent {
	copied := SyscallsEvent{}
	copied.EventReason = fieldToCopy.EventReason
	return copied
}
func deepCopyTracerMemfdSealEvent(fieldToCopy TracerMemfdSealEvent) TracerMemfdSealEvent {
	copied := TracerMemfdSealEvent{}
	copied.Fd = fieldToCopy.Fd
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyUmountEvent(fieldToCopy UmountEvent) UmountEvent {
	copied := UmountEvent{}
	copied.MountID = fieldToCopy.MountID
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyUnlinkEvent(fieldToCopy UnlinkEvent) UnlinkEvent {
	copied := UnlinkEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Flags = fieldToCopy.Flags
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallDirFd = fieldToCopy.SyscallDirFd
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallFlags = fieldToCopy.SyscallFlags
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyUnloadModuleEvent(fieldToCopy UnloadModuleEvent) UnloadModuleEvent {
	copied := UnloadModuleEvent{}
	copied.Name = fieldToCopy.Name
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
func deepCopyUnshareMountNSEvent(fieldToCopy UnshareMountNSEvent) UnshareMountNSEvent {
	copied := UnshareMountNSEvent{}
	copied.Mount = deepCopyMount(fieldToCopy.Mount)
	return copied
}
func deepCopyUtimesEvent(fieldToCopy UtimesEvent) UtimesEvent {
	copied := UtimesEvent{}
	copied.Atime = fieldToCopy.Atime
	copied.File = deepCopyFileEvent(fieldToCopy.File)
	copied.Mtime = fieldToCopy.Mtime
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyVethPairEvent(fieldToCopy VethPairEvent) VethPairEvent {
	copied := VethPairEvent{}
	copied.HostDevice = deepCopyNetDevice(fieldToCopy.HostDevice)
	copied.PeerDevice = deepCopyNetDevice(fieldToCopy.PeerDevice)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent)
	return copied
}
