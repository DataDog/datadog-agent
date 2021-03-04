// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// SECLLegacyAttributes contains the list of the legacy attributes we need to support
var SECLLegacyAttributes = map[eval.Field]eval.Field{
	// chmod
	"chmod.filename":         "chmod.file.path",
	"chmod.container_path":   "chmod.file.container_path",
	"chmod.overlay_numlower": "chmod.file.overlay_numlower",
	"chmod.basename":         "chmod.file.name",
	"chmod.mode":             "chmod.file.destination.mode",

	// chown
	"chown.filename":         "chown.file.path",
	"chown.container_path":   "chown.file.container_path",
	"chown.overlay_numlower": "chown.file.overlay_numlower",
	"chown.basename":         "chown.file.name",
	"chown.uid":              "chown.file.destination.uid",
	"chown.user":             "chown.file.destination.user",
	"chown.gid":              "chown.file.destination.gid",
	"chown.group":            "chown.file.destination.group",

	// open
	"open.filename":         "open.file.path",
	"open.container_path":   "open.file.container_path",
	"open.overlay_numlower": "open.file.overlay_numlower",
	"open.basename":         "open.file.name",
	"open.mode":             "open.file.destination.mode",

	// mkdir
	"mkdir.filename":         "mkdir.file.path",
	"mkdir.container_path":   "mkdir.file.container_path",
	"mkdir.overlay_numlower": "mkdir.file.overlay_numlower",
	"mkdir.basename":         "mkdir.file.name",
	"mkdir.mode":             "mkdir.file.destination.mode",

	// rmdir
	"rmdir.filename":         "rmdir.file.path",
	"rmdir.container_path":   "rmdir.file.container_path",
	"rmdir.overlay_numlower": "rmdir.file.overlay_numlower",
	"rmdir.basename":         "rmdir.file.name",

	// rename
	"rename.old.filename":         "rename.file.path",
	"rename.old.container_path":   "rename.file.container_path",
	"rename.old.overlay_numlower": "rename.file.overlay_numlower",
	"rename.old.basename":         "rename.file.name",
	"rename.new.filename":         "rename.file.destination.path",
	"rename.new.container_path":   "rename.file.destination.container_path",
	"rename.new.overlay_numlower": "rename.file.destination.overlay_numlower",
	"rename.new.basename":         "rename.file.destination.name",

	// unlink
	"unlink.filename":         "unlink.file.path",
	"unlink.container_path":   "unlink.file.container_path",
	"unlink.overlay_numlower": "unlink.file.overlay_numlower",
	"unlink.basename":         "unlink.file.name",

	// utimes
	"utimes.filename":         "utimes.file.path",
	"utimes.container_path":   "utimes.file.container_path",
	"utimes.overlay_numlower": "utimes.file.overlay_numlower",
	"utimes.basename":         "utimes.file.name",

	// link
	"link.source.filename":         "link.file.path",
	"link.source.container_path":   "link.file.container_path",
	"link.source.overlay_numlower": "link.file.overlay_numlower",
	"link.source.basename":         "link.file.name",
	"link.target.filename":         "link.file.destination.path",
	"link.target.container_path":   "link.file.destination.container_path",
	"link.target.overlay_numlower": "link.file.destination.overlay_numlower",
	"link.target.basename":         "link.file.destination.name",

	// setxattr
	"setxattr.filename":         "setxattr.file.path",
	"setxattr.container_path":   "setxattr.file.container_path",
	"setxattr.overlay_numlower": "setxattr.file.overlay_numlower",
	"setxattr.basename":         "setxattr.file.name",
	"setxattr.namespace":        "setxattr.file.destination.namespace",
	"setxattr.name":             "setxattr.file.destination.name",

	// removexattr
	"removexattr.filename":         "removexattr.file.path",
	"removexattr.container_path":   "removexattr.file.container_path",
	"removexattr.overlay_numlower": "removexattr.file.overlay_numlower",
	"removexattr.basename":         "removexattr.file.name",
	"removexattr.namespace":        "removexattr.file.destination.namespace",
	"removexattr.name":             "removexattr.file.destination.name",

	// exec
	"exec.filename":         "exec.file.path",
	"exec.container_path":   "exec.file.container_path",
	"exec.overlay_numlower": "exec.file.overlay_numlower",
	"exec.basename":         "exec.file.name",

	// process
	"process.filename":         "process.file.path",
	"process.container_path":   "process.file.container_path",
	"process.overlay_numlower": "process.file.overlay_numlower",
	"process.basename":         "process.file.name",
}
