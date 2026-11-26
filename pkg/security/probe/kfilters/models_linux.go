// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kfilters

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// fileApprovers approves the paths and the names of the files of an event type
// on their basename, and the paths on the basename of their parent directory
// with parent.
func fileApprovers(event model.EventType, parent bool, files ...string) rules.ApproverModel {
	var models []rules.ApproverModel
	for _, file := range files {
		prefix := event.String() + "." + file
		models = append(models,
			rules.On(Basename(parent), prefix+".path"),
			rules.On(Basename(false), prefix+".name"))
	}
	return rules.Any(models...)
}

func init() {
	// the kernel approves on the audit uid the events of these event types only
	auid := rules.On(AUID, "process.auid")
	for _, event := range []model.EventType{model.FileChmodEventType, model.FileChownEventType, model.FileMkdirEventType,
		model.FileRmdirEventType, model.FileUnlinkEventType, model.FileUtimesEventType, model.FileChdirEventType} {
		models[event.String()] = rules.Any(fileApprovers(event, true, "file"), auid)
	}
	models["rename"] = rules.Any(fileApprovers(model.FileRenameEventType, true, "file", "file.destination"), auid)
	// the kernel approves the links on the basenames of their files only
	models["link"] = fileApprovers(model.FileLinkEventType, false, "file", "file.destination")

	models["open"] = rules.Any(
		fileApprovers(model.FileOpenEventType, true, "file"),
		auid,
		rules.On(rules.Any(Bits("open_flags_approvers", 100, false), RdOnly), "open.flags"),
		rules.On(UpperLayer, "open.file.in_upper_layer"),
	)
	models["mmap"] = rules.Any(
		fileApprovers(model.MMapEventType, true, "file"),
		rules.On(Bits("mmap_flags_approvers", 0, false), "mmap.flags"),
		rules.On(Bits("mmap_protection_approvers", 0, true), "mmap.protection"),
	)
	models["splice"] = rules.Any(
		fileApprovers(model.SpliceEventType, true, "file"),
		rules.On(Bits("splice_entry_flags_approvers", 0, false), "splice.pipe_entry_flag"),
		rules.On(Bits("splice_exit_flags_approvers", 0, false), "splice.pipe_exit_flag"),
	)
	models["mprotect"] = rules.Any(
		rules.On(Bits("mprotect_vm_protection_approvers", 0, false), "mprotect.vm_protection"),
		rules.On(Bits("mprotect_req_protection_approvers", 0, false), "mprotect.req_protection"),
	)
	models["sysctl"] = rules.On(Bits("sysctl_action_approvers", 0, false), "sysctl.action")
	models["unshare"] = rules.On(Bits("unshare_flags_approvers", 0, true), "unshare.flags")

	models["bpf"] = rules.On(Enum("bpf_cmd_approvers", 0, 0), "bpf.cmd")
	models["prctl"] = rules.On(Enum("prctl_option_approvers", 0, 0), "prctl.option")
	models["setsockopt"] = rules.Any(
		rules.On(Enum("setsockopt_level_or_optname_approvers", 0, 1), "setsockopt.level"),
		rules.On(Enum("setsockopt_level_or_optname_approvers", 1, 2), "setsockopt.optname"),
	)
	models["socket"] = rules.Any(
		rules.On(Enum("socket_field_approvers", 0, 50), "socket.domain"),
		rules.On(Enum("socket_field_approvers", 1, 60), "socket.type"),
		rules.On(Enum("socket_field_approvers", 2, 70), "socket.protocol"),
	)
	models["connect"] = rules.Any(
		rules.On(Enum("connect_addr_family_approvers", 0, 50), "connect.addr.family"),
		rules.On(InetFamily("connect_addr_family_approvers"),
			"connect.addr.port", "connect.addr.ip", "connect.addr.is_public", "connect.addr.hostname"),
	)
	models["bind"] = rules.Any(
		rules.On(Enum("bind_addr_family_approvers", 0, 50), "bind.addr.family"),
		rules.On(InetFamily("bind_addr_family_approvers"), "bind.addr.port", "bind.addr.ip", "bind.addr.is_public"),
	)
}
