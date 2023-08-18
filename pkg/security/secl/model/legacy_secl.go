// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"

// SECLLegacyFields contains the list of the legacy attributes we need to support
var SECLLegacyFields = map[eval.Field]eval.Field{
	// event
	"async": "event.async",

	// chmod
	"chmod.filename": "chmod.file.path",
	"chmod.basename": "chmod.file.name",
	"chmod.mode":     "chmod.file.destination.mode",

	// chown
	"chown.filename": "chown.file.path",
	"chown.basename": "chown.file.name",
	"chown.uid":      "chown.file.destination.uid",
	"chown.user":     "chown.file.destination.user",
	"chown.gid":      "chown.file.destination.gid",
	"chown.group":    "chown.file.destination.group",

	// open
	"open.filename": "open.file.path",
	"open.basename": "open.file.name",
	"open.mode":     "open.file.destination.mode",

	// mkdir
	"mkdir.filename": "mkdir.file.path",
	"mkdir.basename": "mkdir.file.name",
	"mkdir.mode":     "mkdir.file.destination.mode",

	// rmdir
	"rmdir.filename": "rmdir.file.path",
	"rmdir.basename": "rmdir.file.name",

	// rename
	"rename.old.filename": "rename.file.path",
	"rename.old.basename": "rename.file.name",
	"rename.new.filename": "rename.file.destination.path",
	"rename.new.basename": "rename.file.destination.name",

	// unlink
	"unlink.filename": "unlink.file.path",
	"unlink.basename": "unlink.file.name",

	// utimes
	"utimes.filename": "utimes.file.path",
	"utimes.basename": "utimes.file.name",

	// link
	"link.source.filename": "link.file.path",
	"link.source.basename": "link.file.name",
	"link.target.filename": "link.file.destination.path",
	"link.target.basename": "link.file.destination.name",

	// setxattr
	"setxattr.filename":  "setxattr.file.path",
	"setxattr.basename":  "setxattr.file.name",
	"setxattr.namespace": "setxattr.file.destination.namespace",
	"setxattr.name":      "setxattr.file.destination.name",

	// removexattr
	"removexattr.filename":  "removexattr.file.path",
	"removexattr.basename":  "removexattr.file.name",
	"removexattr.namespace": "removexattr.file.destination.namespace",
	"removexattr.name":      "removexattr.file.destination.name",

	// exec
	"exec.filename":         "exec.file.path",
	"exec.overlay_numlower": "exec.file.overlay_numlower",
	"exec.basename":         "exec.file.name",
	"exec.name":             "exec.comm",

	// process
	"process.filename":           "process.file.path",
	"process.basename":           "process.file.name",
	"process.name":               "process.comm",
	"process.ancestors.filename": "process.ancestors.file.path",
	"process.ancestors.basename": "process.ancestors.file.name",
	"process.ancestors.name":     "process.ancestors.comm",
}
