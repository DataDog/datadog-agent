// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

package seclcel

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
)

// modelShapes holds every CEL object type the SECL field namespace is made of,
// keyed by type name and then by member name.
//
// The types come from a trie over the SECL field *names* rather than from the Go
// structs behind them: `connect.addr` draws its members from a nested
// IPPortContext and from flat Go fields tagged `addr.hostname` and `addr.family`,
// so no Go type describes it. One type is shared by every path exposing the same
// members, which is why a member carries no field name: the runtime composes the
// flat SECL name by joining the path it was reached through.
var modelShapes = map[string]map[string]*types.Type{
	// secl.Accept is the type of accept.
	"secl.Accept": {
		"addr":   types.NewObjectType("secl.Addr"),
		"retval": types.IntType,
	},
	// secl.Addr is the type of accept.addr, connect.addr.
	"secl.Addr": {
		"family":    types.IntType,
		"hostname":  types.NewListType(types.StringType),
		"ip":        ext.CIDRType,
		"is_public": types.BoolType,
		"port":      types.IntType,
	},
	// secl.Ancestors is the type of process.ancestors, process.parent, ptrace.tracee.ancestors, ptrace.tracee.parent and 4 more.
	"secl.Ancestors": {
		"args":           types.StringType,
		"args_flags":     types.NewListType(types.StringType),
		"args_options":   types.NewListType(types.StringType),
		"args_truncated": types.BoolType,
		"argv":           types.NewListType(types.StringType),
		"argv0":          types.StringType,
		"auid":           types.IntType,
		"cap_effective":  types.IntType,
		"cap_permitted":  types.IntType,
		"caps_attempted": types.IntType,
		"caps_used":      types.IntType,
		"cgroup":         types.NewObjectType("secl.Cgroup"),
		"comm":           types.StringType,
		"container":      types.NewObjectType("secl.Container"),
		"created_at":     types.IntType,
		"egid":           types.IntType,
		"egroup":         types.StringType,
		"envp":           types.NewListType(types.StringType),
		"envs":           types.NewListType(types.StringType),
		"envs_truncated": types.BoolType,
		"euid":           types.IntType,
		"euser":          types.StringType,
		"file":           types.NewObjectType("secl.File"),
		"fsgid":          types.IntType,
		"fsgroup":        types.StringType,
		"fsuid":          types.IntType,
		"fsuser":         types.StringType,
		"gid":            types.IntType,
		"group":          types.StringType,
		"interpreter":    types.NewObjectType("secl.Interpreter"),
		"is_exec":        types.BoolType,
		"is_kworker":     types.BoolType,
		"is_thread":      types.BoolType,
		"mntns":          types.IntType,
		"netns":          types.IntType,
		"pid":            types.IntType,
		"ppid":           types.IntType,
		"sid":            types.IntType,
		"tid":            types.IntType,
		"tty_name":       types.StringType,
		"uid":            types.IntType,
		"user":           types.StringType,
		"user_session":   types.NewObjectType("secl.UserSession"),
	},
	// secl.Arg1 is the type of ondemand.arg1, ondemand.arg2, ondemand.arg3, ondemand.arg4 and 2 more.
	"secl.Arg1": {
		"str":  types.StringType,
		"uint": types.IntType,
	},
	// secl.Aws is the type of imds.aws.
	"secl.Aws": {
		"is_imds_v2":           types.BoolType,
		"security_credentials": types.NewObjectType("secl.SecurityCredentials"),
	},
	// secl.Bind is the type of bind.
	"secl.Bind": {
		"addr":     types.NewObjectType("secl.BindAddr"),
		"protocol": types.IntType,
		"retval":   types.IntType,
	},
	// secl.BindAddr is the type of bind.addr.
	"secl.BindAddr": {
		"family":    types.IntType,
		"ip":        ext.CIDRType,
		"is_public": types.BoolType,
		"port":      types.IntType,
	},
	// secl.Bool is the type of selinux.bool.
	"secl.Bool": {
		"name":  types.StringType,
		"state": types.StringType,
	},
	// secl.BoolCommit is the type of selinux.bool_commit.
	"secl.BoolCommit": {
		"state": types.BoolType,
	},
	// secl.Bpf is the type of bpf.
	"secl.Bpf": {
		"cmd":    types.IntType,
		"map":    types.NewObjectType("secl.Map"),
		"prog":   types.NewObjectType("secl.Prog"),
		"retval": types.IntType,
	},
	// secl.Capabilities is the type of capabilities.
	"secl.Capabilities": {
		"attempted": types.IntType,
		"used":      types.IntType,
	},
	// secl.Capset is the type of capset.
	"secl.Capset": {
		"cap_effective": types.IntType,
		"cap_permitted": types.IntType,
	},
	// secl.Cgroup is the type of exec.cgroup, exit.cgroup, process.ancestors.cgroup, process.cgroup and 10 more.
	"secl.Cgroup": {
		"created_at": types.IntType,
		"file":       types.NewObjectType("secl.CgroupFile"),
		"id":         types.StringType,
		"version":    types.IntType,
	},
	// secl.CgroupFile is the type of exec.cgroup.file, exit.cgroup.file, process.ancestors.cgroup.file, process.cgroup.file and 10 more.
	"secl.CgroupFile": {
		"inode":    types.IntType,
		"mount_id": types.IntType,
	},
	// secl.CgroupWrite is the type of cgroup_write.
	"secl.CgroupWrite": {
		"file": types.NewObjectType("secl.File"),
		"pid":  types.IntType,
	},
	// secl.Chdir is the type of chdir, rmdir, utimes.
	"secl.Chdir": {
		"file":    types.NewObjectType("secl.File"),
		"retval":  types.IntType,
		"syscall": types.NewObjectType("secl.Syscall"),
	},
	// secl.Chmod is the type of chmod, mkdir.
	"secl.Chmod": {
		"file":    types.NewObjectType("secl.ChmodFile"),
		"retval":  types.IntType,
		"syscall": types.NewObjectType("secl.ChmodSyscall"),
	},
	// secl.ChmodFile is the type of chmod.file, mkdir.file.
	"secl.ChmodFile": {
		"change_time":       types.IntType,
		"destination":       types.NewObjectType("secl.FileDestination"),
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.ChmodSyscall is the type of chmod.syscall, mkdir.syscall.
	"secl.ChmodSyscall": {
		"mode": types.IntType,
		"path": types.StringType,
	},
	// secl.Chown is the type of chown.
	"secl.Chown": {
		"file":    types.NewObjectType("secl.ChownFile"),
		"retval":  types.IntType,
		"syscall": types.NewObjectType("secl.ChownSyscall"),
	},
	// secl.ChownFile is the type of chown.file.
	"secl.ChownFile": {
		"change_time":       types.IntType,
		"destination":       types.NewObjectType("secl.ChownFileDestination"),
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.ChownFileDestination is the type of chown.file.destination.
	"secl.ChownFileDestination": {
		"gid":   types.IntType,
		"group": types.StringType,
		"uid":   types.IntType,
		"user":  types.StringType,
	},
	// secl.ChownSyscall is the type of chown.syscall.
	"secl.ChownSyscall": {
		"gid":  types.IntType,
		"path": types.StringType,
		"uid":  types.IntType,
	},
	// secl.Connect is the type of connect.
	"secl.Connect": {
		"addr":     types.NewObjectType("secl.Addr"),
		"protocol": types.IntType,
		"retval":   types.IntType,
	},
	// secl.Container is the type of exec.container, exit.container, process.ancestors.container, process.container and 10 more.
	"secl.Container": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.Destination is the type of network.destination, network.source, network_flow_monitor.flows.destination, network_flow_monitor.flows.source and 2 more.
	"secl.Destination": {
		"ip":        ext.CIDRType,
		"is_public": types.BoolType,
		"port":      types.IntType,
	},
	// secl.Device is the type of network.device, network_flow_monitor.device, packet.device.
	"secl.Device": {
		"ifname": types.StringType,
		"netns":  types.IntType,
	},
	// secl.Dns is the type of dns.
	"secl.Dns": {
		"id":       types.IntType,
		"question": types.NewObjectType("secl.Question"),
		"response": types.NewObjectType("secl.Response"),
	},
	// secl.Egress is the type of network_flow_monitor.flows.egress, network_flow_monitor.flows.ingress.
	"secl.Egress": {
		"data_size":    types.IntType,
		"packet_count": types.IntType,
	},
	// secl.Enforce is the type of selinux.enforce.
	"secl.Enforce": {
		"status": types.StringType,
	},
	// secl.Event is the type of event.
	"secl.Event": {
		"async":     types.BoolType,
		"hostname":  types.StringType,
		"origin":    types.StringType,
		"os":        types.StringType,
		"rule":      types.NewObjectType("secl.Rule"),
		"service":   types.StringType,
		"signature": types.StringType,
		"source":    types.StringType,
		"timestamp": types.IntType,
	},
	// secl.Exec is the type of exec.
	"secl.Exec": {
		"args":           types.StringType,
		"args_flags":     types.NewListType(types.StringType),
		"args_options":   types.NewListType(types.StringType),
		"args_truncated": types.BoolType,
		"argv":           types.NewListType(types.StringType),
		"argv0":          types.StringType,
		"auid":           types.IntType,
		"cap_effective":  types.IntType,
		"cap_permitted":  types.IntType,
		"caps_attempted": types.IntType,
		"caps_used":      types.IntType,
		"cgroup":         types.NewObjectType("secl.Cgroup"),
		"comm":           types.StringType,
		"container":      types.NewObjectType("secl.Container"),
		"created_at":     types.IntType,
		"egid":           types.IntType,
		"egroup":         types.StringType,
		"envp":           types.NewListType(types.StringType),
		"envs":           types.NewListType(types.StringType),
		"envs_truncated": types.BoolType,
		"euid":           types.IntType,
		"euser":          types.StringType,
		"file":           types.NewObjectType("secl.ExecFile"),
		"fsgid":          types.IntType,
		"fsgroup":        types.StringType,
		"fsuid":          types.IntType,
		"fsuser":         types.StringType,
		"gid":            types.IntType,
		"group":          types.StringType,
		"interpreter":    types.NewObjectType("secl.Interpreter"),
		"is_exec":        types.BoolType,
		"is_kworker":     types.BoolType,
		"is_thread":      types.BoolType,
		"mntns":          types.IntType,
		"netns":          types.IntType,
		"pid":            types.IntType,
		"ppid":           types.IntType,
		"sid":            types.IntType,
		"syscall":        types.NewObjectType("secl.Syscall"),
		"tid":            types.IntType,
		"tty_name":       types.StringType,
		"uid":            types.IntType,
		"user":           types.StringType,
		"user_session":   types.NewObjectType("secl.UserSession"),
	},
	// secl.ExecFile is the type of exec.file.
	"secl.ExecFile": {
		"change_time":       types.IntType,
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"metadata":          types.NewObjectType("secl.Metadata"),
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.Exit is the type of exit.
	"secl.Exit": {
		"args":           types.StringType,
		"args_flags":     types.NewListType(types.StringType),
		"args_options":   types.NewListType(types.StringType),
		"args_truncated": types.BoolType,
		"argv":           types.NewListType(types.StringType),
		"argv0":          types.StringType,
		"auid":           types.IntType,
		"cap_effective":  types.IntType,
		"cap_permitted":  types.IntType,
		"caps_attempted": types.IntType,
		"caps_used":      types.IntType,
		"cause":          types.IntType,
		"cgroup":         types.NewObjectType("secl.Cgroup"),
		"code":           types.IntType,
		"comm":           types.StringType,
		"container":      types.NewObjectType("secl.Container"),
		"created_at":     types.IntType,
		"egid":           types.IntType,
		"egroup":         types.StringType,
		"envp":           types.NewListType(types.StringType),
		"envs":           types.NewListType(types.StringType),
		"envs_truncated": types.BoolType,
		"euid":           types.IntType,
		"euser":          types.StringType,
		"file":           types.NewObjectType("secl.File"),
		"fsgid":          types.IntType,
		"fsgroup":        types.StringType,
		"fsuid":          types.IntType,
		"fsuser":         types.StringType,
		"gid":            types.IntType,
		"group":          types.StringType,
		"interpreter":    types.NewObjectType("secl.Interpreter"),
		"is_exec":        types.BoolType,
		"is_kworker":     types.BoolType,
		"is_thread":      types.BoolType,
		"mntns":          types.IntType,
		"netns":          types.IntType,
		"pid":            types.IntType,
		"ppid":           types.IntType,
		"sid":            types.IntType,
		"tid":            types.IntType,
		"tty_name":       types.StringType,
		"uid":            types.IntType,
		"user":           types.StringType,
		"user_session":   types.NewObjectType("secl.UserSession"),
	},
	// secl.File is the type of cgroup_write.file, chdir.file, exec.interpreter.file, exit.file and 33 more.
	"secl.File": {
		"change_time":       types.IntType,
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.FileDestination is the type of chmod.file.destination, mkdir.file.destination.
	"secl.FileDestination": {
		"mode":   types.IntType,
		"rights": types.IntType,
	},
	// secl.Flows is the type of network_flow_monitor.flows.
	"secl.Flows": {
		"destination": types.NewObjectType("secl.Destination"),
		"egress":      types.NewObjectType("secl.Egress"),
		"ingress":     types.NewObjectType("secl.Egress"),
		"l3_protocol": types.IntType,
		"l4_protocol": types.IntType,
		"source":      types.NewObjectType("secl.Destination"),
	},
	// secl.Imds is the type of imds.
	"secl.Imds": {
		"aws":            types.NewObjectType("secl.Aws"),
		"cloud_provider": types.StringType,
		"host":           types.StringType,
		"server":         types.StringType,
		"type":           types.StringType,
		"url":            types.StringType,
		"user_agent":     types.StringType,
	},
	// secl.Interpreter is the type of exec.interpreter, exit.interpreter, process.ancestors.interpreter, process.interpreter and 10 more.
	"secl.Interpreter": {
		"file": types.NewObjectType("secl.File"),
	},
	// secl.Link is the type of link, rename.
	"secl.Link": {
		"file":    types.NewObjectType("secl.LinkFile"),
		"retval":  types.IntType,
		"syscall": types.NewObjectType("secl.LinkSyscall"),
	},
	// secl.LinkFile is the type of link.file, rename.file.
	"secl.LinkFile": {
		"change_time":       types.IntType,
		"destination":       types.NewObjectType("secl.File"),
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.LinkSyscall is the type of link.syscall, rename.syscall.
	"secl.LinkSyscall": {
		"destination": types.NewObjectType("secl.Syscall"),
		"path":        types.StringType,
	},
	// secl.LoadModule is the type of load_module.
	"secl.LoadModule": {
		"args":               types.StringType,
		"args_truncated":     types.BoolType,
		"argv":               types.NewListType(types.StringType),
		"file":               types.NewObjectType("secl.File"),
		"loaded_from_memory": types.BoolType,
		"name":               types.StringType,
		"retval":             types.IntType,
	},
	// secl.Map is the type of bpf.map.
	"secl.Map": {
		"name": types.StringType,
		"type": types.IntType,
	},
	// secl.Metadata is the type of exec.file.metadata.
	"secl.Metadata": {
		"abi":                  types.IntType,
		"architecture":         types.IntType,
		"compression":          types.IntType,
		"is_executable":        types.BoolType,
		"is_garble_obfuscated": types.BoolType,
		"is_upx_packed":        types.BoolType,
		"size":                 types.IntType,
		"type":                 types.IntType,
	},
	// secl.Mmap is the type of mmap.
	"secl.Mmap": {
		"file":       types.NewObjectType("secl.File"),
		"flags":      types.IntType,
		"protection": types.IntType,
		"retval":     types.IntType,
	},
	// secl.Mount is the type of mount.
	"secl.Mount": {
		"detached":   types.BoolType,
		"fs_type":    types.StringType,
		"mountpoint": types.NewObjectType("secl.Syscall"),
		"retval":     types.IntType,
		"root":       types.NewObjectType("secl.Syscall"),
		"source":     types.NewObjectType("secl.Syscall"),
		"syscall":    types.NewObjectType("secl.MountSyscall"),
		"visible":    types.BoolType,
	},
	// secl.MountSyscall is the type of mount.syscall.
	"secl.MountSyscall": {
		"fs_type":    types.StringType,
		"mountpoint": types.NewObjectType("secl.Syscall"),
		"source":     types.NewObjectType("secl.Syscall"),
	},
	// secl.Mprotect is the type of mprotect.
	"secl.Mprotect": {
		"req_protection": types.IntType,
		"retval":         types.IntType,
		"vm_protection":  types.IntType,
	},
	// secl.Network is the type of network.
	"secl.Network": {
		"destination":       types.NewObjectType("secl.Destination"),
		"device":            types.NewObjectType("secl.Device"),
		"l3_protocol":       types.IntType,
		"l4_protocol":       types.IntType,
		"network_direction": types.IntType,
		"size":              types.IntType,
		"source":            types.NewObjectType("secl.Destination"),
		"type":              types.IntType,
	},
	// secl.NetworkFlowMonitor is the type of network_flow_monitor.
	"secl.NetworkFlowMonitor": {
		"device": types.NewObjectType("secl.Device"),
		"flows":  types.NewListType(types.NewObjectType("secl.Flows")),
	},
	// secl.Ondemand is the type of ondemand.
	"secl.Ondemand": {
		"arg1": types.NewObjectType("secl.Arg1"),
		"arg2": types.NewObjectType("secl.Arg1"),
		"arg3": types.NewObjectType("secl.Arg1"),
		"arg4": types.NewObjectType("secl.Arg1"),
		"arg5": types.NewObjectType("secl.Arg1"),
		"arg6": types.NewObjectType("secl.Arg1"),
		"name": types.StringType,
	},
	// secl.Open is the type of open.
	"secl.Open": {
		"file":    types.NewObjectType("secl.OpenFile"),
		"flags":   types.IntType,
		"retval":  types.IntType,
		"syscall": types.NewObjectType("secl.OpenSyscall"),
	},
	// secl.OpenFile is the type of open.file.
	"secl.OpenFile": {
		"change_time":       types.IntType,
		"destination":       types.NewObjectType("secl.OpenFileDestination"),
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.OpenFileDestination is the type of open.file.destination.
	"secl.OpenFileDestination": {
		"mode": types.IntType,
	},
	// secl.OpenSyscall is the type of open.syscall.
	"secl.OpenSyscall": {
		"flags": types.IntType,
		"mode":  types.IntType,
		"path":  types.StringType,
	},
	// secl.Package is the type of cgroup_write.file.package, chdir.file.package, chmod.file.package, chown.file.package and 42 more.
	"secl.Package": {
		"epoch":          types.IntType,
		"name":           types.StringType,
		"release":        types.StringType,
		"source_epoch":   types.IntType,
		"source_release": types.StringType,
		"source_version": types.StringType,
		"version":        types.StringType,
	},
	// secl.Packet is the type of packet.
	"secl.Packet": {
		"destination":       types.NewObjectType("secl.Destination"),
		"device":            types.NewObjectType("secl.Device"),
		"filter":            types.StringType,
		"l3_protocol":       types.IntType,
		"l4_protocol":       types.IntType,
		"network_direction": types.IntType,
		"size":              types.IntType,
		"source":            types.NewObjectType("secl.Destination"),
		"tls":               types.NewObjectType("secl.Tls"),
		"type":              types.IntType,
	},
	// secl.Prctl is the type of prctl.
	"secl.Prctl": {
		"is_name_truncated": types.BoolType,
		"new_name":          types.StringType,
		"option":            types.IntType,
		"retval":            types.IntType,
	},
	// secl.Process is the type of process, ptrace.tracee, setrlimit.target, signal.target.
	"secl.Process": {
		"ancestors":      types.NewListType(types.NewObjectType("secl.Ancestors")),
		"args":           types.StringType,
		"args_flags":     types.NewListType(types.StringType),
		"args_options":   types.NewListType(types.StringType),
		"args_truncated": types.BoolType,
		"argv":           types.NewListType(types.StringType),
		"argv0":          types.StringType,
		"auid":           types.IntType,
		"cap_effective":  types.IntType,
		"cap_permitted":  types.IntType,
		"caps_attempted": types.IntType,
		"caps_used":      types.IntType,
		"cgroup":         types.NewObjectType("secl.Cgroup"),
		"comm":           types.StringType,
		"container":      types.NewObjectType("secl.Container"),
		"created_at":     types.IntType,
		"egid":           types.IntType,
		"egroup":         types.StringType,
		"envp":           types.NewListType(types.StringType),
		"envs":           types.NewListType(types.StringType),
		"envs_truncated": types.BoolType,
		"euid":           types.IntType,
		"euser":          types.StringType,
		"file":           types.NewObjectType("secl.File"),
		"fsgid":          types.IntType,
		"fsgroup":        types.StringType,
		"fsuid":          types.IntType,
		"fsuser":         types.StringType,
		"gid":            types.IntType,
		"group":          types.StringType,
		"interpreter":    types.NewObjectType("secl.Interpreter"),
		"is_exec":        types.BoolType,
		"is_kworker":     types.BoolType,
		"is_thread":      types.BoolType,
		"mntns":          types.IntType,
		"netns":          types.IntType,
		"parent":         types.NewObjectType("secl.Ancestors"),
		"pid":            types.IntType,
		"ppid":           types.IntType,
		"sid":            types.IntType,
		"tid":            types.IntType,
		"tty_name":       types.StringType,
		"uid":            types.IntType,
		"user":           types.StringType,
		"user_session":   types.NewObjectType("secl.UserSession"),
	},
	// secl.Prog is the type of bpf.prog.
	"secl.Prog": {
		"attach_type": types.IntType,
		"helpers":     types.NewListType(types.IntType),
		"name":        types.StringType,
		"tag":         types.StringType,
		"type":        types.IntType,
	},
	// secl.Ptrace is the type of ptrace.
	"secl.Ptrace": {
		"request": types.IntType,
		"retval":  types.IntType,
		"tracee":  types.NewObjectType("secl.Process"),
	},
	// secl.Question is the type of dns.question.
	"secl.Question": {
		"class":  types.IntType,
		"count":  types.IntType,
		"length": types.IntType,
		"name":   types.StringType,
		"type":   types.IntType,
	},
	// secl.Removexattr is the type of removexattr, setxattr.
	"secl.Removexattr": {
		"file":   types.NewObjectType("secl.RemovexattrFile"),
		"retval": types.IntType,
	},
	// secl.RemovexattrFile is the type of removexattr.file, setxattr.file.
	"secl.RemovexattrFile": {
		"change_time":       types.IntType,
		"destination":       types.NewObjectType("secl.RemovexattrFileDestination"),
		"extension":         types.StringType,
		"filesystem":        types.StringType,
		"gid":               types.IntType,
		"group":             types.StringType,
		"hashes":            types.NewListType(types.StringType),
		"in_upper_layer":    types.BoolType,
		"inode":             types.IntType,
		"mode":              types.IntType,
		"modification_time": types.IntType,
		"mount_detached":    types.BoolType,
		"mount_id":          types.IntType,
		"mount_visible":     types.BoolType,
		"name":              types.StringType,
		"package":           types.NewObjectType("secl.Package"),
		"path":              types.StringType,
		"rights":            types.IntType,
		"uid":               types.IntType,
		"user":              types.StringType,
	},
	// secl.RemovexattrFileDestination is the type of removexattr.file.destination, setxattr.file.destination.
	"secl.RemovexattrFileDestination": {
		"name":      types.StringType,
		"namespace": types.StringType,
	},
	// secl.Response is the type of dns.response.
	"secl.Response": {
		"cnames": types.NewListType(types.StringType),
		"code":   types.IntType,
		"ips":    types.NewListType(ext.CIDRType),
	},
	// secl.Rule is the type of event.rule.
	"secl.Rule": {
		"tags": types.NewListType(types.StringType),
	},
	// secl.SecurityCredentials is the type of imds.aws.security_credentials.
	"secl.SecurityCredentials": {
		"type": types.StringType,
	},
	// secl.Selinux is the type of selinux.
	"secl.Selinux": {
		"bool":        types.NewObjectType("secl.Bool"),
		"bool_commit": types.NewObjectType("secl.BoolCommit"),
		"enforce":     types.NewObjectType("secl.Enforce"),
	},
	// secl.Setgid is the type of setgid.
	"secl.Setgid": {
		"egid":    types.IntType,
		"egroup":  types.StringType,
		"fsgid":   types.IntType,
		"fsgroup": types.StringType,
		"gid":     types.IntType,
		"group":   types.StringType,
	},
	// secl.Setrlimit is the type of setrlimit.
	"secl.Setrlimit": {
		"resource": types.IntType,
		"retval":   types.IntType,
		"rlim_cur": types.IntType,
		"rlim_max": types.IntType,
		"target":   types.NewObjectType("secl.Process"),
	},
	// secl.Setsockopt is the type of setsockopt.
	"secl.Setsockopt": {
		"filter_hash":         types.StringType,
		"filter_instructions": types.StringType,
		"filter_len":          types.IntType,
		"is_filter_truncated": types.BoolType,
		"level":               types.IntType,
		"optname":             types.IntType,
		"retval":              types.IntType,
		"socket_family":       types.IntType,
		"socket_protocol":     types.IntType,
		"socket_type":         types.IntType,
		"used_immediates":     types.NewListType(types.IntType),
	},
	// secl.Setuid is the type of setuid.
	"secl.Setuid": {
		"euid":   types.IntType,
		"euser":  types.StringType,
		"fsuid":  types.IntType,
		"fsuser": types.StringType,
		"uid":    types.IntType,
		"user":   types.StringType,
	},
	// secl.Signal is the type of signal.
	"secl.Signal": {
		"pid":    types.IntType,
		"retval": types.IntType,
		"target": types.NewObjectType("secl.Process"),
		"type":   types.IntType,
	},
	// secl.Socket is the type of socket.
	"secl.Socket": {
		"domain":   types.IntType,
		"protocol": types.IntType,
		"retval":   types.IntType,
		"type":     types.IntType,
	},
	// secl.Splice is the type of splice.
	"secl.Splice": {
		"file":            types.NewObjectType("secl.File"),
		"pipe_entry_flag": types.IntType,
		"pipe_exit_flag":  types.IntType,
		"retval":          types.IntType,
	},
	// secl.Syscall is the type of chdir.syscall, exec.syscall, link.syscall.destination, mount.mountpoint and 7 more.
	"secl.Syscall": {
		"path": types.StringType,
	},
	// secl.Sysctl is the type of sysctl.
	"secl.Sysctl": {
		"action":              types.IntType,
		"file_position":       types.IntType,
		"name":                types.StringType,
		"name_truncated":      types.BoolType,
		"old_value":           types.StringType,
		"old_value_truncated": types.BoolType,
		"value":               types.StringType,
		"value_truncated":     types.BoolType,
	},
	// secl.Tls is the type of packet.tls.
	"secl.Tls": {
		"version": types.IntType,
	},
	// secl.Unlink is the type of unlink.
	"secl.Unlink": {
		"file":    types.NewObjectType("secl.File"),
		"flags":   types.IntType,
		"retval":  types.IntType,
		"syscall": types.NewObjectType("secl.UnlinkSyscall"),
	},
	// secl.UnlinkSyscall is the type of unlink.syscall.
	"secl.UnlinkSyscall": {
		"dirfd": types.IntType,
		"flags": types.IntType,
		"path":  types.StringType,
	},
	// secl.UnloadModule is the type of unload_module.
	"secl.UnloadModule": {
		"name":   types.StringType,
		"retval": types.IntType,
	},
	// secl.UserSession is the type of exec.user_session, exit.user_session, process.ancestors.user_session, process.parent.user_session and 10 more.
	"secl.UserSession": {
		"id":              types.StringType,
		"identity":        types.StringType,
		"k8s_groups":      types.NewListType(types.StringType),
		"k8s_session_id":  types.IntType,
		"k8s_uid":         types.StringType,
		"k8s_username":    types.StringType,
		"session_type":    types.IntType,
		"ssh_auth_method": types.IntType,
		"ssh_client_ip":   ext.CIDRType,
		"ssh_client_port": types.IntType,
		"ssh_public_key":  types.StringType,
		"ssh_session_id":  types.IntType,
	},
}

// modelRoots holds the top level segments of the SECL field namespace, which are
// the names a CEL environment declares as variables.
var modelRoots = map[string]*types.Type{
	"accept":               types.NewObjectType("secl.Accept"),
	"bind":                 types.NewObjectType("secl.Bind"),
	"bpf":                  types.NewObjectType("secl.Bpf"),
	"capabilities":         types.NewObjectType("secl.Capabilities"),
	"capset":               types.NewObjectType("secl.Capset"),
	"cgroup_write":         types.NewObjectType("secl.CgroupWrite"),
	"chdir":                types.NewObjectType("secl.Chdir"),
	"chmod":                types.NewObjectType("secl.Chmod"),
	"chown":                types.NewObjectType("secl.Chown"),
	"connect":              types.NewObjectType("secl.Connect"),
	"dns":                  types.NewObjectType("secl.Dns"),
	"event":                types.NewObjectType("secl.Event"),
	"exec":                 types.NewObjectType("secl.Exec"),
	"exit":                 types.NewObjectType("secl.Exit"),
	"imds":                 types.NewObjectType("secl.Imds"),
	"link":                 types.NewObjectType("secl.Link"),
	"load_module":          types.NewObjectType("secl.LoadModule"),
	"mkdir":                types.NewObjectType("secl.Chmod"),
	"mmap":                 types.NewObjectType("secl.Mmap"),
	"mount":                types.NewObjectType("secl.Mount"),
	"mprotect":             types.NewObjectType("secl.Mprotect"),
	"network":              types.NewObjectType("secl.Network"),
	"network_flow_monitor": types.NewObjectType("secl.NetworkFlowMonitor"),
	"ondemand":             types.NewObjectType("secl.Ondemand"),
	"open":                 types.NewObjectType("secl.Open"),
	"packet":               types.NewObjectType("secl.Packet"),
	"prctl":                types.NewObjectType("secl.Prctl"),
	"process":              types.NewObjectType("secl.Process"),
	"ptrace":               types.NewObjectType("secl.Ptrace"),
	"removexattr":          types.NewObjectType("secl.Removexattr"),
	"rename":               types.NewObjectType("secl.Link"),
	"rmdir":                types.NewObjectType("secl.Chdir"),
	"selinux":              types.NewObjectType("secl.Selinux"),
	"setgid":               types.NewObjectType("secl.Setgid"),
	"setrlimit":            types.NewObjectType("secl.Setrlimit"),
	"setsockopt":           types.NewObjectType("secl.Setsockopt"),
	"setuid":               types.NewObjectType("secl.Setuid"),
	"setxattr":             types.NewObjectType("secl.Removexattr"),
	"signal":               types.NewObjectType("secl.Signal"),
	"socket":               types.NewObjectType("secl.Socket"),
	"splice":               types.NewObjectType("secl.Splice"),
	"sysctl":               types.NewObjectType("secl.Sysctl"),
	"unlink":               types.NewObjectType("secl.Unlink"),
	"unload_module":        types.NewObjectType("secl.UnloadModule"),
	"utimes":               types.NewObjectType("secl.Chdir"),
}
