// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package model

import "github.com/DataDog/datadog-agent/pkg/security/secl/eval"

// TranslateLegacyField transforms a legacy attribute into its updated version. Should be idempotent for non legacy
// fields.
func (m *Model) TranslateLegacyField(field eval.Field) eval.Field {
	switch field {
	// chmod
	case "chmod.filename":
		return "chmod.file.path"
	case "chmod.container_path":
		return "chmod.file.container_path"
	case "chmod.overlay_numlower":
		return "chmod.file.overlay_numlower"
	case "chmod.basename":
		return "chmod.file.name"
	case "chmod.mode":
		return "chmod.file.destination.mode"

	// chown
	case "chown.filename":
		return "chown.file.path"
	case "chown.container_path":
		return "chown.file.container_path"
	case "chown.overlay_numlower":
		return "chown.file.overlay_numlower"
	case "chown.basename":
		return "chown.file.name"
	case "chmod.uid":
		return "chown.file.destination.uid"
	case "chmod.user":
		return "chown.file.destination.user"
	case "chmod.gid":
		return "chown.file.destination.gid"
	case "chmod.group":
		return "chown.file.destination.group"

	//	open
	case "open.filename":
		return "open.file.path"
	case "open.container_path":
		return "open.file.container_path"
	case "open.overlay_numlower":
		return "open.file.overlay_numlower"
	case "open.basename":
		return "open.file.name"
	case "open.mode":
		return "open.file.destination.mode"

	// mkdir
	case "mkdir.filename":
		return "mkdir.file.path"
	case "mkdir.container_path":
		return "mkdir.file.container_path"
	case "mkdir.overlay_numlower":
		return "mkdir.file.overlay_numlower"
	case "mkdir.basename":
		return "mkdir.file.name"
	case "mkdir.mode":
		return "mkdir.file.destination.mode"

	// rmdir
	case "rmdir.filename":
		return "rmdir.file.path"
	case "rmdir.container_path":
		return "rmdir.file.container_path"
	case "rmdir.overlay_numlower":
		return "rmdir.file.overlay_numlower"
	case "rmdir.basename":
		return "rmdir.file.name"

	// rename
	case "rename.old.filename":
		return "rename.file.path"
	case "rename.old.container_path":
		return "rename.file.container_path"
	case "rename.old.overlay_numlower":
		return "rename.file.overlay_numlower"
	case "rename.old.basename":
		return "rename.file.name"
	case "rename.new.filename":
		return "rename.file.destination.path"
	case "rename.new.container_path":
		return "rename.file.destination.container_path"
	case "rename.new.overlay_numlower":
		return "rename.file.destination.overlay_numlower"
	case "rename.new.basename":
		return "rename.file.destination.name"

	// unlink
	case "unlink.filename":
		return "unlink.file.path"
	case "unlink.container_path":
		return "unlink.file.container_path"
	case "unlink.overlay_numlower":
		return "unlink.file.overlay_numlower"
	case "unlink.basename":
		return "unlink.file.name"

	// utimes
	case "utimes.filename":
		return "utimes.file.path"
	case "utimes.container_path":
		return "utimes.file.container_path"
	case "utimes.overlay_numlower":
		return "utimes.file.overlay_numlower"
	case "utimes.basename":
		return "utimes.file.name"

	// link
	case "link.source.filename":
		return "link.file.path"
	case "link.source.container_path":
		return "link.file.container_path"
	case "link.source.overlay_numlower":
		return "link.file.overlay_numlower"
	case "link.source.basename":
		return "link.file.name"
	case "link.target.filename":
		return "link.file.destination.path"
	case "link.target.container_path":
		return "link.file.destination.container_path"
	case "link.target.overlay_numlower":
		return "link.file.destination.overlay_numlower"
	case "link.target.basename":
		return "link.file.destination.name"

	// setxattr
	case "setxattr.filename":
		return "setxattr.file.path"
	case "setxattr.container_path":
		return "setxattr.file.container_path"
	case "setxattr.overlay_numlower":
		return "setxattr.file.overlay_numlower"
	case "setxattr.basename":
		return "setxattr.file.name"
	case "setxattr.namespace":
		return "setxattr.file.destination.namespace"
	case "setxattr.name":
		return "setxattr.file.destination.name"

	// removexattr
	case "removexattr.filename":
		return "removexattr.file.path"
	case "removexattr.container_path":
		return "removexattr.file.container_path"
	case "removexattr.overlay_numlower":
		return "removexattr.file.overlay_numlower"
	case "removexattr.basename":
		return "removexattr.file.name"
	case "removexattr.namespace":
		return "removexattr.file.destination.namespace"
	case "removexattr.name":
		return "removexattr.file.destination.name"

	// exec
	case "exec.filename":
		return "exec.file.path"
	case "exec.container_path":
		return "exec.file.container_path"
	case "exec.overlay_numlower":
		return "exec.file.overlay_numlower"
	case "exec.basename":
		return "exec.file.name"

	// process
	case "process.filename":
		return "process.file.path"
	case "process.container_path":
		return "process.file.container_path"
	case "process.overlay_numlower":
		return "process.file.overlay_numlower"
	case "process.basename":
		return "process.file.name"
	default:
		return field
	}
}
