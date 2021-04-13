// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probes

const (
	// SecurityAgentUID is the UID used for all the runtime security module probes
	SecurityAgentUID = "security"
)

const (
	// DentryResolverERPCKey is the key to the eRPC dentry resolver tail call program
	DentryResolverERPCKey uint32 = iota
	// DentryResolverKernKprobeKey is the key to the kernel dentry resolver tail call program
	DentryResolverKernKprobeKey
)

const (
	// DentryResolverKernTracepointKey is the key to the kernel dentry resolver tail call program
	DentryResolverKernTracepointKey uint32 = iota
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
