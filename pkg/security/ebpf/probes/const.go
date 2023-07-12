// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probes

const (
	// SecurityAgentUID is the UID used for all the runtime security module probes
	SecurityAgentUID = "security"
)

const (
	// DentryResolverERPCKey is the key to the eRPC dentry resolver tail call program
	DentryResolverERPCKey uint32 = iota
	// DentryResolverParentERPCKey is the key to the eRPC dentry parent resolver tail call program
	DentryResolverParentERPCKey
	// DentryResolverSegmentERPCKey is the key to the eRPC dentry segment resolver tail call program
	DentryResolverSegmentERPCKey
	// DentryResolverKernKprobeKey is the key to the kernel dentry resolver tail call program
	DentryResolverKernKprobeKey
	// ActivityDumpFilterKprobeKey is the key to the kernel activity dump filter tail call program
	ActivityDumpFilterKprobeKey
)

const (
	// DentryResolverKernTracepointKey is the key to the kernel dentry resolver tail call program
	DentryResolverKernTracepointKey uint32 = iota
	// ActivityDumpFilterTracepointKey is the key to the kernel activity dump filter tail call program
	ActivityDumpFilterTracepointKey
)

const (
	// DentryResolverOpenCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an open event
	DentryResolverOpenCallbackKprobeKey uint32 = iota + 1
	// DentryResolverSetAttrCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an setattr event
	DentryResolverSetAttrCallbackKprobeKey
	// DentryResolverMkdirCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an mkdir event
	DentryResolverMkdirCallbackKprobeKey
	// DentryResolverMountCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an mount event
	DentryResolverMountCallbackKprobeKey
	// DentryResolverSecurityInodeRmdirCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an rmdir or unlink event
	DentryResolverSecurityInodeRmdirCallbackKprobeKey
	// DentryResolverSetXAttrCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an setxattr event
	DentryResolverSetXAttrCallbackKprobeKey
	// DentryResolverUnlinkCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of an unlink event
	DentryResolverUnlinkCallbackKprobeKey
	// DentryResolverLinkSrcCallbackKprobeKey is the key to the callback program to execute after resolving the source dentry of a link event
	DentryResolverLinkSrcCallbackKprobeKey
	// DentryResolverLinkDstCallbackKprobeKey is the key to the callback program to execute after resolving the destination dentry of a link event
	DentryResolverLinkDstCallbackKprobeKey
	// DentryResolverRenameCallbackKprobeKey is the key to the callback program to execute after resolving the destination dentry of a rename event
	DentryResolverRenameCallbackKprobeKey
	// DentryResolverSELinuxCallbackKprobeKey is the key to the callback program to execute after resolving the destination dentry of a selinux event
	DentryResolverSELinuxCallbackKprobeKey
	// DentryResolverUnshareMntNSStageOneCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of a cloned mount when a new mount namespace is created using unshare
	DentryResolverUnshareMntNSStageOneCallbackKprobeKey
	// DentryResolverUnshareMntNSStageTwoCallbackKprobeKey is the key to the callback program to execute after resolving the dentry of a cloned mount mountpoint when a new mount namespace is created using unshare
	DentryResolverUnshareMntNSStageTwoCallbackKprobeKey
)

const (
	// DentryResolverOpenCallbackTracepointKey is the key to the callback program to execute after resolving the dentry of an open event
	DentryResolverOpenCallbackTracepointKey uint32 = iota + 1
	// DentryResolverMkdirCallbackTracepointKey is the key to the callback program to execute after resolving the dentry of an mkdir event
	DentryResolverMkdirCallbackTracepointKey
	// DentryResolverMountCallbackTracepointKey is the key to the callback program to execute after resolving the dentry of an mount event
	DentryResolverMountCallbackTracepointKey
	// DentryResolverLinkDstCallbackTracepointKey is the key to the callback program to execute after resolving the destination dentry of a link event
	DentryResolverLinkDstCallbackTracepointKey
	// DentryResolverRenameCallbackTracepointKey is the key to the callback program to execute after resolving the destination dentry of a rename event
	DentryResolverRenameCallbackTracepointKey
)

const (
	// TCDNSRequestKey is the key to DNS request program
	TCDNSRequestKey uint32 = iota + 1
	// TCDNSRequestParserKey is the key to DNS request parser program
	TCDNSRequestParserKey
)

const (
	// ExecGetEnvsOffsetKey is the key to the program that computes the environment variables offset
	ExecGetEnvsOffsetKey uint32 = iota
	// ExecParseArgsEnvsSplitKey is the key to the program that splits the parsing of arguments and environment variables between tailcalls
	ExecParseArgsEnvsSplitKey
	// ExecParseArgsEnvsKey is the key to the program that parses arguments and then environment variables
	ExecParseArgsEnvsKey
)
