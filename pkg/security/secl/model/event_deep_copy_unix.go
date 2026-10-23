// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

package model

import (
	tracermetadata "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/utils"
	"github.com/google/gopacket"
	"net"
	"unsafe"
)

// deepCopyType identifies the type of a pointer, slice or map. It is part of deepCopyKey because a struct
// and its first field share the same address.
type deepCopyType int

// deepCopyKey identifies a pointer, slice or map of the original.
type deepCopyKey struct {
	ptr unsafe.Pointer
	len int
	typ deepCopyType
}

// deepCopyContext maps the pointers, slices and maps of the original to the memory of their copy, so that
// the ones sharing memory in the original share it in the copy too.
type deepCopyContext map[deepCopyKey]unsafe.Pointer

func pointerKey[T any](p *T, typ deepCopyType) deepCopyKey {
	return deepCopyKey{unsafe.Pointer(p), 0, typ}
}
func sliceKey[T any](s []T, typ deepCopyType) deepCopyKey {
	return deepCopyKey{unsafe.Pointer(unsafe.SliceData(s)), len(s), typ}
}
func mapKey[V any](m map[string]V, typ deepCopyType) deepCopyKey {
	return deepCopyKey{mapPointer(m), 0, typ}
}
func mapPointer[V any](m map[string]V) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&m))
}
func mapFromPointer[V any](p unsafe.Pointer) map[string]V {
	return *(*map[string]V)(unsafe.Pointer(&p))
}
func deepCopyIPNet(fieldToCopy net.IPNet, c deepCopyContext) net.IPNet {
	return net.IPNet{IP: deepCopyIPBytes(fieldToCopy.IP, c), Mask: deepCopyIPBytes(fieldToCopy.Mask, c)}
}
func deepCopyIPBytes(fieldToCopy []byte, c deepCopyContext) []byte {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyIPBytesType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*byte)(copied), len(fieldToCopy))
	}
	copied := make([]byte, len(fieldToCopy))
	copy(copied, fieldToCopy)
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	return copied
}

// DeepCopy creates a deep copy of the Event where the copy shares nothing with the original
func (e *Event) DeepCopy() *Event {
	if e == nil {
		return nil
	}
	copied := &Event{}
	c := make(deepCopyContext, 32)
	copied.Accept = deepCopyAcceptEvent(e.Accept, c)
	copied.ArgsEnvs = deepCopyArgsEnvsEvent(e.ArgsEnvs, c)
	copied.Async = e.Async
	copied.BPF = deepCopyBPFEvent(e.BPF, c)
	copied.BaseEvent = deepCopyBaseEvent(e.BaseEvent, c)
	copied.Bind = deepCopyBindEvent(e.Bind, c)
	copied.CapabilitiesUsage = deepCopyCapabilitiesEvent(e.CapabilitiesUsage, c)
	copied.Capset = deepCopyCapsetEvent(e.Capset, c)
	copied.CgroupTracing = deepCopyCgroupTracingEvent(e.CgroupTracing, c)
	copied.CgroupWrite = deepCopyCgroupWriteEvent(e.CgroupWrite, c)
	copied.Chdir = deepCopyChdirEvent(e.Chdir, c)
	copied.Chmod = deepCopyChmodEvent(e.Chmod, c)
	copied.Chown = deepCopyChownEvent(e.Chown, c)
	copied.Connect = deepCopyConnectEvent(e.Connect, c)
	copied.DNS = deepCopyDNSEvent(e.DNS, c)
	copied.Exec = deepCopyExecEvent(e.Exec, c)
	copied.Exit = deepCopyExitEvent(e.Exit, c)
	copied.FailedDNS = deepCopyFailedDNSEvent(e.FailedDNS, c)
	copied.GoLabels = deepCopyGoLabelsContext(e.GoLabels, c)
	copied.IMDS = deepCopyIMDSEvent(e.IMDS, c)
	copied.InvalidateDentry = deepCopyInvalidateDentryEvent(e.InvalidateDentry, c)
	copied.Link = deepCopyLinkEvent(e.Link, c)
	copied.LoadModule = deepCopyLoadModuleEvent(e.LoadModule, c)
	copied.LoginUIDWrite = deepCopyLoginUIDWriteEvent(e.LoginUIDWrite, c)
	copied.MMap = deepCopyMMapEvent(e.MMap, c)
	copied.MProtect = deepCopyMProtectEvent(e.MProtect, c)
	copied.Mkdir = deepCopyMkdirEvent(e.Mkdir, c)
	copied.Mount = deepCopyMountEvent(e.Mount, c)
	copied.MountReleased = deepCopyMountReleasedEvent(e.MountReleased, c)
	copied.NetDevice = deepCopyNetDeviceEvent(e.NetDevice, c)
	copied.NetworkContext = deepCopyNetworkContext(e.NetworkContext, c)
	copied.NetworkFlowMonitor = deepCopyNetworkFlowMonitorEvent(e.NetworkFlowMonitor, c)
	copied.OnDemand = deepCopyOnDemandEvent(e.OnDemand, c)
	copied.Open = deepCopyOpenEvent(e.Open, c)
	copied.PTrace = deepCopyPTraceEvent(e.PTrace, c)
	copied.PrCtl = deepCopyPrCtlEvent(e.PrCtl, c)
	copied.RawPacket = deepCopyRawPacketEvent(e.RawPacket, c)
	copied.RemoveXAttr = deepCopySetXAttrEvent(e.RemoveXAttr, c)
	copied.Rename = deepCopyRenameEvent(e.Rename, c)
	copied.Rmdir = deepCopyRmdirEvent(e.Rmdir, c)
	copied.SELinux = deepCopySELinuxEvent(e.SELinux, c)
	copied.SetGID = deepCopySetgidEvent(e.SetGID, c)
	copied.SetSockOpt = deepCopySetSockOptEvent(e.SetSockOpt, c)
	copied.SetUID = deepCopySetuidEvent(e.SetUID, c)
	copied.SetXAttr = deepCopySetXAttrEvent(e.SetXAttr, c)
	copied.Setrlimit = deepCopySetrlimitEvent(e.Setrlimit, c)
	copied.Signal = deepCopySignalEvent(e.Signal, c)
	copied.Signature = e.Signature
	copied.Socket = deepCopySocketEvent(e.Socket, c)
	copied.SpanContext = deepCopySpanContext(e.SpanContext, c)
	copied.Splice = deepCopySpliceEvent(e.Splice, c)
	copied.SysCtl = deepCopySysCtlEvent(e.SysCtl, c)
	copied.Syscalls = deepCopySyscallsEvent(e.Syscalls, c)
	copied.TracerMemfdSeal = deepCopyTracerMemfdSealEvent(e.TracerMemfdSeal, c)
	copied.Umount = deepCopyUmountEvent(e.Umount, c)
	copied.Unlink = deepCopyUnlinkEvent(e.Unlink, c)
	copied.UnloadModule = deepCopyUnloadModuleEvent(e.UnloadModule, c)
	copied.Unshare = deepCopyUnshareEvent(e.Unshare, c)
	copied.UnshareMountNS = deepCopyUnshareMountNSEvent(e.UnshareMountNS, c)
	copied.Utimes = deepCopyUtimesEvent(e.Utimes, c)
	copied.VethPair = deepCopyVethPairEvent(e.VethPair, c)
	// FieldHandlers is an interface that must be copied by reference (not deep copied)
	// It provides access to shared resolvers needed for field resolution
	copied.FieldHandlers = e.FieldHandlers
	return copied
}
func deepCopyAcceptEvent(fieldToCopy AcceptEvent, c deepCopyContext) AcceptEvent {
	copied := AcceptEvent{}
	copied.Addr = deepCopyIPPortContext(fieldToCopy.Addr, c)
	copied.AddrFamily = fieldToCopy.AddrFamily
	copied.Hostnames = deepCopystringArr(fieldToCopy.Hostnames, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyIPPortContext(fieldToCopy IPPortContext, c deepCopyContext) IPPortContext {
	copied := IPPortContext{}
	copied.IPNet = deepCopyIPNet(fieldToCopy.IPNet, c)
	copied.IsPublic = fieldToCopy.IsPublic
	copied.IsPublicResolved = fieldToCopy.IsPublicResolved
	copied.Port = fieldToCopy.Port
	return copied
}
func deepCopystringArr(fieldToCopy []string, c deepCopyContext) []string {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopystringArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*string)(copied), len(fieldToCopy))
	}
	copied := make([]string, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopySyscallEvent(fieldToCopy SyscallEvent, c deepCopyContext) SyscallEvent {
	copied := SyscallEvent{}
	copied.Retval = fieldToCopy.Retval
	return copied
}
func deepCopyArgsEnvsEvent(fieldToCopy ArgsEnvsEvent, c deepCopyContext) ArgsEnvsEvent {
	copied := ArgsEnvsEvent{}
	copied.ArgsEnvs = deepCopyArgsEnvs(fieldToCopy.ArgsEnvs, c)
	return copied
}
func deepCopyArgsEnvs(fieldToCopy ArgsEnvs, c deepCopyContext) ArgsEnvs {
	copied := ArgsEnvs{}
	copied.ID = fieldToCopy.ID
	copied.Size = fieldToCopy.Size
	copied.ValuesRaw = fieldToCopy.ValuesRaw
	return copied
}
func deepCopyBPFEvent(fieldToCopy BPFEvent, c deepCopyContext) BPFEvent {
	copied := BPFEvent{}
	copied.Cmd = fieldToCopy.Cmd
	copied.Map = deepCopyBPFMap(fieldToCopy.Map, c)
	copied.Program = deepCopyBPFProgram(fieldToCopy.Program, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyBPFMap(fieldToCopy BPFMap, c deepCopyContext) BPFMap {
	copied := BPFMap{}
	copied.ID = fieldToCopy.ID
	copied.Name = fieldToCopy.Name
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyBPFProgram(fieldToCopy BPFProgram, c deepCopyContext) BPFProgram {
	copied := BPFProgram{}
	copied.AttachType = fieldToCopy.AttachType
	copied.Helpers = deepCopyuint32Arr(fieldToCopy.Helpers, c)
	copied.ID = fieldToCopy.ID
	copied.Name = fieldToCopy.Name
	copied.Tag = fieldToCopy.Tag
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyuint32Arr(fieldToCopy []uint32, c deepCopyContext) []uint32 {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyuint32ArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*uint32)(copied), len(fieldToCopy))
	}
	copied := make([]uint32, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyBaseEvent(fieldToCopy BaseEvent, c deepCopyContext) BaseEvent {
	copied := BaseEvent{}
	copied.Flags = fieldToCopy.Flags
	copied.Hostname = fieldToCopy.Hostname
	copied.ID = fieldToCopy.ID
	copied.Origin = fieldToCopy.Origin
	copied.Os = fieldToCopy.Os
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext, c)
	copied.ProcessCacheEntry = deepCopyProcessCacheEntryPtr(fieldToCopy.ProcessCacheEntry, c)
	copied.ProcessContext = deepCopyProcessContextPtr(fieldToCopy.ProcessContext, c)
	copied.RuleContext = deepCopyRuleContext(fieldToCopy.RuleContext, c)
	copied.RuleTags = deepCopystringArr(fieldToCopy.RuleTags, c)
	copied.Rules = deepCopyMatchedRulePtrArr(fieldToCopy.Rules, c)
	copied.SecurityProfileContext = deepCopySecurityProfileContext(fieldToCopy.SecurityProfileContext, c)
	copied.Service = fieldToCopy.Service
	copied.Source = fieldToCopy.Source
	copied.Timestamp = fieldToCopy.Timestamp
	copied.TimestampRaw = fieldToCopy.TimestampRaw
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyPIDContext(fieldToCopy PIDContext, c deepCopyContext) PIDContext {
	copied := PIDContext{}
	copied.ExecInode = fieldToCopy.ExecInode
	copied.IsKworker = fieldToCopy.IsKworker
	copied.MntNS = fieldToCopy.MntNS
	copied.NSID = fieldToCopy.NSID
	copied.NetNS = fieldToCopy.NetNS
	copied.PPid = fieldToCopy.PPid
	copied.Pid = fieldToCopy.Pid
	copied.SID = fieldToCopy.SID
	copied.Tid = fieldToCopy.Tid
	copied.UserSessionID = fieldToCopy.UserSessionID
	return copied
}
func deepCopyProcessCacheEntryPtr(fieldToCopy *ProcessCacheEntry, c deepCopyContext) *ProcessCacheEntry {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyProcessCacheEntryPtrType)
	if copied, ok := c[key]; ok {
		return (*ProcessCacheEntry)(copied)
	}
	copied := &ProcessCacheEntry{}
	c[key] = unsafe.Pointer(copied)
	c[pointerKey(&fieldToCopy.ProcessContext, deepCopyProcessContextPtrType)] = unsafe.Pointer(&copied.ProcessContext)
	c[pointerKey(&fieldToCopy.ProcessContext.Process, deepCopyProcessPtrType)] = unsafe.Pointer(&copied.ProcessContext.Process)
	copied.ProcessContext = deepCopyProcessContext(fieldToCopy.ProcessContext, c)
	return copied
}
func deepCopyProcessContext(fieldToCopy ProcessContext, c deepCopyContext) ProcessContext {
	copied := ProcessContext{}
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor, c)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent, c)
	copied.Process = deepCopyProcess(fieldToCopy.Process, c)
	return copied
}
func deepCopyProcessPtr(fieldToCopy *Process, c deepCopyContext) *Process {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyProcessPtrType)
	if copied, ok := c[key]; ok {
		return (*Process)(copied)
	}
	copied := &Process{}
	c[key] = unsafe.Pointer(copied)
	copied.AWSSecurityCredentials = deepCopyAWSSecurityCredentialsArr(fieldToCopy.AWSSecurityCredentials, c)
	copied.Args = fieldToCopy.Args
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry, c)
	copied.ArgsID = fieldToCopy.ArgsID
	copied.ArgsScrubbed = fieldToCopy.ArgsScrubbed
	copied.ArgsTruncated = fieldToCopy.ArgsTruncated
	copied.Argv = deepCopystringArr(fieldToCopy.Argv, c)
	copied.Argv0 = fieldToCopy.Argv0
	copied.ArgvScrubbed = deepCopystringArr(fieldToCopy.ArgvScrubbed, c)
	copied.CGroup = deepCopyCGroupContext(fieldToCopy.CGroup, c)
	copied.CapsAttempted = fieldToCopy.CapsAttempted
	copied.CapsAttemptedHostUserNS = fieldToCopy.CapsAttemptedHostUserNS
	copied.CapsUsed = fieldToCopy.CapsUsed
	copied.CapsUsedHostUserNS = fieldToCopy.CapsUsedHostUserNS
	copied.Comm = fieldToCopy.Comm
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext, c)
	copied.Cookie = fieldToCopy.Cookie
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Credentials = deepCopyCredentials(fieldToCopy.Credentials, c)
	copied.Envp = deepCopystringArr(fieldToCopy.Envp, c)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs, c)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry, c)
	copied.EnvsID = fieldToCopy.EnvsID
	copied.EnvsTruncated = fieldToCopy.EnvsTruncated
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent, c)
	copied.ForkFlags = fieldToCopy.ForkFlags
	copied.ForkTime = fieldToCopy.ForkTime
	copied.IsExec = fieldToCopy.IsExec
	copied.IsExecExec = fieldToCopy.IsExecExec
	copied.IsParentMissing = fieldToCopy.IsParentMissing
	copied.IsThread = fieldToCopy.IsThread
	copied.IsThroughSymLink = fieldToCopy.IsThroughSymLink
	copied.LinuxBinprm = deepCopyLinuxBinprm(fieldToCopy.LinuxBinprm, c)
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext, c)
	copied.Source = fieldToCopy.Source
	copied.StopExecutionTime = fieldToCopy.StopExecutionTime
	copied.SymlinkBasenameStr = fieldToCopy.SymlinkBasenameStr
	copied.SymlinkPathnameStr = fieldToCopy.SymlinkPathnameStr
	copied.TTYName = fieldToCopy.TTYName
	copied.Tracer = deepCopyTracer(fieldToCopy.Tracer, c)
	copied.UserSession = deepCopyUserSessionContext(fieldToCopy.UserSession, c)
	return copied
}
func deepCopyAWSSecurityCredentialsArr(fieldToCopy []AWSSecurityCredentials, c deepCopyContext) []AWSSecurityCredentials {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyAWSSecurityCredentialsArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*AWSSecurityCredentials)(copied), len(fieldToCopy))
	}
	copied := make([]AWSSecurityCredentials, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyAWSSecurityCredentials(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyAWSSecurityCredentials(fieldToCopy AWSSecurityCredentials, c deepCopyContext) AWSSecurityCredentials {
	copied := AWSSecurityCredentials{}
	copied.AccessKeyID = fieldToCopy.AccessKeyID
	copied.Code = fieldToCopy.Code
	copied.Expiration = fieldToCopy.Expiration
	copied.ExpirationRaw = fieldToCopy.ExpirationRaw
	copied.LastUpdated = fieldToCopy.LastUpdated
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyArgsEntryPtr(fieldToCopy *ArgsEntry, c deepCopyContext) *ArgsEntry {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyArgsEntryPtrType)
	if copied, ok := c[key]; ok {
		return (*ArgsEntry)(copied)
	}
	copied := &ArgsEntry{}
	c[key] = unsafe.Pointer(copied)
	copied.ScrubbedResolved = fieldToCopy.ScrubbedResolved
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values, c)
	return copied
}
func deepCopyCGroupContext(fieldToCopy CGroupContext, c deepCopyContext) CGroupContext {
	copied := CGroupContext{}
	copied.CGroupID = fieldToCopy.CGroupID
	copied.CGroupPathKey = deepCopyPathKey(fieldToCopy.CGroupPathKey, c)
	copied.CGroupSource = fieldToCopy.CGroupSource
	copied.CGroupVersion = fieldToCopy.CGroupVersion
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Releasable = deepCopyReleasablePtr(fieldToCopy.Releasable, c)
	return copied
}
func deepCopyPathKey(fieldToCopy PathKey, c deepCopyContext) PathKey {
	copied := PathKey{}
	copied.Inode = fieldToCopy.Inode
	copied.MountID = fieldToCopy.MountID
	copied.PathID = fieldToCopy.PathID
	return copied
}
func deepCopyReleasablePtr(fieldToCopy *Releasable, c deepCopyContext) *Releasable {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyReleasablePtrType)
	if copied, ok := c[key]; ok {
		return (*Releasable)(copied)
	}
	copied := &Releasable{}
	c[key] = unsafe.Pointer(copied)
	return copied
}
func deepCopyContainerContext(fieldToCopy ContainerContext, c deepCopyContext) ContainerContext {
	copied := ContainerContext{}
	copied.ContainerID = fieldToCopy.ContainerID
	copied.ContainerSource = fieldToCopy.ContainerSource
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.PodUID = fieldToCopy.PodUID
	copied.Releasable = deepCopyReleasablePtr(fieldToCopy.Releasable, c)
	copied.Tags = deepCopystringArr(fieldToCopy.Tags, c)
	return copied
}
func deepCopyCredentials(fieldToCopy Credentials, c deepCopyContext) Credentials {
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
func deepCopyEnvsEntryPtr(fieldToCopy *EnvsEntry, c deepCopyContext) *EnvsEntry {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyEnvsEntryPtrType)
	if copied, ok := c[key]; ok {
		return (*EnvsEntry)(copied)
	}
	copied := &EnvsEntry{}
	c[key] = unsafe.Pointer(copied)
	copied.Truncated = fieldToCopy.Truncated
	copied.Values = deepCopystringArr(fieldToCopy.Values, c)
	return copied
}
func deepCopyFileEvent(fieldToCopy FileEvent, c deepCopyContext) FileEvent {
	copied := FileEvent{}
	copied.BasenameStr = fieldToCopy.BasenameStr
	copied.Extension = fieldToCopy.Extension
	copied.FileFields = deepCopyFileFields(fieldToCopy.FileFields, c)
	copied.Filesystem = fieldToCopy.Filesystem
	copied.HashState = fieldToCopy.HashState
	copied.Hashes = deepCopystringArr(fieldToCopy.Hashes, c)
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
func deepCopyFileFields(fieldToCopy FileFields, c deepCopyContext) FileFields {
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
	copied.PathKey = deepCopyPathKey(fieldToCopy.PathKey, c)
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyLinuxBinprm(fieldToCopy LinuxBinprm, c deepCopyContext) LinuxBinprm {
	copied := LinuxBinprm{}
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent, c)
	return copied
}
func deepCopyTracer(fieldToCopy Tracer, c deepCopyContext) Tracer {
	copied := Tracer{}
	copied.Metadata = deepCopyTracerMetadata(fieldToCopy.Metadata, c)
	copied.MetadataSource = fieldToCopy.MetadataSource
	copied.ThreadlocalAttributeKeys = deepCopystringArr(fieldToCopy.ThreadlocalAttributeKeys, c)
	copied.Trace = deepCopySpanContext(fieldToCopy.Trace, c)
	return copied
}
func deepCopyTracerMetadata(fieldToCopy tracermetadata.TracerMetadata, c deepCopyContext) tracermetadata.TracerMetadata {
	copied := tracermetadata.TracerMetadata{}
	copied.ContainerID = fieldToCopy.ContainerID
	copied.Hostname = fieldToCopy.Hostname
	copied.LogsCollected = fieldToCopy.LogsCollected
	copied.ProcessTags = fieldToCopy.ProcessTags
	copied.RuntimeID = fieldToCopy.RuntimeID
	copied.SchemaVersion = fieldToCopy.SchemaVersion
	copied.ServiceEnv = fieldToCopy.ServiceEnv
	copied.ServiceName = fieldToCopy.ServiceName
	copied.ServiceVersion = fieldToCopy.ServiceVersion
	copied.TracerLanguage = fieldToCopy.TracerLanguage
	copied.TracerVersion = fieldToCopy.TracerVersion
	return copied
}
func deepCopySpanContext(fieldToCopy SpanContext, c deepCopyContext) SpanContext {
	copied := SpanContext{}
	copied.Attributes = deepCopystringMap(fieldToCopy.Attributes, c)
	copied.ExtraAttrsID = fieldToCopy.ExtraAttrsID
	copied.HasExtraAttrs = fieldToCopy.HasExtraAttrs
	copied.SpanID = fieldToCopy.SpanID
	copied.TraceID = deepCopyTraceID(fieldToCopy.TraceID, c)
	return copied
}
func deepCopystringMap(fieldToCopy map[string]string, c deepCopyContext) map[string]string {
	if fieldToCopy == nil {
		return nil
	}
	key := mapKey(fieldToCopy, deepCopystringMapType)
	if copied, ok := c[key]; ok {
		return mapFromPointer[string](copied)
	}
	copied := make(map[string]string, len(fieldToCopy))
	c[key] = mapPointer(copied)
	for k, v := range fieldToCopy {
		copied[k] = v
	}
	return copied
}
func deepCopyTraceID(fieldToCopy utils.TraceID, c deepCopyContext) utils.TraceID {
	copied := utils.TraceID{}
	copied.Hi = fieldToCopy.Hi
	copied.Lo = fieldToCopy.Lo
	return copied
}
func deepCopyUserSessionContext(fieldToCopy UserSessionContext, c deepCopyContext) UserSessionContext {
	copied := UserSessionContext{}
	copied.ID = fieldToCopy.ID
	copied.Identity = fieldToCopy.Identity
	copied.K8SSessionContext = deepCopyK8SSessionContext(fieldToCopy.K8SSessionContext, c)
	copied.SSHSessionContext = deepCopySSHSessionContext(fieldToCopy.SSHSessionContext, c)
	copied.SessionType = fieldToCopy.SessionType
	return copied
}
func deepCopyK8SSessionContext(fieldToCopy K8SSessionContext, c deepCopyContext) K8SSessionContext {
	copied := K8SSessionContext{}
	copied.K8SExtra = deepCopystringArrMap(fieldToCopy.K8SExtra, c)
	copied.K8SGroups = deepCopystringArr(fieldToCopy.K8SGroups, c)
	copied.K8SResolved = fieldToCopy.K8SResolved
	copied.K8SSessionID = fieldToCopy.K8SSessionID
	copied.K8SUID = fieldToCopy.K8SUID
	copied.K8SUsername = fieldToCopy.K8SUsername
	return copied
}
func deepCopystringArrMap(fieldToCopy map[string][]string, c deepCopyContext) map[string][]string {
	if fieldToCopy == nil {
		return nil
	}
	key := mapKey(fieldToCopy, deepCopystringArrMapType)
	if copied, ok := c[key]; ok {
		return mapFromPointer[[]string](copied)
	}
	copied := make(map[string][]string, len(fieldToCopy))
	c[key] = mapPointer(copied)
	for k, v := range fieldToCopy {
		copied[k] = deepCopystringArr(v, c)
	}
	return copied
}
func deepCopySSHSessionContext(fieldToCopy SSHSessionContext, c deepCopyContext) SSHSessionContext {
	copied := SSHSessionContext{}
	copied.SSHAuthMethod = fieldToCopy.SSHAuthMethod
	copied.SSHClientIP = deepCopyIPNet(fieldToCopy.SSHClientIP, c)
	copied.SSHClientPort = fieldToCopy.SSHClientPort
	copied.SSHDPid = fieldToCopy.SSHDPid
	copied.SSHPublicKey = fieldToCopy.SSHPublicKey
	copied.SSHSessionID = fieldToCopy.SSHSessionID
	return copied
}
func deepCopyProcess(fieldToCopy Process, c deepCopyContext) Process {
	copied := Process{}
	copied.AWSSecurityCredentials = deepCopyAWSSecurityCredentialsArr(fieldToCopy.AWSSecurityCredentials, c)
	copied.Args = fieldToCopy.Args
	copied.ArgsEntry = deepCopyArgsEntryPtr(fieldToCopy.ArgsEntry, c)
	copied.ArgsID = fieldToCopy.ArgsID
	copied.ArgsScrubbed = fieldToCopy.ArgsScrubbed
	copied.ArgsTruncated = fieldToCopy.ArgsTruncated
	copied.Argv = deepCopystringArr(fieldToCopy.Argv, c)
	copied.Argv0 = fieldToCopy.Argv0
	copied.ArgvScrubbed = deepCopystringArr(fieldToCopy.ArgvScrubbed, c)
	copied.CGroup = deepCopyCGroupContext(fieldToCopy.CGroup, c)
	copied.CapsAttempted = fieldToCopy.CapsAttempted
	copied.CapsAttemptedHostUserNS = fieldToCopy.CapsAttemptedHostUserNS
	copied.CapsUsed = fieldToCopy.CapsUsed
	copied.CapsUsedHostUserNS = fieldToCopy.CapsUsedHostUserNS
	copied.Comm = fieldToCopy.Comm
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext, c)
	copied.Cookie = fieldToCopy.Cookie
	copied.CreatedAt = fieldToCopy.CreatedAt
	copied.Credentials = deepCopyCredentials(fieldToCopy.Credentials, c)
	copied.Envp = deepCopystringArr(fieldToCopy.Envp, c)
	copied.Envs = deepCopystringArr(fieldToCopy.Envs, c)
	copied.EnvsEntry = deepCopyEnvsEntryPtr(fieldToCopy.EnvsEntry, c)
	copied.EnvsID = fieldToCopy.EnvsID
	copied.EnvsTruncated = fieldToCopy.EnvsTruncated
	copied.ExecTime = fieldToCopy.ExecTime
	copied.ExitTime = fieldToCopy.ExitTime
	copied.FileEvent = deepCopyFileEvent(fieldToCopy.FileEvent, c)
	copied.ForkFlags = fieldToCopy.ForkFlags
	copied.ForkTime = fieldToCopy.ForkTime
	copied.IsExec = fieldToCopy.IsExec
	copied.IsExecExec = fieldToCopy.IsExecExec
	copied.IsParentMissing = fieldToCopy.IsParentMissing
	copied.IsThread = fieldToCopy.IsThread
	copied.IsThroughSymLink = fieldToCopy.IsThroughSymLink
	copied.LinuxBinprm = deepCopyLinuxBinprm(fieldToCopy.LinuxBinprm, c)
	copied.PIDContext = deepCopyPIDContext(fieldToCopy.PIDContext, c)
	copied.Source = fieldToCopy.Source
	copied.StopExecutionTime = fieldToCopy.StopExecutionTime
	copied.SymlinkBasenameStr = fieldToCopy.SymlinkBasenameStr
	copied.SymlinkPathnameStr = fieldToCopy.SymlinkPathnameStr
	copied.TTYName = fieldToCopy.TTYName
	copied.Tracer = deepCopyTracer(fieldToCopy.Tracer, c)
	copied.UserSession = deepCopyUserSessionContext(fieldToCopy.UserSession, c)
	return copied
}
func deepCopyProcessContextPtr(fieldToCopy *ProcessContext, c deepCopyContext) *ProcessContext {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyProcessContextPtrType)
	if copied, ok := c[key]; ok {
		return (*ProcessContext)(copied)
	}
	copied := &ProcessContext{}
	c[key] = unsafe.Pointer(copied)
	c[pointerKey(&fieldToCopy.Process, deepCopyProcessPtrType)] = unsafe.Pointer(&copied.Process)
	copied.Ancestor = deepCopyProcessCacheEntryPtr(fieldToCopy.Ancestor, c)
	copied.Parent = deepCopyProcessPtr(fieldToCopy.Parent, c)
	copied.Process = deepCopyProcess(fieldToCopy.Process, c)
	return copied
}
func deepCopyRuleContext(fieldToCopy RuleContext, c deepCopyContext) RuleContext {
	copied := RuleContext{}
	copied.Expression = fieldToCopy.Expression
	copied.MatchingSubExprs = deepCopyMatchingSubExprArr(fieldToCopy.MatchingSubExprs, c)
	return copied
}
func deepCopyMatchingSubExprArr(fieldToCopy []eval.MatchingSubExpr, c deepCopyContext) []eval.MatchingSubExpr {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyMatchingSubExprArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*eval.MatchingSubExpr)(copied), len(fieldToCopy))
	}
	copied := make([]eval.MatchingSubExpr, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchingSubExpr(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyMatchingValue(fieldToCopy eval.MatchingValue, c deepCopyContext) eval.MatchingValue {
	copied := eval.MatchingValue{}
	copied.Field = fieldToCopy.Field
	copied.Offset = fieldToCopy.Offset
	return copied
}
func deepCopyMatchingSubExpr(fieldToCopy eval.MatchingSubExpr, c deepCopyContext) eval.MatchingSubExpr {
	copied := eval.MatchingSubExpr{}
	copied.Offset = fieldToCopy.Offset
	copied.ValueA = deepCopyMatchingValue(fieldToCopy.ValueA, c)
	copied.ValueB = deepCopyMatchingValue(fieldToCopy.ValueB, c)
	return copied
}
func deepCopyMatchedRulePtrArr(fieldToCopy []*MatchedRule, c deepCopyContext) []*MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyMatchedRulePtrArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((**MatchedRule)(copied), len(fieldToCopy))
	}
	copied := make([]*MatchedRule, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyMatchedRulePtr(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyMatchedRulePtr(fieldToCopy *MatchedRule, c deepCopyContext) *MatchedRule {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyMatchedRulePtrType)
	if copied, ok := c[key]; ok {
		return (*MatchedRule)(copied)
	}
	copied := &MatchedRule{}
	c[key] = unsafe.Pointer(copied)
	copied.PolicyName = fieldToCopy.PolicyName
	copied.PolicyVersion = fieldToCopy.PolicyVersion
	copied.RuleID = fieldToCopy.RuleID
	copied.RuleTags = deepCopystringMap(fieldToCopy.RuleTags, c)
	copied.RuleVersion = fieldToCopy.RuleVersion
	return copied
}
func deepCopySecurityProfileContext(fieldToCopy SecurityProfileContext, c deepCopyContext) SecurityProfileContext {
	copied := SecurityProfileContext{}
	copied.EventTypeState = fieldToCopy.EventTypeState
	copied.EventTypes = deepCopyEventTypeArr(fieldToCopy.EventTypes, c)
	copied.Name = fieldToCopy.Name
	copied.ProfileAlreadySent = fieldToCopy.ProfileAlreadySent
	copied.Tags = deepCopystringArr(fieldToCopy.Tags, c)
	copied.Version = fieldToCopy.Version
	return copied
}
func deepCopyEventTypeArr(fieldToCopy []EventType, c deepCopyContext) []EventType {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyEventTypeArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*EventType)(copied), len(fieldToCopy))
	}
	copied := make([]EventType, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyBindEvent(fieldToCopy BindEvent, c deepCopyContext) BindEvent {
	copied := BindEvent{}
	copied.Addr = deepCopyIPPortContext(fieldToCopy.Addr, c)
	copied.AddrFamily = fieldToCopy.AddrFamily
	copied.Protocol = fieldToCopy.Protocol
	copied.SampleCookie = fieldToCopy.SampleCookie
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyCapabilitiesEvent(fieldToCopy CapabilitiesEvent, c deepCopyContext) CapabilitiesEvent {
	copied := CapabilitiesEvent{}
	copied.Attempted = fieldToCopy.Attempted
	copied.AttemptedHostUserNS = fieldToCopy.AttemptedHostUserNS
	copied.Cookie = fieldToCopy.Cookie
	copied.Used = fieldToCopy.Used
	copied.UsedHostUserNS = fieldToCopy.UsedHostUserNS
	return copied
}
func deepCopyCapsetEvent(fieldToCopy CapsetEvent, c deepCopyContext) CapsetEvent {
	copied := CapsetEvent{}
	copied.CapEffective = fieldToCopy.CapEffective
	copied.CapPermitted = fieldToCopy.CapPermitted
	return copied
}
func deepCopyCgroupTracingEvent(fieldToCopy CgroupTracingEvent, c deepCopyContext) CgroupTracingEvent {
	copied := CgroupTracingEvent{}
	copied.CGroupContext = deepCopyCGroupContext(fieldToCopy.CGroupContext, c)
	copied.Config = deepCopyActivityDumpLoadConfig(fieldToCopy.Config, c)
	copied.ConfigCookie = fieldToCopy.ConfigCookie
	copied.ContainerContext = deepCopyContainerContext(fieldToCopy.ContainerContext, c)
	copied.Pid = fieldToCopy.Pid
	return copied
}
func deepCopyActivityDumpLoadConfig(fieldToCopy ActivityDumpLoadConfig, c deepCopyContext) ActivityDumpLoadConfig {
	copied := ActivityDumpLoadConfig{}
	copied.EndTimestampRaw = fieldToCopy.EndTimestampRaw
	copied.Paused = fieldToCopy.Paused
	copied.Rate = fieldToCopy.Rate
	copied.StartTimestampRaw = fieldToCopy.StartTimestampRaw
	copied.Timeout = fieldToCopy.Timeout
	copied.TracedEventTypes = deepCopyEventTypeArr(fieldToCopy.TracedEventTypes, c)
	copied.WaitListTimestampRaw = fieldToCopy.WaitListTimestampRaw
	return copied
}
func deepCopyCgroupWriteEvent(fieldToCopy CgroupWriteEvent, c deepCopyContext) CgroupWriteEvent {
	copied := CgroupWriteEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Pid = fieldToCopy.Pid
	copied.Retval = fieldToCopy.Retval
	return copied
}
func deepCopyChdirEvent(fieldToCopy ChdirEvent, c deepCopyContext) ChdirEvent {
	copied := ChdirEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopySyscallContext(fieldToCopy SyscallContext, c deepCopyContext) SyscallContext {
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
func deepCopyChmodEvent(fieldToCopy ChmodEvent, c deepCopyContext) ChmodEvent {
	copied := ChmodEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Mode = fieldToCopy.Mode
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallMode = fieldToCopy.SyscallMode
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyChownEvent(fieldToCopy ChownEvent, c deepCopyContext) ChownEvent {
	copied := ChownEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.GID = fieldToCopy.GID
	copied.Group = fieldToCopy.Group
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallGID = fieldToCopy.SyscallGID
	copied.SyscallPath = fieldToCopy.SyscallPath
	copied.SyscallUID = fieldToCopy.SyscallUID
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopyConnectEvent(fieldToCopy ConnectEvent, c deepCopyContext) ConnectEvent {
	copied := ConnectEvent{}
	copied.Addr = deepCopyIPPortContext(fieldToCopy.Addr, c)
	copied.AddrFamily = fieldToCopy.AddrFamily
	copied.Hostnames = deepCopystringArr(fieldToCopy.Hostnames, c)
	copied.Protocol = fieldToCopy.Protocol
	copied.SampleCookie = fieldToCopy.SampleCookie
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyDNSEvent(fieldToCopy DNSEvent, c deepCopyContext) DNSEvent {
	copied := DNSEvent{}
	copied.ID = fieldToCopy.ID
	copied.Question = deepCopyDNSQuestion(fieldToCopy.Question, c)
	copied.Response = deepCopyDNSResponsePtr(fieldToCopy.Response, c)
	return copied
}
func deepCopyDNSQuestion(fieldToCopy DNSQuestion, c deepCopyContext) DNSQuestion {
	copied := DNSQuestion{}
	copied.Class = fieldToCopy.Class
	copied.Count = fieldToCopy.Count
	copied.Name = fieldToCopy.Name
	copied.Size = fieldToCopy.Size
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyDNSResponsePtr(fieldToCopy *DNSResponse, c deepCopyContext) *DNSResponse {
	if fieldToCopy == nil {
		return nil
	}
	key := pointerKey(fieldToCopy, deepCopyDNSResponsePtrType)
	if copied, ok := c[key]; ok {
		return (*DNSResponse)(copied)
	}
	copied := &DNSResponse{}
	c[key] = unsafe.Pointer(copied)
	copied.CNames = deepCopystringArr(fieldToCopy.CNames, c)
	copied.IPs = deepCopyIPNetArr(fieldToCopy.IPs, c)
	copied.ResponseCode = fieldToCopy.ResponseCode
	return copied
}
func deepCopyIPNetArr(fieldToCopy []net.IPNet, c deepCopyContext) []net.IPNet {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyIPNetArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*net.IPNet)(copied), len(fieldToCopy))
	}
	copied := make([]net.IPNet, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyIPNet(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyExecEvent(fieldToCopy ExecEvent, c deepCopyContext) ExecEvent {
	copied := ExecEvent{}
	copied.FileMetadata = deepCopyFileMetadata(fieldToCopy.FileMetadata, c)
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process, c)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyFileMetadata(fieldToCopy FileMetadata, c deepCopyContext) FileMetadata {
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
func deepCopyExitEvent(fieldToCopy ExitEvent, c deepCopyContext) ExitEvent {
	copied := ExitEvent{}
	copied.Cause = fieldToCopy.Cause
	copied.Code = fieldToCopy.Code
	copied.Process = deepCopyProcessPtr(fieldToCopy.Process, c)
	return copied
}
func deepCopyFailedDNSEvent(fieldToCopy FailedDNSEvent, c deepCopyContext) FailedDNSEvent {
	copied := FailedDNSEvent{}
	copied.Payload = deepCopybyteArr(fieldToCopy.Payload, c)
	return copied
}
func deepCopybyteArr(fieldToCopy []byte, c deepCopyContext) []byte {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopybyteArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*byte)(copied), len(fieldToCopy))
	}
	copied := make([]byte, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyGoLabelsContext(fieldToCopy GoLabelsContext, c deepCopyContext) GoLabelsContext {
	copied := GoLabelsContext{}
	copied.ID = fieldToCopy.ID
	copied.Resolved = fieldToCopy.Resolved
	return copied
}
func deepCopyIMDSEvent(fieldToCopy IMDSEvent, c deepCopyContext) IMDSEvent {
	copied := IMDSEvent{}
	copied.AWS = deepCopyAWSIMDSEvent(fieldToCopy.AWS, c)
	copied.CloudProvider = fieldToCopy.CloudProvider
	copied.CredentialSource = fieldToCopy.CredentialSource
	copied.Host = fieldToCopy.Host
	copied.Server = fieldToCopy.Server
	copied.Type = fieldToCopy.Type
	copied.URL = fieldToCopy.URL
	copied.UserAgent = fieldToCopy.UserAgent
	return copied
}
func deepCopyAWSIMDSEvent(fieldToCopy AWSIMDSEvent, c deepCopyContext) AWSIMDSEvent {
	copied := AWSIMDSEvent{}
	copied.IsIMDSv2 = fieldToCopy.IsIMDSv2
	copied.SecurityCredentials = deepCopyAWSSecurityCredentials(fieldToCopy.SecurityCredentials, c)
	return copied
}
func deepCopyInvalidateDentryEvent(fieldToCopy InvalidateDentryEvent, c deepCopyContext) InvalidateDentryEvent {
	copied := InvalidateDentryEvent{}
	copied.Inode = fieldToCopy.Inode
	copied.MountID = fieldToCopy.MountID
	return copied
}
func deepCopyLinkEvent(fieldToCopy LinkEvent, c deepCopyContext) LinkEvent {
	copied := LinkEvent{}
	copied.Source = deepCopyFileEvent(fieldToCopy.Source, c)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallDestinationPath = fieldToCopy.SyscallDestinationPath
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallPath = fieldToCopy.SyscallPath
	copied.Target = deepCopyFileEvent(fieldToCopy.Target, c)
	return copied
}
func deepCopyLoadModuleEvent(fieldToCopy LoadModuleEvent, c deepCopyContext) LoadModuleEvent {
	copied := LoadModuleEvent{}
	copied.Args = fieldToCopy.Args
	copied.ArgsTruncated = fieldToCopy.ArgsTruncated
	copied.Argv = deepCopystringArr(fieldToCopy.Argv, c)
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.LoadedFromMemory = fieldToCopy.LoadedFromMemory
	copied.Name = fieldToCopy.Name
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyLoginUIDWriteEvent(fieldToCopy LoginUIDWriteEvent, c deepCopyContext) LoginUIDWriteEvent {
	copied := LoginUIDWriteEvent{}
	copied.AUID = fieldToCopy.AUID
	return copied
}
func deepCopyMMapEvent(fieldToCopy MMapEvent, c deepCopyContext) MMapEvent {
	copied := MMapEvent{}
	copied.Addr = fieldToCopy.Addr
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Flags = fieldToCopy.Flags
	copied.Len = fieldToCopy.Len
	copied.Offset = fieldToCopy.Offset
	copied.Protection = fieldToCopy.Protection
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyMProtectEvent(fieldToCopy MProtectEvent, c deepCopyContext) MProtectEvent {
	copied := MProtectEvent{}
	copied.ReqProtection = fieldToCopy.ReqProtection
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.VMEnd = fieldToCopy.VMEnd
	copied.VMProtection = fieldToCopy.VMProtection
	copied.VMStart = fieldToCopy.VMStart
	return copied
}
func deepCopyMkdirEvent(fieldToCopy MkdirEvent, c deepCopyContext) MkdirEvent {
	copied := MkdirEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Mode = fieldToCopy.Mode
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallMode = fieldToCopy.SyscallMode
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyMountEvent(fieldToCopy MountEvent, c deepCopyContext) MountEvent {
	copied := MountEvent{}
	copied.Mount = deepCopyMount(fieldToCopy.Mount, c)
	copied.MountPointPath = fieldToCopy.MountPointPath
	copied.MountRootPath = fieldToCopy.MountRootPath
	copied.MountSourcePath = fieldToCopy.MountSourcePath
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallFSType = fieldToCopy.SyscallFSType
	copied.SyscallMountpointPath = fieldToCopy.SyscallMountpointPath
	copied.SyscallSourcePath = fieldToCopy.SyscallSourcePath
	return copied
}
func deepCopyMount(fieldToCopy Mount, c deepCopyContext) Mount {
	copied := Mount{}
	copied.BindSrcMountID = fieldToCopy.BindSrcMountID
	copied.BindSrcMountIDUnique = fieldToCopy.BindSrcMountIDUnique
	copied.Children = deepCopyuint32Arr(fieldToCopy.Children, c)
	copied.Detached = fieldToCopy.Detached
	copied.Device = fieldToCopy.Device
	copied.FSType = fieldToCopy.FSType
	copied.MountFlags = fieldToCopy.MountFlags
	copied.MountID = fieldToCopy.MountID
	copied.MountIDUnique = fieldToCopy.MountIDUnique
	copied.MountPointStr = fieldToCopy.MountPointStr
	copied.NamespaceInode = fieldToCopy.NamespaceInode
	copied.Origin = fieldToCopy.Origin
	copied.ParentMountIDUnique = fieldToCopy.ParentMountIDUnique
	copied.ParentPathKey = deepCopyPathKey(fieldToCopy.ParentPathKey, c)
	copied.Path = fieldToCopy.Path
	copied.RootPathKey = deepCopyPathKey(fieldToCopy.RootPathKey, c)
	copied.RootStr = fieldToCopy.RootStr
	copied.Visible = fieldToCopy.Visible
	return copied
}
func deepCopyMountReleasedEvent(fieldToCopy MountReleasedEvent, c deepCopyContext) MountReleasedEvent {
	copied := MountReleasedEvent{}
	copied.MountID = fieldToCopy.MountID
	copied.MountIDUnique = fieldToCopy.MountIDUnique
	return copied
}
func deepCopyNetDeviceEvent(fieldToCopy NetDeviceEvent, c deepCopyContext) NetDeviceEvent {
	copied := NetDeviceEvent{}
	copied.Device = deepCopyNetDevice(fieldToCopy.Device, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyNetDevice(fieldToCopy NetDevice, c deepCopyContext) NetDevice {
	copied := NetDevice{}
	copied.IfIndex = fieldToCopy.IfIndex
	copied.Name = fieldToCopy.Name
	copied.NetNS = fieldToCopy.NetNS
	copied.PeerIfIndex = fieldToCopy.PeerIfIndex
	copied.PeerNetNS = fieldToCopy.PeerNetNS
	return copied
}
func deepCopyNetworkContext(fieldToCopy NetworkContext, c deepCopyContext) NetworkContext {
	copied := NetworkContext{}
	copied.Destination = deepCopyIPPortContext(fieldToCopy.Destination, c)
	copied.Device = deepCopyNetworkDeviceContext(fieldToCopy.Device, c)
	copied.L3Protocol = fieldToCopy.L3Protocol
	copied.L4Protocol = fieldToCopy.L4Protocol
	copied.NetworkDirection = fieldToCopy.NetworkDirection
	copied.Size = fieldToCopy.Size
	copied.Source = deepCopyIPPortContext(fieldToCopy.Source, c)
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopyNetworkDeviceContext(fieldToCopy NetworkDeviceContext, c deepCopyContext) NetworkDeviceContext {
	copied := NetworkDeviceContext{}
	copied.IfIndex = fieldToCopy.IfIndex
	copied.IfName = fieldToCopy.IfName
	copied.NetNS = fieldToCopy.NetNS
	return copied
}
func deepCopyNetworkFlowMonitorEvent(fieldToCopy NetworkFlowMonitorEvent, c deepCopyContext) NetworkFlowMonitorEvent {
	copied := NetworkFlowMonitorEvent{}
	copied.Device = deepCopyNetworkDeviceContext(fieldToCopy.Device, c)
	copied.Flows = deepCopyFlowArr(fieldToCopy.Flows, c)
	copied.FlowsCount = fieldToCopy.FlowsCount
	return copied
}
func deepCopyFlowArr(fieldToCopy []Flow, c deepCopyContext) []Flow {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyFlowArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*Flow)(copied), len(fieldToCopy))
	}
	copied := make([]Flow, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = deepCopyFlow(fieldToCopy[i], c)
	}
	return copied
}
func deepCopyNetworkStats(fieldToCopy NetworkStats, c deepCopyContext) NetworkStats {
	copied := NetworkStats{}
	copied.DataSize = fieldToCopy.DataSize
	copied.PacketCount = fieldToCopy.PacketCount
	return copied
}
func deepCopyFlow(fieldToCopy Flow, c deepCopyContext) Flow {
	copied := Flow{}
	copied.Destination = deepCopyIPPortContext(fieldToCopy.Destination, c)
	copied.Egress = deepCopyNetworkStats(fieldToCopy.Egress, c)
	copied.Ingress = deepCopyNetworkStats(fieldToCopy.Ingress, c)
	copied.L3Protocol = fieldToCopy.L3Protocol
	copied.L4Protocol = fieldToCopy.L4Protocol
	copied.Source = deepCopyIPPortContext(fieldToCopy.Source, c)
	return copied
}
func deepCopyOnDemandEvent(fieldToCopy OnDemandEvent, c deepCopyContext) OnDemandEvent {
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
func deepCopyOpenEvent(fieldToCopy OpenEvent, c deepCopyContext) OpenEvent {
	copied := OpenEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Flags = fieldToCopy.Flags
	copied.Mode = fieldToCopy.Mode
	copied.SampleCookie = fieldToCopy.SampleCookie
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallFlags = fieldToCopy.SyscallFlags
	copied.SyscallMode = fieldToCopy.SyscallMode
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyPTraceEvent(fieldToCopy PTraceEvent, c deepCopyContext) PTraceEvent {
	copied := PTraceEvent{}
	copied.Address = fieldToCopy.Address
	copied.NSPID = fieldToCopy.NSPID
	copied.PID = fieldToCopy.PID
	copied.Request = fieldToCopy.Request
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.Tracee = deepCopyProcessContextPtr(fieldToCopy.Tracee, c)
	return copied
}
func deepCopyPrCtlEvent(fieldToCopy PrCtlEvent, c deepCopyContext) PrCtlEvent {
	copied := PrCtlEvent{}
	copied.IsNameTruncated = fieldToCopy.IsNameTruncated
	copied.NewName = fieldToCopy.NewName
	copied.Option = fieldToCopy.Option
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyRawPacketEvent(fieldToCopy RawPacketEvent, c deepCopyContext) RawPacketEvent {
	copied := RawPacketEvent{}
	copied.CaptureInfo = deepCopyCaptureInfo(fieldToCopy.CaptureInfo, c)
	copied.Data = deepCopybyteArr(fieldToCopy.Data, c)
	copied.Filter = fieldToCopy.Filter
	copied.NetworkContext = deepCopyNetworkContext(fieldToCopy.NetworkContext, c)
	copied.TLSContext = deepCopyTLSContext(fieldToCopy.TLSContext, c)
	return copied
}
func deepCopyCaptureInfo(fieldToCopy gopacket.CaptureInfo, c deepCopyContext) gopacket.CaptureInfo {
	return fieldToCopy
}
func deepCopyTLSContext(fieldToCopy TLSContext, c deepCopyContext) TLSContext {
	copied := TLSContext{}
	copied.Version = fieldToCopy.Version
	return copied
}
func deepCopySetXAttrEvent(fieldToCopy SetXAttrEvent, c deepCopyContext) SetXAttrEvent {
	copied := SetXAttrEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Name = fieldToCopy.Name
	copied.NameRaw = fieldToCopy.NameRaw
	copied.Namespace = fieldToCopy.Namespace
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyRenameEvent(fieldToCopy RenameEvent, c deepCopyContext) RenameEvent {
	copied := RenameEvent{}
	copied.New = deepCopyFileEvent(fieldToCopy.New, c)
	copied.Old = deepCopyFileEvent(fieldToCopy.Old, c)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallDestinationPath = fieldToCopy.SyscallDestinationPath
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyRmdirEvent(fieldToCopy RmdirEvent, c deepCopyContext) RmdirEvent {
	copied := RmdirEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopySELinuxEvent(fieldToCopy SELinuxEvent, c deepCopyContext) SELinuxEvent {
	copied := SELinuxEvent{}
	copied.BoolChangeValue = fieldToCopy.BoolChangeValue
	copied.BoolCommitValue = fieldToCopy.BoolCommitValue
	copied.BoolName = fieldToCopy.BoolName
	copied.EnforceStatus = fieldToCopy.EnforceStatus
	copied.EventKind = fieldToCopy.EventKind
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	return copied
}
func deepCopySetgidEvent(fieldToCopy SetgidEvent, c deepCopyContext) SetgidEvent {
	copied := SetgidEvent{}
	copied.EGID = fieldToCopy.EGID
	copied.EGroup = fieldToCopy.EGroup
	copied.FSGID = fieldToCopy.FSGID
	copied.FSGroup = fieldToCopy.FSGroup
	copied.GID = fieldToCopy.GID
	copied.Group = fieldToCopy.Group
	return copied
}
func deepCopySetSockOptEvent(fieldToCopy SetSockOptEvent, c deepCopyContext) SetSockOptEvent {
	copied := SetSockOptEvent{}
	copied.FilterHash = fieldToCopy.FilterHash
	copied.FilterInstructions = fieldToCopy.FilterInstructions
	copied.FilterLen = fieldToCopy.FilterLen
	copied.IsFilterTruncated = fieldToCopy.IsFilterTruncated
	copied.Level = fieldToCopy.Level
	copied.OptName = fieldToCopy.OptName
	copied.RawFilter = deepCopybyteArr(fieldToCopy.RawFilter, c)
	copied.SizeToRead = fieldToCopy.SizeToRead
	copied.SocketFamily = fieldToCopy.SocketFamily
	copied.SocketProtocol = fieldToCopy.SocketProtocol
	copied.SocketType = fieldToCopy.SocketType
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.UsedImmediates = deepCopyintArr(fieldToCopy.UsedImmediates, c)
	return copied
}
func deepCopyintArr(fieldToCopy []int, c deepCopyContext) []int {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopyintArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*int)(copied), len(fieldToCopy))
	}
	copied := make([]int, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopySetuidEvent(fieldToCopy SetuidEvent, c deepCopyContext) SetuidEvent {
	copied := SetuidEvent{}
	copied.EUID = fieldToCopy.EUID
	copied.EUser = fieldToCopy.EUser
	copied.FSUID = fieldToCopy.FSUID
	copied.FSUser = fieldToCopy.FSUser
	copied.UID = fieldToCopy.UID
	copied.User = fieldToCopy.User
	return copied
}
func deepCopySetrlimitEvent(fieldToCopy SetrlimitEvent, c deepCopyContext) SetrlimitEvent {
	copied := SetrlimitEvent{}
	copied.Resource = fieldToCopy.Resource
	copied.RlimCur = fieldToCopy.RlimCur
	copied.RlimMax = fieldToCopy.RlimMax
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.Target = deepCopyProcessContextPtr(fieldToCopy.Target, c)
	copied.TargetPid = fieldToCopy.TargetPid
	return copied
}
func deepCopySignalEvent(fieldToCopy SignalEvent, c deepCopyContext) SignalEvent {
	copied := SignalEvent{}
	copied.PID = fieldToCopy.PID
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.Target = deepCopyProcessContextPtr(fieldToCopy.Target, c)
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopySocketEvent(fieldToCopy SocketEvent, c deepCopyContext) SocketEvent {
	copied := SocketEvent{}
	copied.Domain = fieldToCopy.Domain
	copied.Protocol = fieldToCopy.Protocol
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.Type = fieldToCopy.Type
	return copied
}
func deepCopySpliceEvent(fieldToCopy SpliceEvent, c deepCopyContext) SpliceEvent {
	copied := SpliceEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.PipeEntryFlag = fieldToCopy.PipeEntryFlag
	copied.PipeExitFlag = fieldToCopy.PipeExitFlag
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopySysCtlEvent(fieldToCopy SysCtlEvent, c deepCopyContext) SysCtlEvent {
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
func deepCopySyscallsEvent(fieldToCopy SyscallsEvent, c deepCopyContext) SyscallsEvent {
	copied := SyscallsEvent{}
	copied.EventReason = fieldToCopy.EventReason
	copied.SampleCookie = fieldToCopy.SampleCookie
	copied.SyscallID = fieldToCopy.SyscallID
	copied.Syscalls = deepCopySyscallArr(fieldToCopy.Syscalls, c)
	return copied
}
func deepCopySyscallArr(fieldToCopy []Syscall, c deepCopyContext) []Syscall {
	if fieldToCopy == nil {
		return nil
	}
	key := sliceKey(fieldToCopy, deepCopySyscallArrType)
	if copied, ok := c[key]; ok {
		return unsafe.Slice((*Syscall)(copied), len(fieldToCopy))
	}
	copied := make([]Syscall, len(fieldToCopy))
	c[key] = unsafe.Pointer(unsafe.SliceData(copied))
	for i := range fieldToCopy {
		copied[i] = fieldToCopy[i]
	}
	return copied
}
func deepCopyTracerMemfdSealEvent(fieldToCopy TracerMemfdSealEvent, c deepCopyContext) TracerMemfdSealEvent {
	copied := TracerMemfdSealEvent{}
	copied.Fd = fieldToCopy.Fd
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyUmountEvent(fieldToCopy UmountEvent, c deepCopyContext) UmountEvent {
	copied := UmountEvent{}
	copied.MountID = fieldToCopy.MountID
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyUnlinkEvent(fieldToCopy UnlinkEvent, c deepCopyContext) UnlinkEvent {
	copied := UnlinkEvent{}
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Flags = fieldToCopy.Flags
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallDirFd = fieldToCopy.SyscallDirFd
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallFlags = fieldToCopy.SyscallFlags
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyUnloadModuleEvent(fieldToCopy UnloadModuleEvent, c deepCopyContext) UnloadModuleEvent {
	copied := UnloadModuleEvent{}
	copied.Name = fieldToCopy.Name
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyUnshareEvent(fieldToCopy UnshareEvent, c deepCopyContext) UnshareEvent {
	copied := UnshareEvent{}
	copied.Flags = fieldToCopy.Flags
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}
func deepCopyUnshareMountNSEvent(fieldToCopy UnshareMountNSEvent, c deepCopyContext) UnshareMountNSEvent {
	copied := UnshareMountNSEvent{}
	copied.Mount = deepCopyMount(fieldToCopy.Mount, c)
	return copied
}
func deepCopyUtimesEvent(fieldToCopy UtimesEvent, c deepCopyContext) UtimesEvent {
	copied := UtimesEvent{}
	copied.Atime = fieldToCopy.Atime
	copied.File = deepCopyFileEvent(fieldToCopy.File, c)
	copied.Mtime = fieldToCopy.Mtime
	copied.SyscallContext = deepCopySyscallContext(fieldToCopy.SyscallContext, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	copied.SyscallPath = fieldToCopy.SyscallPath
	return copied
}
func deepCopyVethPairEvent(fieldToCopy VethPairEvent, c deepCopyContext) VethPairEvent {
	copied := VethPairEvent{}
	copied.HostDevice = deepCopyNetDevice(fieldToCopy.HostDevice, c)
	copied.PeerDevice = deepCopyNetDevice(fieldToCopy.PeerDevice, c)
	copied.SyscallEvent = deepCopySyscallEvent(fieldToCopy.SyscallEvent, c)
	return copied
}

const (
	_ deepCopyType = iota
	deepCopyAWSSecurityCredentialsArrType
	deepCopyArgsEntryPtrType
	deepCopyDNSResponsePtrType
	deepCopyEnvsEntryPtrType
	deepCopyEventTypeArrType
	deepCopyFlowArrType
	deepCopyIPNetArrType
	deepCopyMatchedRulePtrType
	deepCopyMatchedRulePtrArrType
	deepCopyMatchingSubExprArrType
	deepCopyProcessCacheEntryPtrType
	deepCopyProcessContextPtrType
	deepCopyProcessPtrType
	deepCopyReleasablePtrType
	deepCopySyscallArrType
	deepCopybyteArrType
	deepCopyintArrType
	deepCopystringArrType
	deepCopystringArrMapType
	deepCopystringMapType
	deepCopyuint32ArrType
	deepCopyIPBytesType
)
