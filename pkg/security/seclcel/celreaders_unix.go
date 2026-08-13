// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build unix

package seclcel

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"net"
)

// The readers call whichever of the conversions the fields of this platform's
// model need. Naming them all keeps the set that exists from depending on which
// fields happen to be declared, so adding the first field of a shape does not
// also have to add the conversion for it.
var _ = []any{net.IPNet{}, stringsToVal, intsToVal, boolsToVal, cidrsToVal, cidrToVal}

// celReaders reads one SECL field, indexed by its place in the layout.
//
// A rule reads a field by index: the optimization pass resolves the name once,
// when the rule is planned, and the expression carries the index. So this is a
// slice rather than a map, and celReaderIndex — the name side — is needed only at
// planning time.
//
// Each reader is the body of the evaluator the accessors return for the same
// field, so the two inherit one contract rather than mirroring each other: the
// same `check:` guards, the same handlers, the same defaults. What differs is
// what a reader is given and what it returns. It is handed the element it reads
// from rather than an index into a register, so a field of an iterated element
// is a direct struct read and two fields of one element read from one pointer;
// and it returns a CEL value rather than a Go one, so no type adapter runs
// between the model and the interpreter.
//
// The readers cover exactly the leaves of the CEL type tree: the `length` and
// `root_domain` pseudo fields are absent from both, being translated to size()
// and a helper call instead.
var celReaders = []celReader{
	// 0: accept.addr.family
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Accept.AddrFamily))
	},
	// 1: accept.addr.hostname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.hostname")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveAcceptHostnames(ev, &ev.Accept))
	},
	// 2: accept.addr.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Accept.Addr.IPNet)
	},
	// 3: accept.addr.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.Accept.Addr))
	},
	// 4: accept.addr.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Accept.Addr.Port))
	},
	// 5: accept.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Accept.SyscallEvent.Retval))
	},
	// 6: bind.addr.family
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.AddrFamily))
	},
	// 7: bind.addr.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Bind.Addr.IPNet)
	},
	// 8: bind.addr.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.Bind.Addr))
	},
	// 9: bind.addr.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.Addr.Port))
	},
	// 10: bind.protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.Protocol))
	},
	// 11: bind.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.SyscallEvent.Retval))
	},
	// 12: bpf.cmd
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.cmd")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Cmd))
	},
	// 13: bpf.map.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.map.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BPF.Map.Name)
	},
	// 14: bpf.map.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.map.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Map.Type))
	},
	// 15: bpf.prog.attach_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.attach_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Program.AttachType))
	},
	// 16: bpf.prog.helpers
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.helpers")
		ev := ctx.Event.(*model.Event)
		result := make([]int, len(ev.BPF.Program.Helpers))
		for i, v := range ev.BPF.Program.Helpers {
			result[i] = int(v)
		}
		return intsToVal(result)
	},
	// 17: bpf.prog.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BPF.Program.Name)
	},
	// 18: bpf.prog.tag
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.tag")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BPF.Program.Tag)
	},
	// 19: bpf.prog.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Program.Type))
	},
	// 20: bpf.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.SyscallEvent.Retval))
	},
	// 21: capabilities.attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capabilities.attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCapabilitiesAttempted(ev, &ev.CapabilitiesUsage)))
	},
	// 22: capabilities.used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capabilities.used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCapabilitiesUsed(ev, &ev.CapabilitiesUsage)))
	},
	// 23: capset.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capset.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Capset.CapEffective))
	},
	// 24: capset.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capset.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Capset.CapPermitted))
	},
	// 25: cgroup_write.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.CTime))
	},
	// 26: cgroup_write.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.CgroupWrite.File))
	},
	// 27: cgroup_write.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.CgroupWrite.File))
	},
	// 28: cgroup_write.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.GID))
	},
	// 29: cgroup_write.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.CgroupWrite.File.FileFields))
	},
	// 30: cgroup_write.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.CgroupWrite.File))
	},
	// 31: cgroup_write.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.CgroupWrite.File.FileFields))
	},
	// 32: cgroup_write.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.PathKey.Inode))
	},
	// 33: cgroup_write.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.Mode))
	},
	// 34: cgroup_write.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.MTime))
	},
	// 35: cgroup_write.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.CgroupWrite.File.MountDetached)
	},
	// 36: cgroup_write.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.PathKey.MountID))
	},
	// 37: cgroup_write.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.CgroupWrite.File.MountVisible)
	},
	// 38: cgroup_write.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.CgroupWrite.File))
	},
	// 39: cgroup_write.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.CgroupWrite.File))
	},
	// 40: cgroup_write.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.CgroupWrite.File))
	},
	// 41: cgroup_write.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.CgroupWrite.File))
	},
	// 42: cgroup_write.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.CgroupWrite.File))
	},
	// 43: cgroup_write.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.CgroupWrite.File))
	},
	// 44: cgroup_write.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.CgroupWrite.File))
	},
	// 45: cgroup_write.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.CgroupWrite.File))
	},
	// 46: cgroup_write.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.CgroupWrite.File))
	},
	// 47: cgroup_write.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.CgroupWrite.File.FileFields)))
	},
	// 48: cgroup_write.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.UID))
	},
	// 49: cgroup_write.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.CgroupWrite.File.FileFields))
	},
	// 50: cgroup_write.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.Pid))
	},
	// 51: chdir.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.CTime))
	},
	// 52: chdir.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Chdir.File))
	},
	// 53: chdir.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chdir.File))
	},
	// 54: chdir.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.GID))
	},
	// 55: chdir.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chdir.File.FileFields))
	},
	// 56: chdir.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chdir.File))
	},
	// 57: chdir.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chdir.File.FileFields))
	},
	// 58: chdir.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.PathKey.Inode))
	},
	// 59: chdir.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.Mode))
	},
	// 60: chdir.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.MTime))
	},
	// 61: chdir.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chdir.File.MountDetached)
	},
	// 62: chdir.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.PathKey.MountID))
	},
	// 63: chdir.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chdir.File.MountVisible)
	},
	// 64: chdir.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chdir.File))
	},
	// 65: chdir.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Chdir.File))
	},
	// 66: chdir.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Chdir.File))
	},
	// 67: chdir.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Chdir.File))
	},
	// 68: chdir.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Chdir.File))
	},
	// 69: chdir.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Chdir.File))
	},
	// 70: chdir.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chdir.File))
	},
	// 71: chdir.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chdir.File))
	},
	// 72: chdir.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chdir.File))
	},
	// 73: chdir.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Chdir.File.FileFields)))
	},
	// 74: chdir.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.UID))
	},
	// 75: chdir.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chdir.File.FileFields))
	},
	// 76: chdir.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.SyscallEvent.Retval))
	},
	// 77: chdir.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chdir.SyscallContext))
	},
	// 78: chmod.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.CTime))
	},
	// 79: chmod.file.destination.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.Mode))
	},
	// 80: chmod.file.destination.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.Mode))
	},
	// 81: chmod.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Chmod.File))
	},
	// 82: chmod.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File))
	},
	// 83: chmod.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.GID))
	},
	// 84: chmod.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields))
	},
	// 85: chmod.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File))
	},
	// 86: chmod.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chmod.File.FileFields))
	},
	// 87: chmod.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.PathKey.Inode))
	},
	// 88: chmod.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.Mode))
	},
	// 89: chmod.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.MTime))
	},
	// 90: chmod.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chmod.File.MountDetached)
	},
	// 91: chmod.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.PathKey.MountID))
	},
	// 92: chmod.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chmod.File.MountVisible)
	},
	// 93: chmod.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File))
	},
	// 94: chmod.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Chmod.File))
	},
	// 95: chmod.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File))
	},
	// 96: chmod.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Chmod.File))
	},
	// 97: chmod.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Chmod.File))
	},
	// 98: chmod.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Chmod.File))
	},
	// 99: chmod.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File))
	},
	// 100: chmod.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File))
	},
	// 101: chmod.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File))
	},
	// 102: chmod.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Chmod.File.FileFields)))
	},
	// 103: chmod.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.UID))
	},
	// 104: chmod.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields))
	},
	// 105: chmod.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.SyscallEvent.Retval))
	},
	// 106: chmod.syscall.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.syscall.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chmod.SyscallContext)))
	},
	// 107: chmod.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chmod.SyscallContext))
	},
	// 108: chown.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.CTime))
	},
	// 109: chown.file.destination.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.GID))
	},
	// 110: chown.file.destination.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown))
	},
	// 111: chown.file.destination.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.UID))
	},
	// 112: chown.file.destination.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown))
	},
	// 113: chown.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Chown.File))
	},
	// 114: chown.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File))
	},
	// 115: chown.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.GID))
	},
	// 116: chown.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields))
	},
	// 117: chown.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File))
	},
	// 118: chown.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chown.File.FileFields))
	},
	// 119: chown.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.PathKey.Inode))
	},
	// 120: chown.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.Mode))
	},
	// 121: chown.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.MTime))
	},
	// 122: chown.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chown.File.MountDetached)
	},
	// 123: chown.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.PathKey.MountID))
	},
	// 124: chown.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chown.File.MountVisible)
	},
	// 125: chown.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File))
	},
	// 126: chown.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Chown.File))
	},
	// 127: chown.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File))
	},
	// 128: chown.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Chown.File))
	},
	// 129: chown.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Chown.File))
	},
	// 130: chown.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Chown.File))
	},
	// 131: chown.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File))
	},
	// 132: chown.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File))
	},
	// 133: chown.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File))
	},
	// 134: chown.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Chown.File.FileFields)))
	},
	// 135: chown.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.UID))
	},
	// 136: chown.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields))
	},
	// 137: chown.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.SyscallEvent.Retval))
	},
	// 138: chown.syscall.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.syscall.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chown.SyscallContext)))
	},
	// 139: chown.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chown.SyscallContext))
	},
	// 140: chown.syscall.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.syscall.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chown.SyscallContext)))
	},
	// 141: connect.addr.family
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.AddrFamily))
	},
	// 142: connect.addr.hostname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.hostname")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveConnectHostnames(ev, &ev.Connect))
	},
	// 143: connect.addr.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Connect.Addr.IPNet)
	},
	// 144: connect.addr.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.Connect.Addr))
	},
	// 145: connect.addr.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.Addr.Port))
	},
	// 146: connect.protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.Protocol))
	},
	// 147: connect.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.SyscallEvent.Retval))
	},
	// 148: dns.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.ID))
	},
	// 149: dns.question.class
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.class")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Class))
	},
	// 150: dns.question.count
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.count")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Count))
	},
	// 151: dns.question.length
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.length")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Size))
	},
	// 152: dns.question.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DNS.Question.Name)
	},
	// 153: dns.question.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Type))
	},
	// 154: dns.response.cnames
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.response.cnames")
		ev := ctx.Event.(*model.Event)
		if !ev.DNS.HasResponse() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.DNS.Response.CNames)
	},
	// 155: dns.response.code
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.response.code")
		ev := ctx.Event.(*model.Event)
		if !ev.DNS.HasResponse() {
			return types.Int(-1)
		}
		return types.Int(int(ev.DNS.Response.ResponseCode))
	},
	// 156: dns.response.ips
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.response.ips")
		ev := ctx.Event.(*model.Event)
		if !ev.DNS.HasResponse() {
			return cidrsToVal([]net.IPNet{})
		}
		return cidrsToVal(ev.DNS.Response.IPs)
	},
	// 157: event.async
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.async")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveAsync(ev))
	},
	// 158: event.hostname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.hostname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent))
	},
	// 159: event.origin
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.origin")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.Origin)
	},
	// 160: event.os
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.os")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.Os)
	},
	// 161: event.rule.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.rule.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.BaseEvent.RuleTags)
	},
	// 162: event.service
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.service")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent))
	},
	// 163: event.signature
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.signature")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSignature(ev))
	},
	// 164: event.source
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.source")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSource(ev, &ev.BaseEvent))
	},
	// 165: event.timestamp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.timestamp")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)))
	},
	// 166: exec.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process))
	},
	// 167: exec.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process))
	},
	// 168: exec.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process))
	},
	// 169: exec.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process))
	},
	// 170: exec.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process))
	},
	// 171: exec.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process))
	},
	// 172: exec.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.AUID))
	},
	// 173: exec.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.CapEffective))
	},
	// 174: exec.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.CapPermitted))
	},
	// 175: exec.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CapsAttempted))
	},
	// 176: exec.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CapsUsed))
	},
	// 177: exec.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CGroup.CreatedAt))
	},
	// 178: exec.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CGroup.CGroupPathKey.Inode))
	},
	// 179: exec.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CGroup.CGroupPathKey.MountID))
	},
	// 180: exec.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exec.Process.CGroup.CGroupID))
	},
	// 181: exec.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Exec.Process.CGroup))
	},
	// 182: exec.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Comm)
	},
	// 183: exec.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.ContainerContext.CreatedAt))
	},
	// 184: exec.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exec.Process.ContainerContext.ContainerID))
	},
	// 185: exec.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exec.Process.ContainerContext))
	},
	// 186: exec.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)))
	},
	// 187: exec.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.EGID))
	},
	// 188: exec.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.EGroup)
	},
	// 189: exec.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process))
	},
	// 190: exec.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process))
	},
	// 191: exec.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process))
	},
	// 192: exec.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.EUID))
	},
	// 193: exec.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.EUser)
	},
	// 194: exec.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.CTime))
	},
	// 195: exec.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.FileEvent))
	},
	// 196: exec.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent))
	},
	// 197: exec.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.GID))
	},
	// 198: exec.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields))
	},
	// 199: exec.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent))
	},
	// 200: exec.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields))
	},
	// 201: exec.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 202: exec.file.metadata.abi
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.abi")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataABI(ev, &ev.Exec.FileMetadata))
	},
	// 203: exec.file.metadata.architecture
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.architecture")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataArchitecture(ev, &ev.Exec.FileMetadata))
	},
	// 204: exec.file.metadata.compression
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.compression")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataCompression(ev, &ev.Exec.FileMetadata))
	},
	// 205: exec.file.metadata.is_executable
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.is_executable")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileMetadataIsExecutable(ev, &ev.Exec.FileMetadata))
	},
	// 206: exec.file.metadata.is_garble_obfuscated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.is_garble_obfuscated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileMetadataIsGarbleObfuscated(ev, &ev.Exec.FileMetadata))
	},
	// 207: exec.file.metadata.is_upx_packed
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.is_upx_packed")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileMetadataIsUPXPacked(ev, &ev.Exec.FileMetadata))
	},
	// 208: exec.file.metadata.size
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.size")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveFileMetadataSize(ev, &ev.Exec.FileMetadata)))
	},
	// 209: exec.file.metadata.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataType(ev, &ev.Exec.FileMetadata))
	},
	// 210: exec.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.Mode))
	},
	// 211: exec.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.MTime))
	},
	// 212: exec.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.FileEvent.MountDetached)
	},
	// 213: exec.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 214: exec.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.FileEvent.MountVisible)
	},
	// 215: exec.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
	},
	// 216: exec.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exec.Process.FileEvent))
	},
	// 217: exec.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent))
	},
	// 218: exec.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exec.Process.FileEvent))
	},
	// 219: exec.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exec.Process.FileEvent))
	},
	// 220: exec.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exec.Process.FileEvent))
	},
	// 221: exec.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent))
	},
	// 222: exec.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent))
	},
	// 223: exec.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
	},
	// 224: exec.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields)))
	},
	// 225: exec.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.UID))
	},
	// 226: exec.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields))
	},
	// 227: exec.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.FSGID))
	},
	// 228: exec.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.FSGroup)
	},
	// 229: exec.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.FSUID))
	},
	// 230: exec.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.FSUser)
	},
	// 231: exec.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.GID))
	},
	// 232: exec.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.Group)
	},
	// 233: exec.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 234: exec.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 235: exec.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 236: exec.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 237: exec.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 238: exec.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 239: exec.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 240: exec.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 241: exec.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 242: exec.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 243: exec.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 244: exec.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 245: exec.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 246: exec.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 247: exec.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 248: exec.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 249: exec.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 250: exec.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 251: exec.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 252: exec.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 253: exec.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 254: exec.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	// 255: exec.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 256: exec.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 257: exec.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 258: exec.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exec.Process.IsExec)
	},
	// 259: exec.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exec.Process.PIDContext.IsKworker)
	},
	// 260: exec.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exec.Process))
	},
	// 261: exec.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.MntNS))
	},
	// 262: exec.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.NetNS))
	},
	// 263: exec.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.Pid))
	},
	// 264: exec.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PPid))
	},
	// 265: exec.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.SID))
	},
	// 266: exec.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Exec.SyscallContext))
	},
	// 267: exec.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.Tid))
	},
	// 268: exec.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.TTYName)
	},
	// 269: exec.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.UID))
	},
	// 270: exec.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.User)
	},
	// 271: exec.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Exec.Process.UserSession))
	},
	// 272: exec.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Exec.Process.UserSession))
	},
	// 273: exec.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exec.Process.UserSession.K8SSessionContext))
	},
	// 274: exec.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 275: exec.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exec.Process.UserSession.K8SSessionContext))
	},
	// 276: exec.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exec.Process.UserSession.K8SSessionContext))
	},
	// 277: exec.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Exec.Process.UserSession))
	},
	// 278: exec.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exec.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 279: exec.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Exec.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 280: exec.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exec.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 281: exec.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 282: exec.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 283: exit.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process))
	},
	// 284: exit.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process))
	},
	// 285: exit.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process))
	},
	// 286: exit.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process))
	},
	// 287: exit.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process))
	},
	// 288: exit.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process))
	},
	// 289: exit.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.AUID))
	},
	// 290: exit.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.CapEffective))
	},
	// 291: exit.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.CapPermitted))
	},
	// 292: exit.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CapsAttempted))
	},
	// 293: exit.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CapsUsed))
	},
	// 294: exit.cause
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cause")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Cause))
	},
	// 295: exit.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CGroup.CreatedAt))
	},
	// 296: exit.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CGroup.CGroupPathKey.Inode))
	},
	// 297: exit.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CGroup.CGroupPathKey.MountID))
	},
	// 298: exit.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exit.Process.CGroup.CGroupID))
	},
	// 299: exit.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Exit.Process.CGroup))
	},
	// 300: exit.code
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.code")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Code))
	},
	// 301: exit.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Comm)
	},
	// 302: exit.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.ContainerContext.CreatedAt))
	},
	// 303: exit.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exit.Process.ContainerContext.ContainerID))
	},
	// 304: exit.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exit.Process.ContainerContext))
	},
	// 305: exit.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)))
	},
	// 306: exit.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.EGID))
	},
	// 307: exit.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.EGroup)
	},
	// 308: exit.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process))
	},
	// 309: exit.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process))
	},
	// 310: exit.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process))
	},
	// 311: exit.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.EUID))
	},
	// 312: exit.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.EUser)
	},
	// 313: exit.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.CTime))
	},
	// 314: exit.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.FileEvent))
	},
	// 315: exit.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent))
	},
	// 316: exit.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.GID))
	},
	// 317: exit.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields))
	},
	// 318: exit.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent))
	},
	// 319: exit.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields))
	},
	// 320: exit.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 321: exit.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.Mode))
	},
	// 322: exit.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.MTime))
	},
	// 323: exit.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.FileEvent.MountDetached)
	},
	// 324: exit.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 325: exit.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.FileEvent.MountVisible)
	},
	// 326: exit.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
	},
	// 327: exit.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exit.Process.FileEvent))
	},
	// 328: exit.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent))
	},
	// 329: exit.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exit.Process.FileEvent))
	},
	// 330: exit.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exit.Process.FileEvent))
	},
	// 331: exit.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exit.Process.FileEvent))
	},
	// 332: exit.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent))
	},
	// 333: exit.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent))
	},
	// 334: exit.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
	},
	// 335: exit.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields)))
	},
	// 336: exit.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.UID))
	},
	// 337: exit.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields))
	},
	// 338: exit.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.FSGID))
	},
	// 339: exit.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.FSGroup)
	},
	// 340: exit.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.FSUID))
	},
	// 341: exit.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.FSUser)
	},
	// 342: exit.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.GID))
	},
	// 343: exit.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.Group)
	},
	// 344: exit.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 345: exit.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 346: exit.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 347: exit.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 348: exit.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 349: exit.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 350: exit.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 351: exit.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 352: exit.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 353: exit.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 354: exit.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 355: exit.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 356: exit.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 357: exit.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 358: exit.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 359: exit.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 360: exit.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 361: exit.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 362: exit.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 363: exit.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 364: exit.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 365: exit.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	// 366: exit.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 367: exit.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 368: exit.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 369: exit.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exit.Process.IsExec)
	},
	// 370: exit.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exit.Process.PIDContext.IsKworker)
	},
	// 371: exit.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exit.Process))
	},
	// 372: exit.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.MntNS))
	},
	// 373: exit.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.NetNS))
	},
	// 374: exit.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.Pid))
	},
	// 375: exit.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PPid))
	},
	// 376: exit.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.SID))
	},
	// 377: exit.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.Tid))
	},
	// 378: exit.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.TTYName)
	},
	// 379: exit.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.UID))
	},
	// 380: exit.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.User)
	},
	// 381: exit.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Exit.Process.UserSession))
	},
	// 382: exit.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Exit.Process.UserSession))
	},
	// 383: exit.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exit.Process.UserSession.K8SSessionContext))
	},
	// 384: exit.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 385: exit.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exit.Process.UserSession.K8SSessionContext))
	},
	// 386: exit.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exit.Process.UserSession.K8SSessionContext))
	},
	// 387: exit.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Exit.Process.UserSession))
	},
	// 388: exit.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exit.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 389: exit.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Exit.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 390: exit.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exit.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 391: exit.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 392: exit.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 393: imds.aws.is_imds_v2
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.aws.is_imds_v2")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.IMDS.AWS.IsIMDSv2)
	},
	// 394: imds.aws.security_credentials.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.aws.security_credentials.type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.AWS.SecurityCredentials.Type)
	},
	// 395: imds.cloud_provider
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.cloud_provider")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.CloudProvider)
	},
	// 396: imds.host
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.host")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.Host)
	},
	// 397: imds.server
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.server")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.Server)
	},
	// 398: imds.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.Type)
	},
	// 399: imds.url
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.url")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.URL)
	},
	// 400: imds.user_agent
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.user_agent")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.UserAgent)
	},
	// 401: link.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.CTime))
	},
	// 402: link.file.destination.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.CTime))
	},
	// 403: link.file.destination.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Link.Target))
	},
	// 404: link.file.destination.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target))
	},
	// 405: link.file.destination.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.GID))
	},
	// 406: link.file.destination.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields))
	},
	// 407: link.file.destination.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target))
	},
	// 408: link.file.destination.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields))
	},
	// 409: link.file.destination.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.PathKey.Inode))
	},
	// 410: link.file.destination.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.Mode))
	},
	// 411: link.file.destination.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.MTime))
	},
	// 412: link.file.destination.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Target.MountDetached)
	},
	// 413: link.file.destination.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.PathKey.MountID))
	},
	// 414: link.file.destination.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Target.MountVisible)
	},
	// 415: link.file.destination.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target))
	},
	// 416: link.file.destination.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Link.Target))
	},
	// 417: link.file.destination.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target))
	},
	// 418: link.file.destination.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Link.Target))
	},
	// 419: link.file.destination.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Link.Target))
	},
	// 420: link.file.destination.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Link.Target))
	},
	// 421: link.file.destination.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target))
	},
	// 422: link.file.destination.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target))
	},
	// 423: link.file.destination.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target))
	},
	// 424: link.file.destination.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Link.Target.FileFields)))
	},
	// 425: link.file.destination.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.UID))
	},
	// 426: link.file.destination.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields))
	},
	// 427: link.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Link.Source))
	},
	// 428: link.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source))
	},
	// 429: link.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.GID))
	},
	// 430: link.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields))
	},
	// 431: link.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source))
	},
	// 432: link.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Source.FileFields))
	},
	// 433: link.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.PathKey.Inode))
	},
	// 434: link.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.Mode))
	},
	// 435: link.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.MTime))
	},
	// 436: link.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Source.MountDetached)
	},
	// 437: link.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.PathKey.MountID))
	},
	// 438: link.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Source.MountVisible)
	},
	// 439: link.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source))
	},
	// 440: link.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Link.Source))
	},
	// 441: link.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source))
	},
	// 442: link.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Link.Source))
	},
	// 443: link.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Link.Source))
	},
	// 444: link.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Link.Source))
	},
	// 445: link.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source))
	},
	// 446: link.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source))
	},
	// 447: link.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source))
	},
	// 448: link.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Link.Source.FileFields)))
	},
	// 449: link.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.UID))
	},
	// 450: link.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields))
	},
	// 451: link.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.SyscallEvent.Retval))
	},
	// 452: link.syscall.destination.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.syscall.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Link.SyscallContext))
	},
	// 453: link.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Link.SyscallContext))
	},
	// 454: load_module.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule))
	},
	// 455: load_module.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.ArgsTruncated)
	},
	// 456: load_module.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule))
	},
	// 457: load_module.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.CTime))
	},
	// 458: load_module.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.LoadModule.File))
	},
	// 459: load_module.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File))
	},
	// 460: load_module.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.GID))
	},
	// 461: load_module.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields))
	},
	// 462: load_module.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File))
	},
	// 463: load_module.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.LoadModule.File.FileFields))
	},
	// 464: load_module.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.PathKey.Inode))
	},
	// 465: load_module.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.Mode))
	},
	// 466: load_module.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.MTime))
	},
	// 467: load_module.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.File.MountDetached)
	},
	// 468: load_module.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.PathKey.MountID))
	},
	// 469: load_module.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.File.MountVisible)
	},
	// 470: load_module.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File))
	},
	// 471: load_module.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.LoadModule.File))
	},
	// 472: load_module.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File))
	},
	// 473: load_module.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.LoadModule.File))
	},
	// 474: load_module.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.LoadModule.File))
	},
	// 475: load_module.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.LoadModule.File))
	},
	// 476: load_module.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File))
	},
	// 477: load_module.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File))
	},
	// 478: load_module.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File))
	},
	// 479: load_module.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.LoadModule.File.FileFields)))
	},
	// 480: load_module.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.UID))
	},
	// 481: load_module.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields))
	},
	// 482: load_module.loaded_from_memory
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.loaded_from_memory")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.LoadedFromMemory)
	},
	// 483: load_module.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.LoadModule.Name)
	},
	// 484: load_module.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.SyscallEvent.Retval))
	},
	// 485: mkdir.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.CTime))
	},
	// 486: mkdir.file.destination.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.Mode))
	},
	// 487: mkdir.file.destination.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.Mode))
	},
	// 488: mkdir.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Mkdir.File))
	},
	// 489: mkdir.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File))
	},
	// 490: mkdir.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.GID))
	},
	// 491: mkdir.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields))
	},
	// 492: mkdir.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File))
	},
	// 493: mkdir.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Mkdir.File.FileFields))
	},
	// 494: mkdir.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.PathKey.Inode))
	},
	// 495: mkdir.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.Mode))
	},
	// 496: mkdir.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.MTime))
	},
	// 497: mkdir.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mkdir.File.MountDetached)
	},
	// 498: mkdir.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.PathKey.MountID))
	},
	// 499: mkdir.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mkdir.File.MountVisible)
	},
	// 500: mkdir.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File))
	},
	// 501: mkdir.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Mkdir.File))
	},
	// 502: mkdir.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File))
	},
	// 503: mkdir.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Mkdir.File))
	},
	// 504: mkdir.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Mkdir.File))
	},
	// 505: mkdir.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Mkdir.File))
	},
	// 506: mkdir.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File))
	},
	// 507: mkdir.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File))
	},
	// 508: mkdir.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File))
	},
	// 509: mkdir.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Mkdir.File.FileFields)))
	},
	// 510: mkdir.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.UID))
	},
	// 511: mkdir.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields))
	},
	// 512: mkdir.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.SyscallEvent.Retval))
	},
	// 513: mkdir.syscall.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.syscall.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Mkdir.SyscallContext)))
	},
	// 514: mkdir.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mkdir.SyscallContext))
	},
	// 515: mmap.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.CTime))
	},
	// 516: mmap.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.MMap.File))
	},
	// 517: mmap.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File))
	},
	// 518: mmap.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.GID))
	},
	// 519: mmap.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields))
	},
	// 520: mmap.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File))
	},
	// 521: mmap.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.MMap.File.FileFields))
	},
	// 522: mmap.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.PathKey.Inode))
	},
	// 523: mmap.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.Mode))
	},
	// 524: mmap.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.MTime))
	},
	// 525: mmap.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.MMap.File.MountDetached)
	},
	// 526: mmap.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.PathKey.MountID))
	},
	// 527: mmap.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.MMap.File.MountVisible)
	},
	// 528: mmap.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File))
	},
	// 529: mmap.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.MMap.File))
	},
	// 530: mmap.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File))
	},
	// 531: mmap.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.MMap.File))
	},
	// 532: mmap.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.MMap.File))
	},
	// 533: mmap.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.MMap.File))
	},
	// 534: mmap.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File))
	},
	// 535: mmap.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File))
	},
	// 536: mmap.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File))
	},
	// 537: mmap.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.MMap.File.FileFields)))
	},
	// 538: mmap.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.UID))
	},
	// 539: mmap.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields))
	},
	// 540: mmap.flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.Flags))
	},
	// 541: mmap.protection
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.protection")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.Protection))
	},
	// 542: mmap.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.SyscallEvent.Retval))
	},
	// 543: mount.detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mount.Mount.Detached)
	},
	// 544: mount.fs_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.fs_type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Mount.Mount.FSType)
	},
	// 545: mount.mountpoint.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.mountpoint.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount))
	},
	// 546: mount.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mount.SyscallEvent.Retval))
	},
	// 547: mount.root.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.root.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveMountRootPath(ev, &ev.Mount))
	},
	// 548: mount.source.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.source.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount))
	},
	// 549: mount.syscall.fs_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.syscall.fs_type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Mount.SyscallContext))
	},
	// 550: mount.syscall.mountpoint.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.syscall.mountpoint.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Mount.SyscallContext))
	},
	// 551: mount.syscall.source.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.syscall.source.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mount.SyscallContext))
	},
	// 552: mount.visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mount.Mount.Visible)
	},
	// 553: mprotect.req_protection
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mprotect.req_protection")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.MProtect.ReqProtection)
	},
	// 554: mprotect.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mprotect.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MProtect.SyscallEvent.Retval))
	},
	// 555: mprotect.vm_protection
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mprotect.vm_protection")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.MProtect.VMProtection)
	},
	// 556: network.destination.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.destination.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.NetworkContext.Destination.IPNet)
	},
	// 557: network.destination.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.destination.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.NetworkContext.Destination))
	},
	// 558: network.destination.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.destination.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Destination.Port))
	},
	// 559: network.device.ifname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.device.ifname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkContext.Device))
	},
	// 560: network.device.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.device.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Device.NetNS))
	},
	// 561: network.l3_protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.l3_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.L3Protocol))
	},
	// 562: network.l4_protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.l4_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.L4Protocol))
	},
	// 563: network.network_direction
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.network_direction")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.NetworkDirection))
	},
	// 564: network.size
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.size")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Size))
	},
	// 565: network.source.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.source.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.NetworkContext.Source.IPNet)
	},
	// 566: network.source.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.source.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.NetworkContext.Source))
	},
	// 567: network.source.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.source.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Source.Port))
	},
	// 568: network.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Type))
	},
	// 569: network_flow_monitor.device.ifname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.device.ifname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkFlowMonitor.Device))
	},
	// 570: network_flow_monitor.device.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.device.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkFlowMonitor.Device.NetNS))
	},
	// 571: network_flow_monitor.flows.destination.ip
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.destination.ip")
		element := *(e.(*model.Flow))
		return cidrToVal(element.Destination.IPNet)
	},
	// 572: network_flow_monitor.flows.destination.is_public
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.destination.is_public")
		element := *(e.(*model.Flow))
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &element.Destination))
	},
	// 573: network_flow_monitor.flows.destination.port
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.destination.port")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Destination.Port))
	},
	// 574: network_flow_monitor.flows.egress.data_size
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.egress.data_size")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Egress.DataSize))
	},
	// 575: network_flow_monitor.flows.egress.packet_count
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.egress.packet_count")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Egress.PacketCount))
	},
	// 576: network_flow_monitor.flows.ingress.data_size
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.ingress.data_size")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Ingress.DataSize))
	},
	// 577: network_flow_monitor.flows.ingress.packet_count
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.ingress.packet_count")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Ingress.PacketCount))
	},
	// 578: network_flow_monitor.flows.l3_protocol
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.l3_protocol")
		element := *(e.(*model.Flow))
		return types.Int(int(element.L3Protocol))
	},
	// 579: network_flow_monitor.flows.l4_protocol
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.l4_protocol")
		element := *(e.(*model.Flow))
		return types.Int(int(element.L4Protocol))
	},
	// 580: network_flow_monitor.flows.source.ip
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.source.ip")
		element := *(e.(*model.Flow))
		return cidrToVal(element.Source.IPNet)
	},
	// 581: network_flow_monitor.flows.source.is_public
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.source.is_public")
		element := *(e.(*model.Flow))
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &element.Source))
	},
	// 582: network_flow_monitor.flows.source.port
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.source.port")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Source.Port))
	},
	// 583: ondemand.arg1.str
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg1.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg1Str(ev, &ev.OnDemand))
	},
	// 584: ondemand.arg1.uint
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg1.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg1Uint(ev, &ev.OnDemand)))
	},
	// 585: ondemand.arg2.str
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg2.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg2Str(ev, &ev.OnDemand))
	},
	// 586: ondemand.arg2.uint
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg2.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg2Uint(ev, &ev.OnDemand)))
	},
	// 587: ondemand.arg3.str
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg3.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg3Str(ev, &ev.OnDemand))
	},
	// 588: ondemand.arg3.uint
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg3.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg3Uint(ev, &ev.OnDemand)))
	},
	// 589: ondemand.arg4.str
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg4.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg4Str(ev, &ev.OnDemand))
	},
	// 590: ondemand.arg4.uint
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg4.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg4Uint(ev, &ev.OnDemand)))
	},
	// 591: ondemand.arg5.str
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg5.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg5Str(ev, &ev.OnDemand))
	},
	// 592: ondemand.arg5.uint
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg5.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg5Uint(ev, &ev.OnDemand)))
	},
	// 593: ondemand.arg6.str
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg6.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg6Str(ev, &ev.OnDemand))
	},
	// 594: ondemand.arg6.uint
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg6.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg6Uint(ev, &ev.OnDemand)))
	},
	// 595: ondemand.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandName(ev, &ev.OnDemand))
	},
	// 596: open.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.CTime))
	},
	// 597: open.file.destination.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.Mode))
	},
	// 598: open.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Open.File))
	},
	// 599: open.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File))
	},
	// 600: open.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.GID))
	},
	// 601: open.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields))
	},
	// 602: open.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File))
	},
	// 603: open.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Open.File.FileFields))
	},
	// 604: open.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.PathKey.Inode))
	},
	// 605: open.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.Mode))
	},
	// 606: open.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.MTime))
	},
	// 607: open.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Open.File.MountDetached)
	},
	// 608: open.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.PathKey.MountID))
	},
	// 609: open.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Open.File.MountVisible)
	},
	// 610: open.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File))
	},
	// 611: open.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Open.File))
	},
	// 612: open.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File))
	},
	// 613: open.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Open.File))
	},
	// 614: open.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Open.File))
	},
	// 615: open.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Open.File))
	},
	// 616: open.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File))
	},
	// 617: open.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File))
	},
	// 618: open.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File))
	},
	// 619: open.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Open.File.FileFields)))
	},
	// 620: open.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.UID))
	},
	// 621: open.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields))
	},
	// 622: open.flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.Flags))
	},
	// 623: open.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.SyscallEvent.Retval))
	},
	// 624: open.syscall.flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.syscall.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Open.SyscallContext)))
	},
	// 625: open.syscall.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.syscall.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Open.SyscallContext)))
	},
	// 626: open.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Open.SyscallContext))
	},
	// 627: packet.destination.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.destination.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.RawPacket.NetworkContext.Destination.IPNet)
	},
	// 628: packet.destination.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.destination.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.RawPacket.NetworkContext.Destination))
	},
	// 629: packet.destination.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.destination.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Destination.Port))
	},
	// 630: packet.device.ifname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.device.ifname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.RawPacket.NetworkContext.Device))
	},
	// 631: packet.device.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.device.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Device.NetNS))
	},
	// 632: packet.filter
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.filter")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.RawPacket.Filter)
	},
	// 633: packet.l3_protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.l3_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.L3Protocol))
	},
	// 634: packet.l4_protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.l4_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.L4Protocol))
	},
	// 635: packet.network_direction
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.network_direction")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.NetworkDirection))
	},
	// 636: packet.size
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.size")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Size))
	},
	// 637: packet.source.ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.source.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.RawPacket.NetworkContext.Source.IPNet)
	},
	// 638: packet.source.is_public
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.source.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.RawPacket.NetworkContext.Source))
	},
	// 639: packet.source.port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.source.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Source.Port))
	},
	// 640: packet.tls.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.tls.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.TLSContext.Version))
	},
	// 641: packet.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Type))
	},
	// 642: prctl.is_name_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.is_name_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.PrCtl.IsNameTruncated)
	},
	// 643: prctl.new_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.new_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PrCtl.NewName)
	},
	// 644: prctl.option
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.option")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.PrCtl.Option)
	},
	// 645: prctl.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PrCtl.SyscallEvent.Retval))
	},
	// 646: process.ancestors.args
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	// 647: process.ancestors.args_flags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	// 648: process.ancestors.args_options
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	// 649: process.ancestors.args_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	// 650: process.ancestors.argv
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	// 651: process.ancestors.argv0
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	// 652: process.ancestors.auid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	// 653: process.ancestors.cap_effective
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	// 654: process.ancestors.cap_permitted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	// 655: process.ancestors.caps_attempted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	// 656: process.ancestors.caps_used
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	// 657: process.ancestors.cgroup.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	// 658: process.ancestors.cgroup.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	// 659: process.ancestors.cgroup.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	// 660: process.ancestors.cgroup.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	// 661: process.ancestors.cgroup.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	// 662: process.ancestors.comm
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	// 663: process.ancestors.container.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 664: process.ancestors.container.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 665: process.ancestors.container.tags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	// 666: process.ancestors.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	// 667: process.ancestors.egid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	// 668: process.ancestors.egroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	// 669: process.ancestors.envp
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	// 670: process.ancestors.envs
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	// 671: process.ancestors.envs_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	// 672: process.ancestors.euid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	// 673: process.ancestors.euser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	// 674: process.ancestors.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	// 675: process.ancestors.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 676: process.ancestors.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 677: process.ancestors.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	// 678: process.ancestors.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 679: process.ancestors.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 680: process.ancestors.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 681: process.ancestors.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 682: process.ancestors.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	// 683: process.ancestors.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	// 684: process.ancestors.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	// 685: process.ancestors.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 686: process.ancestors.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	// 687: process.ancestors.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 688: process.ancestors.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 689: process.ancestors.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 690: process.ancestors.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 691: process.ancestors.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 692: process.ancestors.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 693: process.ancestors.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 694: process.ancestors.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 695: process.ancestors.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 696: process.ancestors.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	// 697: process.ancestors.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	// 698: process.ancestors.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 699: process.ancestors.fsgid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	// 700: process.ancestors.fsgroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	// 701: process.ancestors.fsuid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	// 702: process.ancestors.fsuser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	// 703: process.ancestors.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	// 704: process.ancestors.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	// 705: process.ancestors.interpreter.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 706: process.ancestors.interpreter.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 707: process.ancestors.interpreter.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 708: process.ancestors.interpreter.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 709: process.ancestors.interpreter.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 710: process.ancestors.interpreter.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 711: process.ancestors.interpreter.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 712: process.ancestors.interpreter.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 713: process.ancestors.interpreter.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 714: process.ancestors.interpreter.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 715: process.ancestors.interpreter.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 716: process.ancestors.interpreter.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 717: process.ancestors.interpreter.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 718: process.ancestors.interpreter.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 719: process.ancestors.interpreter.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 720: process.ancestors.interpreter.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 721: process.ancestors.interpreter.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 722: process.ancestors.interpreter.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 723: process.ancestors.interpreter.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 724: process.ancestors.interpreter.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 725: process.ancestors.interpreter.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 726: process.ancestors.interpreter.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 727: process.ancestors.interpreter.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 728: process.ancestors.interpreter.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 729: process.ancestors.interpreter.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 730: process.ancestors.is_exec
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	// 731: process.ancestors.is_kworker
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	// 732: process.ancestors.is_thread
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	// 733: process.ancestors.mntns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	// 734: process.ancestors.netns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	// 735: process.ancestors.pid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	// 736: process.ancestors.ppid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	// 737: process.ancestors.sid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	// 738: process.ancestors.tid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	// 739: process.ancestors.tty_name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	// 740: process.ancestors.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	// 741: process.ancestors.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	// 742: process.ancestors.user_session.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	// 743: process.ancestors.user_session.identity
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	// 744: process.ancestors.user_session.k8s_groups
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 745: process.ancestors.user_session.k8s_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 746: process.ancestors.user_session.k8s_uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 747: process.ancestors.user_session.k8s_username
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 748: process.ancestors.user_session.session_type
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	// 749: process.ancestors.user_session.ssh_auth_method
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	// 750: process.ancestors.user_session.ssh_client_ip
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 751: process.ancestors.user_session.ssh_client_port
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	// 752: process.ancestors.user_session.ssh_public_key
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 753: process.ancestors.user_session.ssh_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 754: process.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 755: process.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 756: process.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 757: process.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 758: process.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 759: process.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 760: process.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.AUID))
	},
	// 761: process.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.CapEffective))
	},
	// 762: process.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.CapPermitted))
	},
	// 763: process.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CapsAttempted))
	},
	// 764: process.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CapsUsed))
	},
	// 765: process.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CGroup.CreatedAt))
	},
	// 766: process.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	// 767: process.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	// 768: process.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.BaseEvent.ProcessContext.Process.CGroup.CGroupID))
	},
	// 769: process.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.BaseEvent.ProcessContext.Process.CGroup))
	},
	// 770: process.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Comm)
	},
	// 771: process.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 772: process.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.BaseEvent.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 773: process.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Process.ContainerContext))
	},
	// 774: process.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)))
	},
	// 775: process.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.EGID))
	},
	// 776: process.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.EGroup)
	},
	// 777: process.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 778: process.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 779: process.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 780: process.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.EUID))
	},
	// 781: process.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.EUser)
	},
	// 782: process.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	// 783: process.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 784: process.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 785: process.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	// 786: process.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
	},
	// 787: process.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 788: process.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
	},
	// 789: process.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 790: process.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	// 791: process.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	// 792: process.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.FileEvent.MountDetached)
	},
	// 793: process.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 794: process.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.FileEvent.MountVisible)
	},
	// 795: process.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 796: process.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 797: process.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 798: process.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 799: process.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 800: process.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 801: process.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 802: process.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 803: process.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 804: process.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)))
	},
	// 805: process.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	// 806: process.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
	},
	// 807: process.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.FSGID))
	},
	// 808: process.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.FSGroup)
	},
	// 809: process.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.FSUID))
	},
	// 810: process.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.FSUser)
	},
	// 811: process.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.GID))
	},
	// 812: process.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.Group)
	},
	// 813: process.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 814: process.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 815: process.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 816: process.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 817: process.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 818: process.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 819: process.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 820: process.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 821: process.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 822: process.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 823: process.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 824: process.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 825: process.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 826: process.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 827: process.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 828: process.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 829: process.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 830: process.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 831: process.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 832: process.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 833: process.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 834: process.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 835: process.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 836: process.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 837: process.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 838: process.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.BaseEvent.ProcessContext.Process.IsExec)
	},
	// 839: process.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.BaseEvent.ProcessContext.Process.PIDContext.IsKworker)
	},
	// 840: process.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 841: process.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.MntNS))
	},
	// 842: process.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.NetNS))
	},
	// 843: process.parent.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 844: process.parent.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 845: process.parent.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 846: process.parent.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 847: process.parent.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 848: process.parent.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 849: process.parent.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.AUID))
	},
	// 850: process.parent.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.CapEffective))
	},
	// 851: process.parent.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.CapPermitted))
	},
	// 852: process.parent.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CapsAttempted))
	},
	// 853: process.parent.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CapsUsed))
	},
	// 854: process.parent.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CGroup.CreatedAt))
	},
	// 855: process.parent.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupPathKey.Inode))
	},
	// 856: process.parent.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupPathKey.MountID))
	},
	// 857: process.parent.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupID))
	},
	// 858: process.parent.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.BaseEvent.ProcessContext.Parent.CGroup))
	},
	// 859: process.parent.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Comm)
	},
	// 860: process.parent.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.ContainerContext.CreatedAt))
	},
	// 861: process.parent.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.BaseEvent.ProcessContext.Parent.ContainerContext.ContainerID))
	},
	// 862: process.parent.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Parent.ContainerContext))
	},
	// 863: process.parent.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)))
	},
	// 864: process.parent.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.EGID))
	},
	// 865: process.parent.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.EGroup)
	},
	// 866: process.parent.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 867: process.parent.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 868: process.parent.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 869: process.parent.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.EUID))
	},
	// 870: process.parent.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.EUser)
	},
	// 871: process.parent.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.CTime))
	},
	// 872: process.parent.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 873: process.parent.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 874: process.parent.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.GID))
	},
	// 875: process.parent.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields))
	},
	// 876: process.parent.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 877: process.parent.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields))
	},
	// 878: process.parent.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.PathKey.Inode))
	},
	// 879: process.parent.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.Mode))
	},
	// 880: process.parent.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.MTime))
	},
	// 881: process.parent.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.FileEvent.MountDetached)
	},
	// 882: process.parent.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.PathKey.MountID))
	},
	// 883: process.parent.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.FileEvent.MountVisible)
	},
	// 884: process.parent.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 885: process.parent.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 886: process.parent.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 887: process.parent.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 888: process.parent.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 889: process.parent.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 890: process.parent.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 891: process.parent.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 892: process.parent.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 893: process.parent.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields)))
	},
	// 894: process.parent.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields.UID))
	},
	// 895: process.parent.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent.FileFields))
	},
	// 896: process.parent.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.FSGID))
	},
	// 897: process.parent.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.FSGroup)
	},
	// 898: process.parent.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.FSUID))
	},
	// 899: process.parent.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.FSUser)
	},
	// 900: process.parent.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.GID))
	},
	// 901: process.parent.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.Group)
	},
	// 902: process.parent.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 903: process.parent.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 904: process.parent.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 905: process.parent.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 906: process.parent.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 907: process.parent.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 908: process.parent.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 909: process.parent.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 910: process.parent.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 911: process.parent.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 912: process.parent.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.MountDetached)
	},
	// 913: process.parent.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 914: process.parent.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.MountVisible)
	},
	// 915: process.parent.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 916: process.parent.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 917: process.parent.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 918: process.parent.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 919: process.parent.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 920: process.parent.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 921: process.parent.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 922: process.parent.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 923: process.parent.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent))
	},
	// 924: process.parent.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields)))
	},
	// 925: process.parent.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 926: process.parent.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		if !ev.BaseEvent.ProcessContext.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 927: process.parent.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.IsExec)
	},
	// 928: process.parent.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.PIDContext.IsKworker)
	},
	// 929: process.parent.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 930: process.parent.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.MntNS))
	},
	// 931: process.parent.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.NetNS))
	},
	// 932: process.parent.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid))
	},
	// 933: process.parent.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PPid))
	},
	// 934: process.parent.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.SID))
	},
	// 935: process.parent.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Tid))
	},
	// 936: process.parent.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.TTYName)
	},
	// 937: process.parent.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.UID))
	},
	// 938: process.parent.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.User)
	},
	// 939: process.parent.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession))
	},
	// 940: process.parent.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession))
	},
	// 941: process.parent.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext))
	},
	// 942: process.parent.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 943: process.parent.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext))
	},
	// 944: process.parent.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext))
	},
	// 945: process.parent.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession))
	},
	// 946: process.parent.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 947: process.parent.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 948: process.parent.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 949: process.parent.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 950: process.parent.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 951: process.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid))
	},
	// 952: process.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PPid))
	},
	// 953: process.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.SID))
	},
	// 954: process.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.Tid))
	},
	// 955: process.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.TTYName)
	},
	// 956: process.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.UID))
	},
	// 957: process.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.User)
	},
	// 958: process.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.BaseEvent.ProcessContext.Process.UserSession))
	},
	// 959: process.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.BaseEvent.ProcessContext.Process.UserSession))
	},
	// 960: process.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 961: process.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 962: process.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 963: process.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 964: process.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.BaseEvent.ProcessContext.Process.UserSession))
	},
	// 965: process.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 966: process.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 967: process.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 968: process.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 969: process.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 970: ptrace.request
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.request")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Request))
	},
	// 971: ptrace.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.SyscallEvent.Retval))
	},
	// 972: ptrace.tracee.ancestors.args
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	// 973: ptrace.tracee.ancestors.args_flags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	// 974: ptrace.tracee.ancestors.args_options
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	// 975: ptrace.tracee.ancestors.args_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	// 976: ptrace.tracee.ancestors.argv
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	// 977: ptrace.tracee.ancestors.argv0
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	// 978: ptrace.tracee.ancestors.auid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	// 979: ptrace.tracee.ancestors.cap_effective
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	// 980: ptrace.tracee.ancestors.cap_permitted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	// 981: ptrace.tracee.ancestors.caps_attempted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	// 982: ptrace.tracee.ancestors.caps_used
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	// 983: ptrace.tracee.ancestors.cgroup.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	// 984: ptrace.tracee.ancestors.cgroup.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	// 985: ptrace.tracee.ancestors.cgroup.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	// 986: ptrace.tracee.ancestors.cgroup.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	// 987: ptrace.tracee.ancestors.cgroup.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	// 988: ptrace.tracee.ancestors.comm
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	// 989: ptrace.tracee.ancestors.container.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 990: ptrace.tracee.ancestors.container.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 991: ptrace.tracee.ancestors.container.tags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	// 992: ptrace.tracee.ancestors.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	// 993: ptrace.tracee.ancestors.egid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	// 994: ptrace.tracee.ancestors.egroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	// 995: ptrace.tracee.ancestors.envp
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	// 996: ptrace.tracee.ancestors.envs
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	// 997: ptrace.tracee.ancestors.envs_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	// 998: ptrace.tracee.ancestors.euid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	// 999: ptrace.tracee.ancestors.euser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	// 1000: ptrace.tracee.ancestors.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	// 1001: ptrace.tracee.ancestors.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1002: ptrace.tracee.ancestors.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1003: ptrace.tracee.ancestors.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	// 1004: ptrace.tracee.ancestors.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1005: ptrace.tracee.ancestors.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1006: ptrace.tracee.ancestors.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1007: ptrace.tracee.ancestors.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 1008: ptrace.tracee.ancestors.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	// 1009: ptrace.tracee.ancestors.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	// 1010: ptrace.tracee.ancestors.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	// 1011: ptrace.tracee.ancestors.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 1012: ptrace.tracee.ancestors.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	// 1013: ptrace.tracee.ancestors.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1014: ptrace.tracee.ancestors.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 1015: ptrace.tracee.ancestors.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1016: ptrace.tracee.ancestors.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1017: ptrace.tracee.ancestors.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 1018: ptrace.tracee.ancestors.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1019: ptrace.tracee.ancestors.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1020: ptrace.tracee.ancestors.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1021: ptrace.tracee.ancestors.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1022: ptrace.tracee.ancestors.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	// 1023: ptrace.tracee.ancestors.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	// 1024: ptrace.tracee.ancestors.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1025: ptrace.tracee.ancestors.fsgid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	// 1026: ptrace.tracee.ancestors.fsgroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	// 1027: ptrace.tracee.ancestors.fsuid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	// 1028: ptrace.tracee.ancestors.fsuser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	// 1029: ptrace.tracee.ancestors.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	// 1030: ptrace.tracee.ancestors.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	// 1031: ptrace.tracee.ancestors.interpreter.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1032: ptrace.tracee.ancestors.interpreter.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1033: ptrace.tracee.ancestors.interpreter.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1034: ptrace.tracee.ancestors.interpreter.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1035: ptrace.tracee.ancestors.interpreter.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1036: ptrace.tracee.ancestors.interpreter.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1037: ptrace.tracee.ancestors.interpreter.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1038: ptrace.tracee.ancestors.interpreter.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1039: ptrace.tracee.ancestors.interpreter.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1040: ptrace.tracee.ancestors.interpreter.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1041: ptrace.tracee.ancestors.interpreter.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1042: ptrace.tracee.ancestors.interpreter.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1043: ptrace.tracee.ancestors.interpreter.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1044: ptrace.tracee.ancestors.interpreter.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1045: ptrace.tracee.ancestors.interpreter.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 1046: ptrace.tracee.ancestors.interpreter.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1047: ptrace.tracee.ancestors.interpreter.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1048: ptrace.tracee.ancestors.interpreter.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 1049: ptrace.tracee.ancestors.interpreter.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1050: ptrace.tracee.ancestors.interpreter.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1051: ptrace.tracee.ancestors.interpreter.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1052: ptrace.tracee.ancestors.interpreter.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1053: ptrace.tracee.ancestors.interpreter.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1054: ptrace.tracee.ancestors.interpreter.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1055: ptrace.tracee.ancestors.interpreter.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1056: ptrace.tracee.ancestors.is_exec
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	// 1057: ptrace.tracee.ancestors.is_kworker
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	// 1058: ptrace.tracee.ancestors.is_thread
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	// 1059: ptrace.tracee.ancestors.mntns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	// 1060: ptrace.tracee.ancestors.netns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	// 1061: ptrace.tracee.ancestors.pid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	// 1062: ptrace.tracee.ancestors.ppid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	// 1063: ptrace.tracee.ancestors.sid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	// 1064: ptrace.tracee.ancestors.tid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	// 1065: ptrace.tracee.ancestors.tty_name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	// 1066: ptrace.tracee.ancestors.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	// 1067: ptrace.tracee.ancestors.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	// 1068: ptrace.tracee.ancestors.user_session.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	// 1069: ptrace.tracee.ancestors.user_session.identity
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	// 1070: ptrace.tracee.ancestors.user_session.k8s_groups
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1071: ptrace.tracee.ancestors.user_session.k8s_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1072: ptrace.tracee.ancestors.user_session.k8s_uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1073: ptrace.tracee.ancestors.user_session.k8s_username
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1074: ptrace.tracee.ancestors.user_session.session_type
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	// 1075: ptrace.tracee.ancestors.user_session.ssh_auth_method
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	// 1076: ptrace.tracee.ancestors.user_session.ssh_client_ip
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1077: ptrace.tracee.ancestors.user_session.ssh_client_port
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	// 1078: ptrace.tracee.ancestors.user_session.ssh_public_key
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1079: ptrace.tracee.ancestors.user_session.ssh_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1080: ptrace.tracee.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process))
	},
	// 1081: ptrace.tracee.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.PTrace.Tracee.Process))
	},
	// 1082: ptrace.tracee.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.PTrace.Tracee.Process))
	},
	// 1083: ptrace.tracee.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process))
	},
	// 1084: ptrace.tracee.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process))
	},
	// 1085: ptrace.tracee.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process))
	},
	// 1086: ptrace.tracee.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.AUID))
	},
	// 1087: ptrace.tracee.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.CapEffective))
	},
	// 1088: ptrace.tracee.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.CapPermitted))
	},
	// 1089: ptrace.tracee.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CapsAttempted))
	},
	// 1090: ptrace.tracee.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CapsUsed))
	},
	// 1091: ptrace.tracee.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CGroup.CreatedAt))
	},
	// 1092: ptrace.tracee.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CGroup.CGroupPathKey.Inode))
	},
	// 1093: ptrace.tracee.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CGroup.CGroupPathKey.MountID))
	},
	// 1094: ptrace.tracee.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.PTrace.Tracee.Process.CGroup.CGroupID))
	},
	// 1095: ptrace.tracee.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.PTrace.Tracee.Process.CGroup))
	},
	// 1096: ptrace.tracee.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Comm)
	},
	// 1097: ptrace.tracee.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.ContainerContext.CreatedAt))
	},
	// 1098: ptrace.tracee.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.PTrace.Tracee.Process.ContainerContext.ContainerID))
	},
	// 1099: ptrace.tracee.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.PTrace.Tracee.Process.ContainerContext))
	},
	// 1100: ptrace.tracee.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process)))
	},
	// 1101: ptrace.tracee.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.EGID))
	},
	// 1102: ptrace.tracee.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.EGroup)
	},
	// 1103: ptrace.tracee.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process))
	},
	// 1104: ptrace.tracee.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process))
	},
	// 1105: ptrace.tracee.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process))
	},
	// 1106: ptrace.tracee.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.EUID))
	},
	// 1107: ptrace.tracee.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.EUser)
	},
	// 1108: ptrace.tracee.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.CTime))
	},
	// 1109: ptrace.tracee.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1110: ptrace.tracee.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1111: ptrace.tracee.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.GID))
	},
	// 1112: ptrace.tracee.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
	},
	// 1113: ptrace.tracee.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1114: ptrace.tracee.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
	},
	// 1115: ptrace.tracee.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 1116: ptrace.tracee.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.Mode))
	},
	// 1117: ptrace.tracee.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.MTime))
	},
	// 1118: ptrace.tracee.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.FileEvent.MountDetached)
	},
	// 1119: ptrace.tracee.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 1120: ptrace.tracee.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.FileEvent.MountVisible)
	},
	// 1121: ptrace.tracee.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1122: ptrace.tracee.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1123: ptrace.tracee.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1124: ptrace.tracee.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1125: ptrace.tracee.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1126: ptrace.tracee.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1127: ptrace.tracee.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1128: ptrace.tracee.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1129: ptrace.tracee.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	// 1130: ptrace.tracee.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)))
	},
	// 1131: ptrace.tracee.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.UID))
	},
	// 1132: ptrace.tracee.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
	},
	// 1133: ptrace.tracee.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.FSGID))
	},
	// 1134: ptrace.tracee.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.FSGroup)
	},
	// 1135: ptrace.tracee.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.FSUID))
	},
	// 1136: ptrace.tracee.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.FSUser)
	},
	// 1137: ptrace.tracee.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.GID))
	},
	// 1138: ptrace.tracee.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.Group)
	},
	// 1139: ptrace.tracee.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1140: ptrace.tracee.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1141: ptrace.tracee.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1142: ptrace.tracee.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1143: ptrace.tracee.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1144: ptrace.tracee.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1145: ptrace.tracee.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1146: ptrace.tracee.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1147: ptrace.tracee.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1148: ptrace.tracee.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1149: ptrace.tracee.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1150: ptrace.tracee.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1151: ptrace.tracee.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1152: ptrace.tracee.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1153: ptrace.tracee.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1154: ptrace.tracee.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1155: ptrace.tracee.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1156: ptrace.tracee.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1157: ptrace.tracee.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1158: ptrace.tracee.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1159: ptrace.tracee.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1160: ptrace.tracee.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	// 1161: ptrace.tracee.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1162: ptrace.tracee.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1163: ptrace.tracee.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1164: ptrace.tracee.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.PTrace.Tracee.Process.IsExec)
	},
	// 1165: ptrace.tracee.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.PTrace.Tracee.Process.PIDContext.IsKworker)
	},
	// 1166: ptrace.tracee.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.PTrace.Tracee.Process))
	},
	// 1167: ptrace.tracee.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.MntNS))
	},
	// 1168: ptrace.tracee.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.NetNS))
	},
	// 1169: ptrace.tracee.parent.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent))
	},
	// 1170: ptrace.tracee.parent.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.PTrace.Tracee.Parent))
	},
	// 1171: ptrace.tracee.parent.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.PTrace.Tracee.Parent))
	},
	// 1172: ptrace.tracee.parent.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.PTrace.Tracee.Parent))
	},
	// 1173: ptrace.tracee.parent.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.PTrace.Tracee.Parent))
	},
	// 1174: ptrace.tracee.parent.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent))
	},
	// 1175: ptrace.tracee.parent.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.AUID))
	},
	// 1176: ptrace.tracee.parent.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.CapEffective))
	},
	// 1177: ptrace.tracee.parent.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.CapPermitted))
	},
	// 1178: ptrace.tracee.parent.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CapsAttempted))
	},
	// 1179: ptrace.tracee.parent.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CapsUsed))
	},
	// 1180: ptrace.tracee.parent.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CGroup.CreatedAt))
	},
	// 1181: ptrace.tracee.parent.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CGroup.CGroupPathKey.Inode))
	},
	// 1182: ptrace.tracee.parent.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CGroup.CGroupPathKey.MountID))
	},
	// 1183: ptrace.tracee.parent.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.PTrace.Tracee.Parent.CGroup.CGroupID))
	},
	// 1184: ptrace.tracee.parent.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.PTrace.Tracee.Parent.CGroup))
	},
	// 1185: ptrace.tracee.parent.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Comm)
	},
	// 1186: ptrace.tracee.parent.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.ContainerContext.CreatedAt))
	},
	// 1187: ptrace.tracee.parent.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.PTrace.Tracee.Parent.ContainerContext.ContainerID))
	},
	// 1188: ptrace.tracee.parent.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.PTrace.Tracee.Parent.ContainerContext))
	},
	// 1189: ptrace.tracee.parent.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent)))
	},
	// 1190: ptrace.tracee.parent.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.EGID))
	},
	// 1191: ptrace.tracee.parent.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.EGroup)
	},
	// 1192: ptrace.tracee.parent.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent))
	},
	// 1193: ptrace.tracee.parent.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent))
	},
	// 1194: ptrace.tracee.parent.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.PTrace.Tracee.Parent))
	},
	// 1195: ptrace.tracee.parent.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.EUID))
	},
	// 1196: ptrace.tracee.parent.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.EUser)
	},
	// 1197: ptrace.tracee.parent.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.CTime))
	},
	// 1198: ptrace.tracee.parent.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1199: ptrace.tracee.parent.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1200: ptrace.tracee.parent.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.GID))
	},
	// 1201: ptrace.tracee.parent.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields))
	},
	// 1202: ptrace.tracee.parent.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1203: ptrace.tracee.parent.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields))
	},
	// 1204: ptrace.tracee.parent.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.PathKey.Inode))
	},
	// 1205: ptrace.tracee.parent.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.Mode))
	},
	// 1206: ptrace.tracee.parent.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.MTime))
	},
	// 1207: ptrace.tracee.parent.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.FileEvent.MountDetached)
	},
	// 1208: ptrace.tracee.parent.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.PathKey.MountID))
	},
	// 1209: ptrace.tracee.parent.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.FileEvent.MountVisible)
	},
	// 1210: ptrace.tracee.parent.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1211: ptrace.tracee.parent.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1212: ptrace.tracee.parent.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1213: ptrace.tracee.parent.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1214: ptrace.tracee.parent.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1215: ptrace.tracee.parent.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1216: ptrace.tracee.parent.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1217: ptrace.tracee.parent.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1218: ptrace.tracee.parent.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.FileEvent))
	},
	// 1219: ptrace.tracee.parent.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields)))
	},
	// 1220: ptrace.tracee.parent.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.FileEvent.FileFields.UID))
	},
	// 1221: ptrace.tracee.parent.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.FileEvent.FileFields))
	},
	// 1222: ptrace.tracee.parent.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.FSGID))
	},
	// 1223: ptrace.tracee.parent.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.FSGroup)
	},
	// 1224: ptrace.tracee.parent.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.FSUID))
	},
	// 1225: ptrace.tracee.parent.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.FSUser)
	},
	// 1226: ptrace.tracee.parent.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.GID))
	},
	// 1227: ptrace.tracee.parent.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.Group)
	},
	// 1228: ptrace.tracee.parent.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1229: ptrace.tracee.parent.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1230: ptrace.tracee.parent.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1231: ptrace.tracee.parent.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1232: ptrace.tracee.parent.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 1233: ptrace.tracee.parent.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1234: ptrace.tracee.parent.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 1235: ptrace.tracee.parent.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1236: ptrace.tracee.parent.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1237: ptrace.tracee.parent.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1238: ptrace.tracee.parent.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1239: ptrace.tracee.parent.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1240: ptrace.tracee.parent.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1241: ptrace.tracee.parent.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1242: ptrace.tracee.parent.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1243: ptrace.tracee.parent.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1244: ptrace.tracee.parent.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1245: ptrace.tracee.parent.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1246: ptrace.tracee.parent.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1247: ptrace.tracee.parent.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1248: ptrace.tracee.parent.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1249: ptrace.tracee.parent.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent))
	},
	// 1250: ptrace.tracee.parent.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1251: ptrace.tracee.parent.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1252: ptrace.tracee.parent.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		if !ev.PTrace.Tracee.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 1253: ptrace.tracee.parent.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.IsExec)
	},
	// 1254: ptrace.tracee.parent.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.PIDContext.IsKworker)
	},
	// 1255: ptrace.tracee.parent.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.PTrace.Tracee.Parent))
	},
	// 1256: ptrace.tracee.parent.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.MntNS))
	},
	// 1257: ptrace.tracee.parent.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.NetNS))
	},
	// 1258: ptrace.tracee.parent.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.Pid))
	},
	// 1259: ptrace.tracee.parent.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PPid))
	},
	// 1260: ptrace.tracee.parent.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.SID))
	},
	// 1261: ptrace.tracee.parent.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.Tid))
	},
	// 1262: ptrace.tracee.parent.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.TTYName)
	},
	// 1263: ptrace.tracee.parent.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.UID))
	},
	// 1264: ptrace.tracee.parent.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.User)
	},
	// 1265: ptrace.tracee.parent.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.PTrace.Tracee.Parent.UserSession))
	},
	// 1266: ptrace.tracee.parent.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.PTrace.Tracee.Parent.UserSession))
	},
	// 1267: ptrace.tracee.parent.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext))
	},
	// 1268: ptrace.tracee.parent.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1269: ptrace.tracee.parent.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext))
	},
	// 1270: ptrace.tracee.parent.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext))
	},
	// 1271: ptrace.tracee.parent.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.PTrace.Tracee.Parent.UserSession))
	},
	// 1272: ptrace.tracee.parent.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 1273: ptrace.tracee.parent.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1274: ptrace.tracee.parent.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 1275: ptrace.tracee.parent.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1276: ptrace.tracee.parent.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1277: ptrace.tracee.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.Pid))
	},
	// 1278: ptrace.tracee.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PPid))
	},
	// 1279: ptrace.tracee.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.SID))
	},
	// 1280: ptrace.tracee.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.Tid))
	},
	// 1281: ptrace.tracee.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.TTYName)
	},
	// 1282: ptrace.tracee.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.UID))
	},
	// 1283: ptrace.tracee.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.User)
	},
	// 1284: ptrace.tracee.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.PTrace.Tracee.Process.UserSession))
	},
	// 1285: ptrace.tracee.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.PTrace.Tracee.Process.UserSession))
	},
	// 1286: ptrace.tracee.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Process.UserSession.K8SSessionContext))
	},
	// 1287: ptrace.tracee.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1288: ptrace.tracee.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Process.UserSession.K8SSessionContext))
	},
	// 1289: ptrace.tracee.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Process.UserSession.K8SSessionContext))
	},
	// 1290: ptrace.tracee.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.PTrace.Tracee.Process.UserSession))
	},
	// 1291: ptrace.tracee.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 1292: ptrace.tracee.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1293: ptrace.tracee.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 1294: ptrace.tracee.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1295: ptrace.tracee.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1296: removexattr.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.CTime))
	},
	// 1297: removexattr.file.destination.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr))
	},
	// 1298: removexattr.file.destination.namespace
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.destination.namespace")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr))
	},
	// 1299: removexattr.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.RemoveXAttr.File))
	},
	// 1300: removexattr.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File))
	},
	// 1301: removexattr.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.GID))
	},
	// 1302: removexattr.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields))
	},
	// 1303: removexattr.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File))
	},
	// 1304: removexattr.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.RemoveXAttr.File.FileFields))
	},
	// 1305: removexattr.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.PathKey.Inode))
	},
	// 1306: removexattr.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.Mode))
	},
	// 1307: removexattr.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.MTime))
	},
	// 1308: removexattr.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.RemoveXAttr.File.MountDetached)
	},
	// 1309: removexattr.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.PathKey.MountID))
	},
	// 1310: removexattr.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.RemoveXAttr.File.MountVisible)
	},
	// 1311: removexattr.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File))
	},
	// 1312: removexattr.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.RemoveXAttr.File))
	},
	// 1313: removexattr.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File))
	},
	// 1314: removexattr.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.RemoveXAttr.File))
	},
	// 1315: removexattr.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.RemoveXAttr.File))
	},
	// 1316: removexattr.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.RemoveXAttr.File))
	},
	// 1317: removexattr.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File))
	},
	// 1318: removexattr.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File))
	},
	// 1319: removexattr.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File))
	},
	// 1320: removexattr.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.RemoveXAttr.File.FileFields)))
	},
	// 1321: removexattr.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.UID))
	},
	// 1322: removexattr.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields))
	},
	// 1323: removexattr.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.SyscallEvent.Retval))
	},
	// 1324: rename.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.CTime))
	},
	// 1325: rename.file.destination.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.CTime))
	},
	// 1326: rename.file.destination.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Rename.New))
	},
	// 1327: rename.file.destination.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New))
	},
	// 1328: rename.file.destination.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.GID))
	},
	// 1329: rename.file.destination.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields))
	},
	// 1330: rename.file.destination.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New))
	},
	// 1331: rename.file.destination.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields))
	},
	// 1332: rename.file.destination.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.PathKey.Inode))
	},
	// 1333: rename.file.destination.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.Mode))
	},
	// 1334: rename.file.destination.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.MTime))
	},
	// 1335: rename.file.destination.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.New.MountDetached)
	},
	// 1336: rename.file.destination.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.PathKey.MountID))
	},
	// 1337: rename.file.destination.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.New.MountVisible)
	},
	// 1338: rename.file.destination.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New))
	},
	// 1339: rename.file.destination.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Rename.New))
	},
	// 1340: rename.file.destination.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New))
	},
	// 1341: rename.file.destination.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Rename.New))
	},
	// 1342: rename.file.destination.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Rename.New))
	},
	// 1343: rename.file.destination.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Rename.New))
	},
	// 1344: rename.file.destination.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New))
	},
	// 1345: rename.file.destination.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New))
	},
	// 1346: rename.file.destination.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New))
	},
	// 1347: rename.file.destination.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Rename.New.FileFields)))
	},
	// 1348: rename.file.destination.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.UID))
	},
	// 1349: rename.file.destination.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields))
	},
	// 1350: rename.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Rename.Old))
	},
	// 1351: rename.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old))
	},
	// 1352: rename.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.GID))
	},
	// 1353: rename.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields))
	},
	// 1354: rename.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old))
	},
	// 1355: rename.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.Old.FileFields))
	},
	// 1356: rename.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.PathKey.Inode))
	},
	// 1357: rename.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.Mode))
	},
	// 1358: rename.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.MTime))
	},
	// 1359: rename.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.Old.MountDetached)
	},
	// 1360: rename.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.PathKey.MountID))
	},
	// 1361: rename.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.Old.MountVisible)
	},
	// 1362: rename.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old))
	},
	// 1363: rename.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Rename.Old))
	},
	// 1364: rename.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old))
	},
	// 1365: rename.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Rename.Old))
	},
	// 1366: rename.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Rename.Old))
	},
	// 1367: rename.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Rename.Old))
	},
	// 1368: rename.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old))
	},
	// 1369: rename.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old))
	},
	// 1370: rename.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old))
	},
	// 1371: rename.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Rename.Old.FileFields)))
	},
	// 1372: rename.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.UID))
	},
	// 1373: rename.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields))
	},
	// 1374: rename.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.SyscallEvent.Retval))
	},
	// 1375: rename.syscall.destination.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.syscall.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Rename.SyscallContext))
	},
	// 1376: rename.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rename.SyscallContext))
	},
	// 1377: rmdir.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.CTime))
	},
	// 1378: rmdir.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Rmdir.File))
	},
	// 1379: rmdir.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File))
	},
	// 1380: rmdir.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.GID))
	},
	// 1381: rmdir.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields))
	},
	// 1382: rmdir.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File))
	},
	// 1383: rmdir.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rmdir.File.FileFields))
	},
	// 1384: rmdir.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.PathKey.Inode))
	},
	// 1385: rmdir.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.Mode))
	},
	// 1386: rmdir.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.MTime))
	},
	// 1387: rmdir.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rmdir.File.MountDetached)
	},
	// 1388: rmdir.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.PathKey.MountID))
	},
	// 1389: rmdir.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rmdir.File.MountVisible)
	},
	// 1390: rmdir.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File))
	},
	// 1391: rmdir.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Rmdir.File))
	},
	// 1392: rmdir.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File))
	},
	// 1393: rmdir.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Rmdir.File))
	},
	// 1394: rmdir.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Rmdir.File))
	},
	// 1395: rmdir.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Rmdir.File))
	},
	// 1396: rmdir.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File))
	},
	// 1397: rmdir.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File))
	},
	// 1398: rmdir.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File))
	},
	// 1399: rmdir.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Rmdir.File.FileFields)))
	},
	// 1400: rmdir.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.UID))
	},
	// 1401: rmdir.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields))
	},
	// 1402: rmdir.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.SyscallEvent.Retval))
	},
	// 1403: rmdir.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rmdir.SyscallContext))
	},
	// 1404: selinux.bool.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.bool.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux))
	},
	// 1405: selinux.bool.state
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.bool.state")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SELinux.BoolChangeValue)
	},
	// 1406: selinux.bool_commit.state
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.bool_commit.state")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SELinux.BoolCommitValue)
	},
	// 1407: selinux.enforce.status
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.enforce.status")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SELinux.EnforceStatus)
	},
	// 1408: setgid.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetGID.EGID))
	},
	// 1409: setgid.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID))
	},
	// 1410: setgid.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetGID.FSGID))
	},
	// 1411: setgid.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID))
	},
	// 1412: setgid.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetGID.GID))
	},
	// 1413: setgid.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID))
	},
	// 1414: setrlimit.resource
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.resource")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Setrlimit.Resource)
	},
	// 1415: setrlimit.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.SyscallEvent.Retval))
	},
	// 1416: setrlimit.rlim_cur
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.rlim_cur")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.RlimCur))
	},
	// 1417: setrlimit.rlim_max
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.rlim_max")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.RlimMax))
	},
	// 1418: setrlimit.target.ancestors.args
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	// 1419: setrlimit.target.ancestors.args_flags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	// 1420: setrlimit.target.ancestors.args_options
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	// 1421: setrlimit.target.ancestors.args_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	// 1422: setrlimit.target.ancestors.argv
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	// 1423: setrlimit.target.ancestors.argv0
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	// 1424: setrlimit.target.ancestors.auid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	// 1425: setrlimit.target.ancestors.cap_effective
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	// 1426: setrlimit.target.ancestors.cap_permitted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	// 1427: setrlimit.target.ancestors.caps_attempted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	// 1428: setrlimit.target.ancestors.caps_used
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	// 1429: setrlimit.target.ancestors.cgroup.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	// 1430: setrlimit.target.ancestors.cgroup.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	// 1431: setrlimit.target.ancestors.cgroup.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	// 1432: setrlimit.target.ancestors.cgroup.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	// 1433: setrlimit.target.ancestors.cgroup.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	// 1434: setrlimit.target.ancestors.comm
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	// 1435: setrlimit.target.ancestors.container.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 1436: setrlimit.target.ancestors.container.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 1437: setrlimit.target.ancestors.container.tags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	// 1438: setrlimit.target.ancestors.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	// 1439: setrlimit.target.ancestors.egid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	// 1440: setrlimit.target.ancestors.egroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	// 1441: setrlimit.target.ancestors.envp
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	// 1442: setrlimit.target.ancestors.envs
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	// 1443: setrlimit.target.ancestors.envs_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	// 1444: setrlimit.target.ancestors.euid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	// 1445: setrlimit.target.ancestors.euser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	// 1446: setrlimit.target.ancestors.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	// 1447: setrlimit.target.ancestors.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1448: setrlimit.target.ancestors.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1449: setrlimit.target.ancestors.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	// 1450: setrlimit.target.ancestors.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1451: setrlimit.target.ancestors.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1452: setrlimit.target.ancestors.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1453: setrlimit.target.ancestors.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 1454: setrlimit.target.ancestors.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	// 1455: setrlimit.target.ancestors.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	// 1456: setrlimit.target.ancestors.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	// 1457: setrlimit.target.ancestors.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 1458: setrlimit.target.ancestors.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	// 1459: setrlimit.target.ancestors.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1460: setrlimit.target.ancestors.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 1461: setrlimit.target.ancestors.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1462: setrlimit.target.ancestors.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1463: setrlimit.target.ancestors.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 1464: setrlimit.target.ancestors.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1465: setrlimit.target.ancestors.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1466: setrlimit.target.ancestors.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1467: setrlimit.target.ancestors.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1468: setrlimit.target.ancestors.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	// 1469: setrlimit.target.ancestors.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	// 1470: setrlimit.target.ancestors.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1471: setrlimit.target.ancestors.fsgid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	// 1472: setrlimit.target.ancestors.fsgroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	// 1473: setrlimit.target.ancestors.fsuid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	// 1474: setrlimit.target.ancestors.fsuser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	// 1475: setrlimit.target.ancestors.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	// 1476: setrlimit.target.ancestors.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	// 1477: setrlimit.target.ancestors.interpreter.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1478: setrlimit.target.ancestors.interpreter.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1479: setrlimit.target.ancestors.interpreter.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1480: setrlimit.target.ancestors.interpreter.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1481: setrlimit.target.ancestors.interpreter.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1482: setrlimit.target.ancestors.interpreter.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1483: setrlimit.target.ancestors.interpreter.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1484: setrlimit.target.ancestors.interpreter.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1485: setrlimit.target.ancestors.interpreter.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1486: setrlimit.target.ancestors.interpreter.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1487: setrlimit.target.ancestors.interpreter.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1488: setrlimit.target.ancestors.interpreter.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1489: setrlimit.target.ancestors.interpreter.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1490: setrlimit.target.ancestors.interpreter.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1491: setrlimit.target.ancestors.interpreter.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 1492: setrlimit.target.ancestors.interpreter.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1493: setrlimit.target.ancestors.interpreter.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1494: setrlimit.target.ancestors.interpreter.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 1495: setrlimit.target.ancestors.interpreter.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1496: setrlimit.target.ancestors.interpreter.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1497: setrlimit.target.ancestors.interpreter.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1498: setrlimit.target.ancestors.interpreter.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1499: setrlimit.target.ancestors.interpreter.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1500: setrlimit.target.ancestors.interpreter.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1501: setrlimit.target.ancestors.interpreter.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1502: setrlimit.target.ancestors.is_exec
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	// 1503: setrlimit.target.ancestors.is_kworker
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	// 1504: setrlimit.target.ancestors.is_thread
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	// 1505: setrlimit.target.ancestors.mntns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	// 1506: setrlimit.target.ancestors.netns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	// 1507: setrlimit.target.ancestors.pid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	// 1508: setrlimit.target.ancestors.ppid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	// 1509: setrlimit.target.ancestors.sid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	// 1510: setrlimit.target.ancestors.tid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	// 1511: setrlimit.target.ancestors.tty_name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	// 1512: setrlimit.target.ancestors.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	// 1513: setrlimit.target.ancestors.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	// 1514: setrlimit.target.ancestors.user_session.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	// 1515: setrlimit.target.ancestors.user_session.identity
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	// 1516: setrlimit.target.ancestors.user_session.k8s_groups
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1517: setrlimit.target.ancestors.user_session.k8s_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1518: setrlimit.target.ancestors.user_session.k8s_uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1519: setrlimit.target.ancestors.user_session.k8s_username
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1520: setrlimit.target.ancestors.user_session.session_type
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	// 1521: setrlimit.target.ancestors.user_session.ssh_auth_method
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	// 1522: setrlimit.target.ancestors.user_session.ssh_client_ip
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1523: setrlimit.target.ancestors.user_session.ssh_client_port
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	// 1524: setrlimit.target.ancestors.user_session.ssh_public_key
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1525: setrlimit.target.ancestors.user_session.ssh_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1526: setrlimit.target.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Setrlimit.Target.Process))
	},
	// 1527: setrlimit.target.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Setrlimit.Target.Process))
	},
	// 1528: setrlimit.target.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Setrlimit.Target.Process))
	},
	// 1529: setrlimit.target.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Setrlimit.Target.Process))
	},
	// 1530: setrlimit.target.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Setrlimit.Target.Process))
	},
	// 1531: setrlimit.target.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Setrlimit.Target.Process))
	},
	// 1532: setrlimit.target.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.AUID))
	},
	// 1533: setrlimit.target.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.CapEffective))
	},
	// 1534: setrlimit.target.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.CapPermitted))
	},
	// 1535: setrlimit.target.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CapsAttempted))
	},
	// 1536: setrlimit.target.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CapsUsed))
	},
	// 1537: setrlimit.target.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CGroup.CreatedAt))
	},
	// 1538: setrlimit.target.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CGroup.CGroupPathKey.Inode))
	},
	// 1539: setrlimit.target.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CGroup.CGroupPathKey.MountID))
	},
	// 1540: setrlimit.target.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Setrlimit.Target.Process.CGroup.CGroupID))
	},
	// 1541: setrlimit.target.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Setrlimit.Target.Process.CGroup))
	},
	// 1542: setrlimit.target.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Comm)
	},
	// 1543: setrlimit.target.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.ContainerContext.CreatedAt))
	},
	// 1544: setrlimit.target.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Setrlimit.Target.Process.ContainerContext.ContainerID))
	},
	// 1545: setrlimit.target.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Setrlimit.Target.Process.ContainerContext))
	},
	// 1546: setrlimit.target.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Setrlimit.Target.Process)))
	},
	// 1547: setrlimit.target.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.EGID))
	},
	// 1548: setrlimit.target.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.EGroup)
	},
	// 1549: setrlimit.target.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Setrlimit.Target.Process))
	},
	// 1550: setrlimit.target.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Setrlimit.Target.Process))
	},
	// 1551: setrlimit.target.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Setrlimit.Target.Process))
	},
	// 1552: setrlimit.target.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.EUID))
	},
	// 1553: setrlimit.target.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.EUser)
	},
	// 1554: setrlimit.target.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.CTime))
	},
	// 1555: setrlimit.target.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1556: setrlimit.target.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1557: setrlimit.target.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.GID))
	},
	// 1558: setrlimit.target.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields))
	},
	// 1559: setrlimit.target.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1560: setrlimit.target.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields))
	},
	// 1561: setrlimit.target.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 1562: setrlimit.target.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.Mode))
	},
	// 1563: setrlimit.target.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.MTime))
	},
	// 1564: setrlimit.target.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.FileEvent.MountDetached)
	},
	// 1565: setrlimit.target.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 1566: setrlimit.target.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.FileEvent.MountVisible)
	},
	// 1567: setrlimit.target.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1568: setrlimit.target.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1569: setrlimit.target.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1570: setrlimit.target.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1571: setrlimit.target.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1572: setrlimit.target.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1573: setrlimit.target.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1574: setrlimit.target.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1575: setrlimit.target.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	// 1576: setrlimit.target.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields)))
	},
	// 1577: setrlimit.target.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.UID))
	},
	// 1578: setrlimit.target.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields))
	},
	// 1579: setrlimit.target.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.FSGID))
	},
	// 1580: setrlimit.target.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.FSGroup)
	},
	// 1581: setrlimit.target.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.FSUID))
	},
	// 1582: setrlimit.target.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.FSUser)
	},
	// 1583: setrlimit.target.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.GID))
	},
	// 1584: setrlimit.target.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.Group)
	},
	// 1585: setrlimit.target.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1586: setrlimit.target.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1587: setrlimit.target.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1588: setrlimit.target.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1589: setrlimit.target.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1590: setrlimit.target.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1591: setrlimit.target.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1592: setrlimit.target.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1593: setrlimit.target.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1594: setrlimit.target.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1595: setrlimit.target.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1596: setrlimit.target.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1597: setrlimit.target.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1598: setrlimit.target.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1599: setrlimit.target.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1600: setrlimit.target.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1601: setrlimit.target.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1602: setrlimit.target.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1603: setrlimit.target.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1604: setrlimit.target.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1605: setrlimit.target.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1606: setrlimit.target.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1607: setrlimit.target.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1608: setrlimit.target.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1609: setrlimit.target.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1610: setrlimit.target.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Setrlimit.Target.Process.IsExec)
	},
	// 1611: setrlimit.target.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Setrlimit.Target.Process.PIDContext.IsKworker)
	},
	// 1612: setrlimit.target.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.Setrlimit.Target.Process))
	},
	// 1613: setrlimit.target.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.MntNS))
	},
	// 1614: setrlimit.target.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.NetNS))
	},
	// 1615: setrlimit.target.parent.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Setrlimit.Target.Parent))
	},
	// 1616: setrlimit.target.parent.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Setrlimit.Target.Parent))
	},
	// 1617: setrlimit.target.parent.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Setrlimit.Target.Parent))
	},
	// 1618: setrlimit.target.parent.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Setrlimit.Target.Parent))
	},
	// 1619: setrlimit.target.parent.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Setrlimit.Target.Parent))
	},
	// 1620: setrlimit.target.parent.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Setrlimit.Target.Parent))
	},
	// 1621: setrlimit.target.parent.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.AUID))
	},
	// 1622: setrlimit.target.parent.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.CapEffective))
	},
	// 1623: setrlimit.target.parent.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.CapPermitted))
	},
	// 1624: setrlimit.target.parent.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CapsAttempted))
	},
	// 1625: setrlimit.target.parent.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CapsUsed))
	},
	// 1626: setrlimit.target.parent.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CGroup.CreatedAt))
	},
	// 1627: setrlimit.target.parent.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CGroup.CGroupPathKey.Inode))
	},
	// 1628: setrlimit.target.parent.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CGroup.CGroupPathKey.MountID))
	},
	// 1629: setrlimit.target.parent.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Setrlimit.Target.Parent.CGroup.CGroupID))
	},
	// 1630: setrlimit.target.parent.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Setrlimit.Target.Parent.CGroup))
	},
	// 1631: setrlimit.target.parent.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Comm)
	},
	// 1632: setrlimit.target.parent.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.ContainerContext.CreatedAt))
	},
	// 1633: setrlimit.target.parent.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Setrlimit.Target.Parent.ContainerContext.ContainerID))
	},
	// 1634: setrlimit.target.parent.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Setrlimit.Target.Parent.ContainerContext))
	},
	// 1635: setrlimit.target.parent.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Setrlimit.Target.Parent)))
	},
	// 1636: setrlimit.target.parent.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.EGID))
	},
	// 1637: setrlimit.target.parent.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.EGroup)
	},
	// 1638: setrlimit.target.parent.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Setrlimit.Target.Parent))
	},
	// 1639: setrlimit.target.parent.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Setrlimit.Target.Parent))
	},
	// 1640: setrlimit.target.parent.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Setrlimit.Target.Parent))
	},
	// 1641: setrlimit.target.parent.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.EUID))
	},
	// 1642: setrlimit.target.parent.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.EUser)
	},
	// 1643: setrlimit.target.parent.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.CTime))
	},
	// 1644: setrlimit.target.parent.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1645: setrlimit.target.parent.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1646: setrlimit.target.parent.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.GID))
	},
	// 1647: setrlimit.target.parent.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Setrlimit.Target.Parent.FileEvent.FileFields))
	},
	// 1648: setrlimit.target.parent.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1649: setrlimit.target.parent.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Setrlimit.Target.Parent.FileEvent.FileFields))
	},
	// 1650: setrlimit.target.parent.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.PathKey.Inode))
	},
	// 1651: setrlimit.target.parent.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.Mode))
	},
	// 1652: setrlimit.target.parent.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.MTime))
	},
	// 1653: setrlimit.target.parent.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.FileEvent.MountDetached)
	},
	// 1654: setrlimit.target.parent.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.PathKey.MountID))
	},
	// 1655: setrlimit.target.parent.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.FileEvent.MountVisible)
	},
	// 1656: setrlimit.target.parent.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1657: setrlimit.target.parent.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1658: setrlimit.target.parent.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1659: setrlimit.target.parent.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1660: setrlimit.target.parent.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1661: setrlimit.target.parent.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1662: setrlimit.target.parent.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1663: setrlimit.target.parent.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1664: setrlimit.target.parent.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Setrlimit.Target.Parent.FileEvent))
	},
	// 1665: setrlimit.target.parent.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Setrlimit.Target.Parent.FileEvent.FileFields)))
	},
	// 1666: setrlimit.target.parent.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.FileEvent.FileFields.UID))
	},
	// 1667: setrlimit.target.parent.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Setrlimit.Target.Parent.FileEvent.FileFields))
	},
	// 1668: setrlimit.target.parent.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.FSGID))
	},
	// 1669: setrlimit.target.parent.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.FSGroup)
	},
	// 1670: setrlimit.target.parent.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.FSUID))
	},
	// 1671: setrlimit.target.parent.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.FSUser)
	},
	// 1672: setrlimit.target.parent.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.GID))
	},
	// 1673: setrlimit.target.parent.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.Group)
	},
	// 1674: setrlimit.target.parent.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1675: setrlimit.target.parent.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1676: setrlimit.target.parent.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1677: setrlimit.target.parent.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1678: setrlimit.target.parent.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 1679: setrlimit.target.parent.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1680: setrlimit.target.parent.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 1681: setrlimit.target.parent.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1682: setrlimit.target.parent.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1683: setrlimit.target.parent.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1684: setrlimit.target.parent.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1685: setrlimit.target.parent.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1686: setrlimit.target.parent.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1687: setrlimit.target.parent.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1688: setrlimit.target.parent.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1689: setrlimit.target.parent.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1690: setrlimit.target.parent.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1691: setrlimit.target.parent.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1692: setrlimit.target.parent.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1693: setrlimit.target.parent.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1694: setrlimit.target.parent.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1695: setrlimit.target.parent.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 1696: setrlimit.target.parent.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1697: setrlimit.target.parent.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1698: setrlimit.target.parent.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		if !ev.Setrlimit.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Setrlimit.Target.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 1699: setrlimit.target.parent.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.IsExec)
	},
	// 1700: setrlimit.target.parent.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.PIDContext.IsKworker)
	},
	// 1701: setrlimit.target.parent.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Setrlimit.Target.Parent))
	},
	// 1702: setrlimit.target.parent.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.MntNS))
	},
	// 1703: setrlimit.target.parent.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.NetNS))
	},
	// 1704: setrlimit.target.parent.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.Pid))
	},
	// 1705: setrlimit.target.parent.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PPid))
	},
	// 1706: setrlimit.target.parent.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.SID))
	},
	// 1707: setrlimit.target.parent.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.Tid))
	},
	// 1708: setrlimit.target.parent.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.TTYName)
	},
	// 1709: setrlimit.target.parent.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.UID))
	},
	// 1710: setrlimit.target.parent.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.User)
	},
	// 1711: setrlimit.target.parent.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Setrlimit.Target.Parent.UserSession))
	},
	// 1712: setrlimit.target.parent.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Setrlimit.Target.Parent.UserSession))
	},
	// 1713: setrlimit.target.parent.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext))
	},
	// 1714: setrlimit.target.parent.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1715: setrlimit.target.parent.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext))
	},
	// 1716: setrlimit.target.parent.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext))
	},
	// 1717: setrlimit.target.parent.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Setrlimit.Target.Parent.UserSession))
	},
	// 1718: setrlimit.target.parent.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 1719: setrlimit.target.parent.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1720: setrlimit.target.parent.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 1721: setrlimit.target.parent.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1722: setrlimit.target.parent.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1723: setrlimit.target.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.Pid))
	},
	// 1724: setrlimit.target.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PPid))
	},
	// 1725: setrlimit.target.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.SID))
	},
	// 1726: setrlimit.target.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.Tid))
	},
	// 1727: setrlimit.target.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.TTYName)
	},
	// 1728: setrlimit.target.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.UID))
	},
	// 1729: setrlimit.target.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.User)
	},
	// 1730: setrlimit.target.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Setrlimit.Target.Process.UserSession))
	},
	// 1731: setrlimit.target.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Setrlimit.Target.Process.UserSession))
	},
	// 1732: setrlimit.target.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Setrlimit.Target.Process.UserSession.K8SSessionContext))
	},
	// 1733: setrlimit.target.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1734: setrlimit.target.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Setrlimit.Target.Process.UserSession.K8SSessionContext))
	},
	// 1735: setrlimit.target.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Setrlimit.Target.Process.UserSession.K8SSessionContext))
	},
	// 1736: setrlimit.target.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Setrlimit.Target.Process.UserSession))
	},
	// 1737: setrlimit.target.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 1738: setrlimit.target.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1739: setrlimit.target.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 1740: setrlimit.target.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1741: setrlimit.target.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1742: setsockopt.filter_hash
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.filter_hash")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetSockOptFilterHash(ev, &ev.SetSockOpt))
	},
	// 1743: setsockopt.filter_instructions
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.filter_instructions")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetSockOptFilterInstructions(ev, &ev.SetSockOpt))
	},
	// 1744: setsockopt.filter_len
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.filter_len")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.FilterLen))
	},
	// 1745: setsockopt.is_filter_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.is_filter_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SetSockOpt.IsFilterTruncated)
	},
	// 1746: setsockopt.level
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.level")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.Level))
	},
	// 1747: setsockopt.optname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.optname")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.OptName))
	},
	// 1748: setsockopt.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SyscallEvent.Retval))
	},
	// 1749: setsockopt.socket_family
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.socket_family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SocketFamily))
	},
	// 1750: setsockopt.socket_protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.socket_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SocketProtocol))
	},
	// 1751: setsockopt.socket_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.socket_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SocketType))
	},
	// 1752: setsockopt.used_immediates
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.used_immediates")
		ev := ctx.Event.(*model.Event)
		return intsToVal(ev.FieldHandlers.ResolveSetSockOptUsedImmediates(ev, &ev.SetSockOpt))
	},
	// 1753: setuid.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetUID.EUID))
	},
	// 1754: setuid.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID))
	},
	// 1755: setuid.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetUID.FSUID))
	},
	// 1756: setuid.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID))
	},
	// 1757: setuid.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetUID.UID))
	},
	// 1758: setuid.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID))
	},
	// 1759: setxattr.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.CTime))
	},
	// 1760: setxattr.file.destination.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr))
	},
	// 1761: setxattr.file.destination.namespace
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.destination.namespace")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr))
	},
	// 1762: setxattr.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.SetXAttr.File))
	},
	// 1763: setxattr.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File))
	},
	// 1764: setxattr.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.GID))
	},
	// 1765: setxattr.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields))
	},
	// 1766: setxattr.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File))
	},
	// 1767: setxattr.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.SetXAttr.File.FileFields))
	},
	// 1768: setxattr.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.PathKey.Inode))
	},
	// 1769: setxattr.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.Mode))
	},
	// 1770: setxattr.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.MTime))
	},
	// 1771: setxattr.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SetXAttr.File.MountDetached)
	},
	// 1772: setxattr.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.PathKey.MountID))
	},
	// 1773: setxattr.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SetXAttr.File.MountVisible)
	},
	// 1774: setxattr.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File))
	},
	// 1775: setxattr.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.SetXAttr.File))
	},
	// 1776: setxattr.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File))
	},
	// 1777: setxattr.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.SetXAttr.File))
	},
	// 1778: setxattr.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.SetXAttr.File))
	},
	// 1779: setxattr.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.SetXAttr.File))
	},
	// 1780: setxattr.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File))
	},
	// 1781: setxattr.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File))
	},
	// 1782: setxattr.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File))
	},
	// 1783: setxattr.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.SetXAttr.File.FileFields)))
	},
	// 1784: setxattr.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.UID))
	},
	// 1785: setxattr.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields))
	},
	// 1786: setxattr.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.SyscallEvent.Retval))
	},
	// 1787: signal.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.PID))
	},
	// 1788: signal.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.SyscallEvent.Retval))
	},
	// 1789: signal.target.ancestors.args
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	// 1790: signal.target.ancestors.args_flags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	// 1791: signal.target.ancestors.args_options
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	// 1792: signal.target.ancestors.args_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	// 1793: signal.target.ancestors.argv
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	// 1794: signal.target.ancestors.argv0
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	// 1795: signal.target.ancestors.auid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	// 1796: signal.target.ancestors.cap_effective
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	// 1797: signal.target.ancestors.cap_permitted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	// 1798: signal.target.ancestors.caps_attempted
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	// 1799: signal.target.ancestors.caps_used
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	// 1800: signal.target.ancestors.cgroup.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	// 1801: signal.target.ancestors.cgroup.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	// 1802: signal.target.ancestors.cgroup.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	// 1803: signal.target.ancestors.cgroup.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	// 1804: signal.target.ancestors.cgroup.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	// 1805: signal.target.ancestors.comm
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	// 1806: signal.target.ancestors.container.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 1807: signal.target.ancestors.container.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 1808: signal.target.ancestors.container.tags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	// 1809: signal.target.ancestors.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	// 1810: signal.target.ancestors.egid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	// 1811: signal.target.ancestors.egroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	// 1812: signal.target.ancestors.envp
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	// 1813: signal.target.ancestors.envs
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	// 1814: signal.target.ancestors.envs_truncated
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	// 1815: signal.target.ancestors.euid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	// 1816: signal.target.ancestors.euser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	// 1817: signal.target.ancestors.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	// 1818: signal.target.ancestors.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1819: signal.target.ancestors.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1820: signal.target.ancestors.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	// 1821: signal.target.ancestors.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1822: signal.target.ancestors.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1823: signal.target.ancestors.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1824: signal.target.ancestors.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 1825: signal.target.ancestors.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	// 1826: signal.target.ancestors.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	// 1827: signal.target.ancestors.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	// 1828: signal.target.ancestors.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 1829: signal.target.ancestors.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	// 1830: signal.target.ancestors.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1831: signal.target.ancestors.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 1832: signal.target.ancestors.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1833: signal.target.ancestors.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1834: signal.target.ancestors.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	// 1835: signal.target.ancestors.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1836: signal.target.ancestors.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1837: signal.target.ancestors.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1838: signal.target.ancestors.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 1839: signal.target.ancestors.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	// 1840: signal.target.ancestors.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	// 1841: signal.target.ancestors.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	// 1842: signal.target.ancestors.fsgid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	// 1843: signal.target.ancestors.fsgroup
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	// 1844: signal.target.ancestors.fsuid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	// 1845: signal.target.ancestors.fsuser
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	// 1846: signal.target.ancestors.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	// 1847: signal.target.ancestors.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	// 1848: signal.target.ancestors.interpreter.file.change_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1849: signal.target.ancestors.interpreter.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1850: signal.target.ancestors.interpreter.file.filesystem
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1851: signal.target.ancestors.interpreter.file.gid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1852: signal.target.ancestors.interpreter.file.group
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1853: signal.target.ancestors.interpreter.file.hashes
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1854: signal.target.ancestors.interpreter.file.in_upper_layer
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1855: signal.target.ancestors.interpreter.file.inode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1856: signal.target.ancestors.interpreter.file.mode
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1857: signal.target.ancestors.interpreter.file.modification_time
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1858: signal.target.ancestors.interpreter.file.mount_detached
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1859: signal.target.ancestors.interpreter.file.mount_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1860: signal.target.ancestors.interpreter.file.mount_visible
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1861: signal.target.ancestors.interpreter.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1862: signal.target.ancestors.interpreter.file.package.epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 1863: signal.target.ancestors.interpreter.file.package.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1864: signal.target.ancestors.interpreter.file.package.release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1865: signal.target.ancestors.interpreter.file.package.source_epoch
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	// 1866: signal.target.ancestors.interpreter.file.package.source_release
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1867: signal.target.ancestors.interpreter.file.package.source_version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1868: signal.target.ancestors.interpreter.file.package.version
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1869: signal.target.ancestors.interpreter.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	// 1870: signal.target.ancestors.interpreter.file.rights
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1871: signal.target.ancestors.interpreter.file.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1872: signal.target.ancestors.interpreter.file.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1873: signal.target.ancestors.is_exec
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	// 1874: signal.target.ancestors.is_kworker
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	// 1875: signal.target.ancestors.is_thread
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	// 1876: signal.target.ancestors.mntns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	// 1877: signal.target.ancestors.netns
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	// 1878: signal.target.ancestors.pid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	// 1879: signal.target.ancestors.ppid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	// 1880: signal.target.ancestors.sid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	// 1881: signal.target.ancestors.tid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	// 1882: signal.target.ancestors.tty_name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	// 1883: signal.target.ancestors.uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	// 1884: signal.target.ancestors.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	// 1885: signal.target.ancestors.user_session.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	// 1886: signal.target.ancestors.user_session.identity
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	// 1887: signal.target.ancestors.user_session.k8s_groups
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1888: signal.target.ancestors.user_session.k8s_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 1889: signal.target.ancestors.user_session.k8s_uid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1890: signal.target.ancestors.user_session.k8s_username
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	// 1891: signal.target.ancestors.user_session.session_type
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	// 1892: signal.target.ancestors.user_session.ssh_auth_method
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	// 1893: signal.target.ancestors.user_session.ssh_client_ip
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 1894: signal.target.ancestors.user_session.ssh_client_port
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	// 1895: signal.target.ancestors.user_session.ssh_public_key
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 1896: signal.target.ancestors.user_session.ssh_session_id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 1897: signal.target.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process))
	},
	// 1898: signal.target.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Signal.Target.Process))
	},
	// 1899: signal.target.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Signal.Target.Process))
	},
	// 1900: signal.target.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process))
	},
	// 1901: signal.target.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process))
	},
	// 1902: signal.target.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process))
	},
	// 1903: signal.target.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.AUID))
	},
	// 1904: signal.target.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.CapEffective))
	},
	// 1905: signal.target.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.CapPermitted))
	},
	// 1906: signal.target.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CapsAttempted))
	},
	// 1907: signal.target.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CapsUsed))
	},
	// 1908: signal.target.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CGroup.CreatedAt))
	},
	// 1909: signal.target.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CGroup.CGroupPathKey.Inode))
	},
	// 1910: signal.target.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CGroup.CGroupPathKey.MountID))
	},
	// 1911: signal.target.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Signal.Target.Process.CGroup.CGroupID))
	},
	// 1912: signal.target.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Signal.Target.Process.CGroup))
	},
	// 1913: signal.target.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Comm)
	},
	// 1914: signal.target.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.ContainerContext.CreatedAt))
	},
	// 1915: signal.target.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Signal.Target.Process.ContainerContext.ContainerID))
	},
	// 1916: signal.target.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Signal.Target.Process.ContainerContext))
	},
	// 1917: signal.target.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process)))
	},
	// 1918: signal.target.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.EGID))
	},
	// 1919: signal.target.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.EGroup)
	},
	// 1920: signal.target.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process))
	},
	// 1921: signal.target.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process))
	},
	// 1922: signal.target.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process))
	},
	// 1923: signal.target.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.EUID))
	},
	// 1924: signal.target.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.EUser)
	},
	// 1925: signal.target.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.CTime))
	},
	// 1926: signal.target.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1927: signal.target.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1928: signal.target.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.GID))
	},
	// 1929: signal.target.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
	},
	// 1930: signal.target.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1931: signal.target.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
	},
	// 1932: signal.target.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.PathKey.Inode))
	},
	// 1933: signal.target.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.Mode))
	},
	// 1934: signal.target.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.MTime))
	},
	// 1935: signal.target.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.FileEvent.MountDetached)
	},
	// 1936: signal.target.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.PathKey.MountID))
	},
	// 1937: signal.target.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.FileEvent.MountVisible)
	},
	// 1938: signal.target.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1939: signal.target.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1940: signal.target.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1941: signal.target.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1942: signal.target.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1943: signal.target.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1944: signal.target.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1945: signal.target.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1946: signal.target.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent))
	},
	// 1947: signal.target.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.FileEvent.FileFields)))
	},
	// 1948: signal.target.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.UID))
	},
	// 1949: signal.target.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
	},
	// 1950: signal.target.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.FSGID))
	},
	// 1951: signal.target.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.FSGroup)
	},
	// 1952: signal.target.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.FSUID))
	},
	// 1953: signal.target.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.FSUser)
	},
	// 1954: signal.target.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.GID))
	},
	// 1955: signal.target.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.Group)
	},
	// 1956: signal.target.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 1957: signal.target.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1958: signal.target.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1959: signal.target.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 1960: signal.target.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1961: signal.target.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1962: signal.target.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1963: signal.target.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 1964: signal.target.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 1965: signal.target.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 1966: signal.target.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	// 1967: signal.target.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 1968: signal.target.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	// 1969: signal.target.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1970: signal.target.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1971: signal.target.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1972: signal.target.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1973: signal.target.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1974: signal.target.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1975: signal.target.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1976: signal.target.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1977: signal.target.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	// 1978: signal.target.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	// 1979: signal.target.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 1980: signal.target.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	// 1981: signal.target.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Signal.Target.Process.IsExec)
	},
	// 1982: signal.target.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Signal.Target.Process.PIDContext.IsKworker)
	},
	// 1983: signal.target.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.Signal.Target.Process))
	},
	// 1984: signal.target.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.MntNS))
	},
	// 1985: signal.target.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.NetNS))
	},
	// 1986: signal.target.parent.args
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent))
	},
	// 1987: signal.target.parent.args_flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Signal.Target.Parent))
	},
	// 1988: signal.target.parent.args_options
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Signal.Target.Parent))
	},
	// 1989: signal.target.parent.args_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Signal.Target.Parent))
	},
	// 1990: signal.target.parent.argv
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Signal.Target.Parent))
	},
	// 1991: signal.target.parent.argv0
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent))
	},
	// 1992: signal.target.parent.auid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.AUID))
	},
	// 1993: signal.target.parent.cap_effective
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.CapEffective))
	},
	// 1994: signal.target.parent.cap_permitted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.CapPermitted))
	},
	// 1995: signal.target.parent.caps_attempted
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CapsAttempted))
	},
	// 1996: signal.target.parent.caps_used
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CapsUsed))
	},
	// 1997: signal.target.parent.cgroup.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CGroup.CreatedAt))
	},
	// 1998: signal.target.parent.cgroup.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CGroup.CGroupPathKey.Inode))
	},
	// 1999: signal.target.parent.cgroup.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CGroup.CGroupPathKey.MountID))
	},
	// 2000: signal.target.parent.cgroup.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Signal.Target.Parent.CGroup.CGroupID))
	},
	// 2001: signal.target.parent.cgroup.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Signal.Target.Parent.CGroup))
	},
	// 2002: signal.target.parent.comm
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Comm)
	},
	// 2003: signal.target.parent.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.ContainerContext.CreatedAt))
	},
	// 2004: signal.target.parent.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Signal.Target.Parent.ContainerContext.ContainerID))
	},
	// 2005: signal.target.parent.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Signal.Target.Parent.ContainerContext))
	},
	// 2006: signal.target.parent.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent)))
	},
	// 2007: signal.target.parent.egid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.EGID))
	},
	// 2008: signal.target.parent.egroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.EGroup)
	},
	// 2009: signal.target.parent.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent))
	},
	// 2010: signal.target.parent.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent))
	},
	// 2011: signal.target.parent.envs_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Signal.Target.Parent))
	},
	// 2012: signal.target.parent.euid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.EUID))
	},
	// 2013: signal.target.parent.euser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.EUser)
	},
	// 2014: signal.target.parent.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.CTime))
	},
	// 2015: signal.target.parent.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2016: signal.target.parent.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2017: signal.target.parent.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.GID))
	},
	// 2018: signal.target.parent.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.FileEvent.FileFields))
	},
	// 2019: signal.target.parent.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2020: signal.target.parent.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.FileEvent.FileFields))
	},
	// 2021: signal.target.parent.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.PathKey.Inode))
	},
	// 2022: signal.target.parent.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.Mode))
	},
	// 2023: signal.target.parent.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.MTime))
	},
	// 2024: signal.target.parent.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.FileEvent.MountDetached)
	},
	// 2025: signal.target.parent.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.PathKey.MountID))
	},
	// 2026: signal.target.parent.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.FileEvent.MountVisible)
	},
	// 2027: signal.target.parent.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2028: signal.target.parent.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2029: signal.target.parent.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2030: signal.target.parent.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2031: signal.target.parent.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2032: signal.target.parent.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2033: signal.target.parent.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2034: signal.target.parent.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2035: signal.target.parent.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.FileEvent))
	},
	// 2036: signal.target.parent.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Parent.FileEvent.FileFields)))
	},
	// 2037: signal.target.parent.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.FileEvent.FileFields.UID))
	},
	// 2038: signal.target.parent.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.FileEvent.FileFields))
	},
	// 2039: signal.target.parent.fsgid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.FSGID))
	},
	// 2040: signal.target.parent.fsgroup
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.FSGroup)
	},
	// 2041: signal.target.parent.fsuid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.FSUID))
	},
	// 2042: signal.target.parent.fsuser
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.FSUser)
	},
	// 2043: signal.target.parent.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.GID))
	},
	// 2044: signal.target.parent.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.Group)
	},
	// 2045: signal.target.parent.interpreter.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	// 2046: signal.target.parent.interpreter.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2047: signal.target.parent.interpreter.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2048: signal.target.parent.interpreter.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.GID))
	},
	// 2049: signal.target.parent.interpreter.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 2050: signal.target.parent.interpreter.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2051: signal.target.parent.interpreter.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 2052: signal.target.parent.interpreter.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	// 2053: signal.target.parent.interpreter.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	// 2054: signal.target.parent.interpreter.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	// 2055: signal.target.parent.interpreter.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.MountDetached)
	},
	// 2056: signal.target.parent.interpreter.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	// 2057: signal.target.parent.interpreter.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.MountVisible)
	},
	// 2058: signal.target.parent.interpreter.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2059: signal.target.parent.interpreter.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2060: signal.target.parent.interpreter.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2061: signal.target.parent.interpreter.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2062: signal.target.parent.interpreter.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2063: signal.target.parent.interpreter.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2064: signal.target.parent.interpreter.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2065: signal.target.parent.interpreter.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2066: signal.target.parent.interpreter.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent))
	},
	// 2067: signal.target.parent.interpreter.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields)))
	},
	// 2068: signal.target.parent.interpreter.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields.UID))
	},
	// 2069: signal.target.parent.interpreter.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		if !ev.Signal.Target.Parent.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Parent.LinuxBinprm.FileEvent.FileFields))
	},
	// 2070: signal.target.parent.is_exec
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.IsExec)
	},
	// 2071: signal.target.parent.is_kworker
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.PIDContext.IsKworker)
	},
	// 2072: signal.target.parent.is_thread
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Signal.Target.Parent))
	},
	// 2073: signal.target.parent.mntns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.MntNS))
	},
	// 2074: signal.target.parent.netns
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.NetNS))
	},
	// 2075: signal.target.parent.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.Pid))
	},
	// 2076: signal.target.parent.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PPid))
	},
	// 2077: signal.target.parent.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.SID))
	},
	// 2078: signal.target.parent.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.Tid))
	},
	// 2079: signal.target.parent.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.TTYName)
	},
	// 2080: signal.target.parent.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.UID))
	},
	// 2081: signal.target.parent.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.User)
	},
	// 2082: signal.target.parent.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Signal.Target.Parent.UserSession))
	},
	// 2083: signal.target.parent.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Signal.Target.Parent.UserSession))
	},
	// 2084: signal.target.parent.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Parent.UserSession.K8SSessionContext))
	},
	// 2085: signal.target.parent.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 2086: signal.target.parent.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Parent.UserSession.K8SSessionContext))
	},
	// 2087: signal.target.parent.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Parent.UserSession.K8SSessionContext))
	},
	// 2088: signal.target.parent.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Signal.Target.Parent.UserSession))
	},
	// 2089: signal.target.parent.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 2090: signal.target.parent.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 2091: signal.target.parent.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 2092: signal.target.parent.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 2093: signal.target.parent.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 2094: signal.target.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.Pid))
	},
	// 2095: signal.target.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PPid))
	},
	// 2096: signal.target.sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.SID))
	},
	// 2097: signal.target.tid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.Tid))
	},
	// 2098: signal.target.tty_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.TTYName)
	},
	// 2099: signal.target.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.UID))
	},
	// 2100: signal.target.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.User)
	},
	// 2101: signal.target.user_session.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Signal.Target.Process.UserSession))
	},
	// 2102: signal.target.user_session.identity
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Signal.Target.Process.UserSession))
	},
	// 2103: signal.target.user_session.k8s_groups
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Process.UserSession.K8SSessionContext))
	},
	// 2104: signal.target.user_session.k8s_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	// 2105: signal.target.user_session.k8s_uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Process.UserSession.K8SSessionContext))
	},
	// 2106: signal.target.user_session.k8s_username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Process.UserSession.K8SSessionContext))
	},
	// 2107: signal.target.user_session.session_type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Signal.Target.Process.UserSession))
	},
	// 2108: signal.target.user_session.ssh_auth_method
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	// 2109: signal.target.user_session.ssh_client_ip
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	// 2110: signal.target.user_session.ssh_client_port
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	// 2111: signal.target.user_session.ssh_public_key
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	// 2112: signal.target.user_session.ssh_session_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	// 2113: signal.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Type))
	},
	// 2114: socket.domain
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.domain")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.Domain))
	},
	// 2115: socket.protocol
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.Protocol))
	},
	// 2116: socket.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.SyscallEvent.Retval))
	},
	// 2117: socket.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.Type))
	},
	// 2118: splice.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.CTime))
	},
	// 2119: splice.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Splice.File))
	},
	// 2120: splice.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File))
	},
	// 2121: splice.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.GID))
	},
	// 2122: splice.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields))
	},
	// 2123: splice.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File))
	},
	// 2124: splice.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Splice.File.FileFields))
	},
	// 2125: splice.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.PathKey.Inode))
	},
	// 2126: splice.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.Mode))
	},
	// 2127: splice.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.MTime))
	},
	// 2128: splice.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Splice.File.MountDetached)
	},
	// 2129: splice.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.PathKey.MountID))
	},
	// 2130: splice.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Splice.File.MountVisible)
	},
	// 2131: splice.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File))
	},
	// 2132: splice.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Splice.File))
	},
	// 2133: splice.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File))
	},
	// 2134: splice.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Splice.File))
	},
	// 2135: splice.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Splice.File))
	},
	// 2136: splice.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Splice.File))
	},
	// 2137: splice.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File))
	},
	// 2138: splice.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File))
	},
	// 2139: splice.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File))
	},
	// 2140: splice.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Splice.File.FileFields)))
	},
	// 2141: splice.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.UID))
	},
	// 2142: splice.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields))
	},
	// 2143: splice.pipe_entry_flag
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.pipe_entry_flag")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.PipeEntryFlag))
	},
	// 2144: splice.pipe_exit_flag
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.pipe_exit_flag")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.PipeExitFlag))
	},
	// 2145: splice.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.SyscallEvent.Retval))
	},
	// 2146: sysctl.action
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.action")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SysCtl.Action))
	},
	// 2147: sysctl.file_position
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.file_position")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SysCtl.FilePosition))
	},
	// 2148: sysctl.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SysCtl.Name)
	},
	// 2149: sysctl.name_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.name_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SysCtl.NameTruncated)
	},
	// 2150: sysctl.old_value
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.old_value")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SysCtl.OldValue)
	},
	// 2151: sysctl.old_value_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.old_value_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SysCtl.OldValueTruncated)
	},
	// 2152: sysctl.value
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.value")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SysCtl.Value)
	},
	// 2153: sysctl.value_truncated
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.value_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SysCtl.ValueTruncated)
	},
	// 2154: unlink.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.CTime))
	},
	// 2155: unlink.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Unlink.File))
	},
	// 2156: unlink.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File))
	},
	// 2157: unlink.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.GID))
	},
	// 2158: unlink.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields))
	},
	// 2159: unlink.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File))
	},
	// 2160: unlink.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Unlink.File.FileFields))
	},
	// 2161: unlink.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.PathKey.Inode))
	},
	// 2162: unlink.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.Mode))
	},
	// 2163: unlink.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.MTime))
	},
	// 2164: unlink.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Unlink.File.MountDetached)
	},
	// 2165: unlink.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.PathKey.MountID))
	},
	// 2166: unlink.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Unlink.File.MountVisible)
	},
	// 2167: unlink.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File))
	},
	// 2168: unlink.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Unlink.File))
	},
	// 2169: unlink.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File))
	},
	// 2170: unlink.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Unlink.File))
	},
	// 2171: unlink.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Unlink.File))
	},
	// 2172: unlink.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Unlink.File))
	},
	// 2173: unlink.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File))
	},
	// 2174: unlink.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File))
	},
	// 2175: unlink.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File))
	},
	// 2176: unlink.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Unlink.File.FileFields)))
	},
	// 2177: unlink.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.UID))
	},
	// 2178: unlink.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields))
	},
	// 2179: unlink.flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.Flags))
	},
	// 2180: unlink.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.SyscallEvent.Retval))
	},
	// 2181: unlink.syscall.dirfd
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.syscall.dirfd")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Unlink.SyscallContext)))
	},
	// 2182: unlink.syscall.flags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.syscall.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Unlink.SyscallContext)))
	},
	// 2183: unlink.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Unlink.SyscallContext))
	},
	// 2184: unload_module.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unload_module.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.UnloadModule.Name)
	},
	// 2185: unload_module.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unload_module.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.UnloadModule.SyscallEvent.Retval))
	},
	// 2186: utimes.file.change_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.CTime))
	},
	// 2187: utimes.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Utimes.File))
	},
	// 2188: utimes.file.filesystem
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File))
	},
	// 2189: utimes.file.gid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.GID))
	},
	// 2190: utimes.file.group
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields))
	},
	// 2191: utimes.file.hashes
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File))
	},
	// 2192: utimes.file.in_upper_layer
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Utimes.File.FileFields))
	},
	// 2193: utimes.file.inode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.PathKey.Inode))
	},
	// 2194: utimes.file.mode
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.Mode))
	},
	// 2195: utimes.file.modification_time
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.MTime))
	},
	// 2196: utimes.file.mount_detached
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Utimes.File.MountDetached)
	},
	// 2197: utimes.file.mount_id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.PathKey.MountID))
	},
	// 2198: utimes.file.mount_visible
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Utimes.File.MountVisible)
	},
	// 2199: utimes.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File))
	},
	// 2200: utimes.file.package.epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Utimes.File))
	},
	// 2201: utimes.file.package.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File))
	},
	// 2202: utimes.file.package.release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Utimes.File))
	},
	// 2203: utimes.file.package.source_epoch
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Utimes.File))
	},
	// 2204: utimes.file.package.source_release
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Utimes.File))
	},
	// 2205: utimes.file.package.source_version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File))
	},
	// 2206: utimes.file.package.version
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File))
	},
	// 2207: utimes.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File))
	},
	// 2208: utimes.file.rights
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Utimes.File.FileFields)))
	},
	// 2209: utimes.file.uid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.UID))
	},
	// 2210: utimes.file.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields))
	},
	// 2211: utimes.retval
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.SyscallEvent.Retval))
	},
	// 2212: utimes.syscall.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Utimes.SyscallContext))
	},
}

// celReaderIndex gives the index of each field in the layout above. It is what
// the optimization pass resolves a field name through, once per rule.
var celReaderIndex = map[string]int{
	"accept.addr.family":                                      0,
	"accept.addr.hostname":                                    1,
	"accept.addr.ip":                                          2,
	"accept.addr.is_public":                                   3,
	"accept.addr.port":                                        4,
	"accept.retval":                                           5,
	"bind.addr.family":                                        6,
	"bind.addr.ip":                                            7,
	"bind.addr.is_public":                                     8,
	"bind.addr.port":                                          9,
	"bind.protocol":                                           10,
	"bind.retval":                                             11,
	"bpf.cmd":                                                 12,
	"bpf.map.name":                                            13,
	"bpf.map.type":                                            14,
	"bpf.prog.attach_type":                                    15,
	"bpf.prog.helpers":                                        16,
	"bpf.prog.name":                                           17,
	"bpf.prog.tag":                                            18,
	"bpf.prog.type":                                           19,
	"bpf.retval":                                              20,
	"capabilities.attempted":                                  21,
	"capabilities.used":                                       22,
	"capset.cap_effective":                                    23,
	"capset.cap_permitted":                                    24,
	"cgroup_write.file.change_time":                           25,
	"cgroup_write.file.extension":                             26,
	"cgroup_write.file.filesystem":                            27,
	"cgroup_write.file.gid":                                   28,
	"cgroup_write.file.group":                                 29,
	"cgroup_write.file.hashes":                                30,
	"cgroup_write.file.in_upper_layer":                        31,
	"cgroup_write.file.inode":                                 32,
	"cgroup_write.file.mode":                                  33,
	"cgroup_write.file.modification_time":                     34,
	"cgroup_write.file.mount_detached":                        35,
	"cgroup_write.file.mount_id":                              36,
	"cgroup_write.file.mount_visible":                         37,
	"cgroup_write.file.name":                                  38,
	"cgroup_write.file.package.epoch":                         39,
	"cgroup_write.file.package.name":                          40,
	"cgroup_write.file.package.release":                       41,
	"cgroup_write.file.package.source_epoch":                  42,
	"cgroup_write.file.package.source_release":                43,
	"cgroup_write.file.package.source_version":                44,
	"cgroup_write.file.package.version":                       45,
	"cgroup_write.file.path":                                  46,
	"cgroup_write.file.rights":                                47,
	"cgroup_write.file.uid":                                   48,
	"cgroup_write.file.user":                                  49,
	"cgroup_write.pid":                                        50,
	"chdir.file.change_time":                                  51,
	"chdir.file.extension":                                    52,
	"chdir.file.filesystem":                                   53,
	"chdir.file.gid":                                          54,
	"chdir.file.group":                                        55,
	"chdir.file.hashes":                                       56,
	"chdir.file.in_upper_layer":                               57,
	"chdir.file.inode":                                        58,
	"chdir.file.mode":                                         59,
	"chdir.file.modification_time":                            60,
	"chdir.file.mount_detached":                               61,
	"chdir.file.mount_id":                                     62,
	"chdir.file.mount_visible":                                63,
	"chdir.file.name":                                         64,
	"chdir.file.package.epoch":                                65,
	"chdir.file.package.name":                                 66,
	"chdir.file.package.release":                              67,
	"chdir.file.package.source_epoch":                         68,
	"chdir.file.package.source_release":                       69,
	"chdir.file.package.source_version":                       70,
	"chdir.file.package.version":                              71,
	"chdir.file.path":                                         72,
	"chdir.file.rights":                                       73,
	"chdir.file.uid":                                          74,
	"chdir.file.user":                                         75,
	"chdir.retval":                                            76,
	"chdir.syscall.path":                                      77,
	"chmod.file.change_time":                                  78,
	"chmod.file.destination.mode":                             79,
	"chmod.file.destination.rights":                           80,
	"chmod.file.extension":                                    81,
	"chmod.file.filesystem":                                   82,
	"chmod.file.gid":                                          83,
	"chmod.file.group":                                        84,
	"chmod.file.hashes":                                       85,
	"chmod.file.in_upper_layer":                               86,
	"chmod.file.inode":                                        87,
	"chmod.file.mode":                                         88,
	"chmod.file.modification_time":                            89,
	"chmod.file.mount_detached":                               90,
	"chmod.file.mount_id":                                     91,
	"chmod.file.mount_visible":                                92,
	"chmod.file.name":                                         93,
	"chmod.file.package.epoch":                                94,
	"chmod.file.package.name":                                 95,
	"chmod.file.package.release":                              96,
	"chmod.file.package.source_epoch":                         97,
	"chmod.file.package.source_release":                       98,
	"chmod.file.package.source_version":                       99,
	"chmod.file.package.version":                              100,
	"chmod.file.path":                                         101,
	"chmod.file.rights":                                       102,
	"chmod.file.uid":                                          103,
	"chmod.file.user":                                         104,
	"chmod.retval":                                            105,
	"chmod.syscall.mode":                                      106,
	"chmod.syscall.path":                                      107,
	"chown.file.change_time":                                  108,
	"chown.file.destination.gid":                              109,
	"chown.file.destination.group":                            110,
	"chown.file.destination.uid":                              111,
	"chown.file.destination.user":                             112,
	"chown.file.extension":                                    113,
	"chown.file.filesystem":                                   114,
	"chown.file.gid":                                          115,
	"chown.file.group":                                        116,
	"chown.file.hashes":                                       117,
	"chown.file.in_upper_layer":                               118,
	"chown.file.inode":                                        119,
	"chown.file.mode":                                         120,
	"chown.file.modification_time":                            121,
	"chown.file.mount_detached":                               122,
	"chown.file.mount_id":                                     123,
	"chown.file.mount_visible":                                124,
	"chown.file.name":                                         125,
	"chown.file.package.epoch":                                126,
	"chown.file.package.name":                                 127,
	"chown.file.package.release":                              128,
	"chown.file.package.source_epoch":                         129,
	"chown.file.package.source_release":                       130,
	"chown.file.package.source_version":                       131,
	"chown.file.package.version":                              132,
	"chown.file.path":                                         133,
	"chown.file.rights":                                       134,
	"chown.file.uid":                                          135,
	"chown.file.user":                                         136,
	"chown.retval":                                            137,
	"chown.syscall.gid":                                       138,
	"chown.syscall.path":                                      139,
	"chown.syscall.uid":                                       140,
	"connect.addr.family":                                     141,
	"connect.addr.hostname":                                   142,
	"connect.addr.ip":                                         143,
	"connect.addr.is_public":                                  144,
	"connect.addr.port":                                       145,
	"connect.protocol":                                        146,
	"connect.retval":                                          147,
	"dns.id":                                                  148,
	"dns.question.class":                                      149,
	"dns.question.count":                                      150,
	"dns.question.length":                                     151,
	"dns.question.name":                                       152,
	"dns.question.type":                                       153,
	"dns.response.cnames":                                     154,
	"dns.response.code":                                       155,
	"dns.response.ips":                                        156,
	"event.async":                                             157,
	"event.hostname":                                          158,
	"event.origin":                                            159,
	"event.os":                                                160,
	"event.rule.tags":                                         161,
	"event.service":                                           162,
	"event.signature":                                         163,
	"event.source":                                            164,
	"event.timestamp":                                         165,
	"exec.args":                                               166,
	"exec.args_flags":                                         167,
	"exec.args_options":                                       168,
	"exec.args_truncated":                                     169,
	"exec.argv":                                               170,
	"exec.argv0":                                              171,
	"exec.auid":                                               172,
	"exec.cap_effective":                                      173,
	"exec.cap_permitted":                                      174,
	"exec.caps_attempted":                                     175,
	"exec.caps_used":                                          176,
	"exec.cgroup.created_at":                                  177,
	"exec.cgroup.file.inode":                                  178,
	"exec.cgroup.file.mount_id":                               179,
	"exec.cgroup.id":                                          180,
	"exec.cgroup.version":                                     181,
	"exec.comm":                                               182,
	"exec.container.created_at":                               183,
	"exec.container.id":                                       184,
	"exec.container.tags":                                     185,
	"exec.created_at":                                         186,
	"exec.egid":                                               187,
	"exec.egroup":                                             188,
	"exec.envp":                                               189,
	"exec.envs":                                               190,
	"exec.envs_truncated":                                     191,
	"exec.euid":                                               192,
	"exec.euser":                                              193,
	"exec.file.change_time":                                   194,
	"exec.file.extension":                                     195,
	"exec.file.filesystem":                                    196,
	"exec.file.gid":                                           197,
	"exec.file.group":                                         198,
	"exec.file.hashes":                                        199,
	"exec.file.in_upper_layer":                                200,
	"exec.file.inode":                                         201,
	"exec.file.metadata.abi":                                  202,
	"exec.file.metadata.architecture":                         203,
	"exec.file.metadata.compression":                          204,
	"exec.file.metadata.is_executable":                        205,
	"exec.file.metadata.is_garble_obfuscated":                 206,
	"exec.file.metadata.is_upx_packed":                        207,
	"exec.file.metadata.size":                                 208,
	"exec.file.metadata.type":                                 209,
	"exec.file.mode":                                          210,
	"exec.file.modification_time":                             211,
	"exec.file.mount_detached":                                212,
	"exec.file.mount_id":                                      213,
	"exec.file.mount_visible":                                 214,
	"exec.file.name":                                          215,
	"exec.file.package.epoch":                                 216,
	"exec.file.package.name":                                  217,
	"exec.file.package.release":                               218,
	"exec.file.package.source_epoch":                          219,
	"exec.file.package.source_release":                        220,
	"exec.file.package.source_version":                        221,
	"exec.file.package.version":                               222,
	"exec.file.path":                                          223,
	"exec.file.rights":                                        224,
	"exec.file.uid":                                           225,
	"exec.file.user":                                          226,
	"exec.fsgid":                                              227,
	"exec.fsgroup":                                            228,
	"exec.fsuid":                                              229,
	"exec.fsuser":                                             230,
	"exec.gid":                                                231,
	"exec.group":                                              232,
	"exec.interpreter.file.change_time":                       233,
	"exec.interpreter.file.extension":                         234,
	"exec.interpreter.file.filesystem":                        235,
	"exec.interpreter.file.gid":                               236,
	"exec.interpreter.file.group":                             237,
	"exec.interpreter.file.hashes":                            238,
	"exec.interpreter.file.in_upper_layer":                    239,
	"exec.interpreter.file.inode":                             240,
	"exec.interpreter.file.mode":                              241,
	"exec.interpreter.file.modification_time":                 242,
	"exec.interpreter.file.mount_detached":                    243,
	"exec.interpreter.file.mount_id":                          244,
	"exec.interpreter.file.mount_visible":                     245,
	"exec.interpreter.file.name":                              246,
	"exec.interpreter.file.package.epoch":                     247,
	"exec.interpreter.file.package.name":                      248,
	"exec.interpreter.file.package.release":                   249,
	"exec.interpreter.file.package.source_epoch":              250,
	"exec.interpreter.file.package.source_release":            251,
	"exec.interpreter.file.package.source_version":            252,
	"exec.interpreter.file.package.version":                   253,
	"exec.interpreter.file.path":                              254,
	"exec.interpreter.file.rights":                            255,
	"exec.interpreter.file.uid":                               256,
	"exec.interpreter.file.user":                              257,
	"exec.is_exec":                                            258,
	"exec.is_kworker":                                         259,
	"exec.is_thread":                                          260,
	"exec.mntns":                                              261,
	"exec.netns":                                              262,
	"exec.pid":                                                263,
	"exec.ppid":                                               264,
	"exec.sid":                                                265,
	"exec.syscall.path":                                       266,
	"exec.tid":                                                267,
	"exec.tty_name":                                           268,
	"exec.uid":                                                269,
	"exec.user":                                               270,
	"exec.user_session.id":                                    271,
	"exec.user_session.identity":                              272,
	"exec.user_session.k8s_groups":                            273,
	"exec.user_session.k8s_session_id":                        274,
	"exec.user_session.k8s_uid":                               275,
	"exec.user_session.k8s_username":                          276,
	"exec.user_session.session_type":                          277,
	"exec.user_session.ssh_auth_method":                       278,
	"exec.user_session.ssh_client_ip":                         279,
	"exec.user_session.ssh_client_port":                       280,
	"exec.user_session.ssh_public_key":                        281,
	"exec.user_session.ssh_session_id":                        282,
	"exit.args":                                               283,
	"exit.args_flags":                                         284,
	"exit.args_options":                                       285,
	"exit.args_truncated":                                     286,
	"exit.argv":                                               287,
	"exit.argv0":                                              288,
	"exit.auid":                                               289,
	"exit.cap_effective":                                      290,
	"exit.cap_permitted":                                      291,
	"exit.caps_attempted":                                     292,
	"exit.caps_used":                                          293,
	"exit.cause":                                              294,
	"exit.cgroup.created_at":                                  295,
	"exit.cgroup.file.inode":                                  296,
	"exit.cgroup.file.mount_id":                               297,
	"exit.cgroup.id":                                          298,
	"exit.cgroup.version":                                     299,
	"exit.code":                                               300,
	"exit.comm":                                               301,
	"exit.container.created_at":                               302,
	"exit.container.id":                                       303,
	"exit.container.tags":                                     304,
	"exit.created_at":                                         305,
	"exit.egid":                                               306,
	"exit.egroup":                                             307,
	"exit.envp":                                               308,
	"exit.envs":                                               309,
	"exit.envs_truncated":                                     310,
	"exit.euid":                                               311,
	"exit.euser":                                              312,
	"exit.file.change_time":                                   313,
	"exit.file.extension":                                     314,
	"exit.file.filesystem":                                    315,
	"exit.file.gid":                                           316,
	"exit.file.group":                                         317,
	"exit.file.hashes":                                        318,
	"exit.file.in_upper_layer":                                319,
	"exit.file.inode":                                         320,
	"exit.file.mode":                                          321,
	"exit.file.modification_time":                             322,
	"exit.file.mount_detached":                                323,
	"exit.file.mount_id":                                      324,
	"exit.file.mount_visible":                                 325,
	"exit.file.name":                                          326,
	"exit.file.package.epoch":                                 327,
	"exit.file.package.name":                                  328,
	"exit.file.package.release":                               329,
	"exit.file.package.source_epoch":                          330,
	"exit.file.package.source_release":                        331,
	"exit.file.package.source_version":                        332,
	"exit.file.package.version":                               333,
	"exit.file.path":                                          334,
	"exit.file.rights":                                        335,
	"exit.file.uid":                                           336,
	"exit.file.user":                                          337,
	"exit.fsgid":                                              338,
	"exit.fsgroup":                                            339,
	"exit.fsuid":                                              340,
	"exit.fsuser":                                             341,
	"exit.gid":                                                342,
	"exit.group":                                              343,
	"exit.interpreter.file.change_time":                       344,
	"exit.interpreter.file.extension":                         345,
	"exit.interpreter.file.filesystem":                        346,
	"exit.interpreter.file.gid":                               347,
	"exit.interpreter.file.group":                             348,
	"exit.interpreter.file.hashes":                            349,
	"exit.interpreter.file.in_upper_layer":                    350,
	"exit.interpreter.file.inode":                             351,
	"exit.interpreter.file.mode":                              352,
	"exit.interpreter.file.modification_time":                 353,
	"exit.interpreter.file.mount_detached":                    354,
	"exit.interpreter.file.mount_id":                          355,
	"exit.interpreter.file.mount_visible":                     356,
	"exit.interpreter.file.name":                              357,
	"exit.interpreter.file.package.epoch":                     358,
	"exit.interpreter.file.package.name":                      359,
	"exit.interpreter.file.package.release":                   360,
	"exit.interpreter.file.package.source_epoch":              361,
	"exit.interpreter.file.package.source_release":            362,
	"exit.interpreter.file.package.source_version":            363,
	"exit.interpreter.file.package.version":                   364,
	"exit.interpreter.file.path":                              365,
	"exit.interpreter.file.rights":                            366,
	"exit.interpreter.file.uid":                               367,
	"exit.interpreter.file.user":                              368,
	"exit.is_exec":                                            369,
	"exit.is_kworker":                                         370,
	"exit.is_thread":                                          371,
	"exit.mntns":                                              372,
	"exit.netns":                                              373,
	"exit.pid":                                                374,
	"exit.ppid":                                               375,
	"exit.sid":                                                376,
	"exit.tid":                                                377,
	"exit.tty_name":                                           378,
	"exit.uid":                                                379,
	"exit.user":                                               380,
	"exit.user_session.id":                                    381,
	"exit.user_session.identity":                              382,
	"exit.user_session.k8s_groups":                            383,
	"exit.user_session.k8s_session_id":                        384,
	"exit.user_session.k8s_uid":                               385,
	"exit.user_session.k8s_username":                          386,
	"exit.user_session.session_type":                          387,
	"exit.user_session.ssh_auth_method":                       388,
	"exit.user_session.ssh_client_ip":                         389,
	"exit.user_session.ssh_client_port":                       390,
	"exit.user_session.ssh_public_key":                        391,
	"exit.user_session.ssh_session_id":                        392,
	"imds.aws.is_imds_v2":                                     393,
	"imds.aws.security_credentials.type":                      394,
	"imds.cloud_provider":                                     395,
	"imds.host":                                               396,
	"imds.server":                                             397,
	"imds.type":                                               398,
	"imds.url":                                                399,
	"imds.user_agent":                                         400,
	"link.file.change_time":                                   401,
	"link.file.destination.change_time":                       402,
	"link.file.destination.extension":                         403,
	"link.file.destination.filesystem":                        404,
	"link.file.destination.gid":                               405,
	"link.file.destination.group":                             406,
	"link.file.destination.hashes":                            407,
	"link.file.destination.in_upper_layer":                    408,
	"link.file.destination.inode":                             409,
	"link.file.destination.mode":                              410,
	"link.file.destination.modification_time":                 411,
	"link.file.destination.mount_detached":                    412,
	"link.file.destination.mount_id":                          413,
	"link.file.destination.mount_visible":                     414,
	"link.file.destination.name":                              415,
	"link.file.destination.package.epoch":                     416,
	"link.file.destination.package.name":                      417,
	"link.file.destination.package.release":                   418,
	"link.file.destination.package.source_epoch":              419,
	"link.file.destination.package.source_release":            420,
	"link.file.destination.package.source_version":            421,
	"link.file.destination.package.version":                   422,
	"link.file.destination.path":                              423,
	"link.file.destination.rights":                            424,
	"link.file.destination.uid":                               425,
	"link.file.destination.user":                              426,
	"link.file.extension":                                     427,
	"link.file.filesystem":                                    428,
	"link.file.gid":                                           429,
	"link.file.group":                                         430,
	"link.file.hashes":                                        431,
	"link.file.in_upper_layer":                                432,
	"link.file.inode":                                         433,
	"link.file.mode":                                          434,
	"link.file.modification_time":                             435,
	"link.file.mount_detached":                                436,
	"link.file.mount_id":                                      437,
	"link.file.mount_visible":                                 438,
	"link.file.name":                                          439,
	"link.file.package.epoch":                                 440,
	"link.file.package.name":                                  441,
	"link.file.package.release":                               442,
	"link.file.package.source_epoch":                          443,
	"link.file.package.source_release":                        444,
	"link.file.package.source_version":                        445,
	"link.file.package.version":                               446,
	"link.file.path":                                          447,
	"link.file.rights":                                        448,
	"link.file.uid":                                           449,
	"link.file.user":                                          450,
	"link.retval":                                             451,
	"link.syscall.destination.path":                           452,
	"link.syscall.path":                                       453,
	"load_module.args":                                        454,
	"load_module.args_truncated":                              455,
	"load_module.argv":                                        456,
	"load_module.file.change_time":                            457,
	"load_module.file.extension":                              458,
	"load_module.file.filesystem":                             459,
	"load_module.file.gid":                                    460,
	"load_module.file.group":                                  461,
	"load_module.file.hashes":                                 462,
	"load_module.file.in_upper_layer":                         463,
	"load_module.file.inode":                                  464,
	"load_module.file.mode":                                   465,
	"load_module.file.modification_time":                      466,
	"load_module.file.mount_detached":                         467,
	"load_module.file.mount_id":                               468,
	"load_module.file.mount_visible":                          469,
	"load_module.file.name":                                   470,
	"load_module.file.package.epoch":                          471,
	"load_module.file.package.name":                           472,
	"load_module.file.package.release":                        473,
	"load_module.file.package.source_epoch":                   474,
	"load_module.file.package.source_release":                 475,
	"load_module.file.package.source_version":                 476,
	"load_module.file.package.version":                        477,
	"load_module.file.path":                                   478,
	"load_module.file.rights":                                 479,
	"load_module.file.uid":                                    480,
	"load_module.file.user":                                   481,
	"load_module.loaded_from_memory":                          482,
	"load_module.name":                                        483,
	"load_module.retval":                                      484,
	"mkdir.file.change_time":                                  485,
	"mkdir.file.destination.mode":                             486,
	"mkdir.file.destination.rights":                           487,
	"mkdir.file.extension":                                    488,
	"mkdir.file.filesystem":                                   489,
	"mkdir.file.gid":                                          490,
	"mkdir.file.group":                                        491,
	"mkdir.file.hashes":                                       492,
	"mkdir.file.in_upper_layer":                               493,
	"mkdir.file.inode":                                        494,
	"mkdir.file.mode":                                         495,
	"mkdir.file.modification_time":                            496,
	"mkdir.file.mount_detached":                               497,
	"mkdir.file.mount_id":                                     498,
	"mkdir.file.mount_visible":                                499,
	"mkdir.file.name":                                         500,
	"mkdir.file.package.epoch":                                501,
	"mkdir.file.package.name":                                 502,
	"mkdir.file.package.release":                              503,
	"mkdir.file.package.source_epoch":                         504,
	"mkdir.file.package.source_release":                       505,
	"mkdir.file.package.source_version":                       506,
	"mkdir.file.package.version":                              507,
	"mkdir.file.path":                                         508,
	"mkdir.file.rights":                                       509,
	"mkdir.file.uid":                                          510,
	"mkdir.file.user":                                         511,
	"mkdir.retval":                                            512,
	"mkdir.syscall.mode":                                      513,
	"mkdir.syscall.path":                                      514,
	"mmap.file.change_time":                                   515,
	"mmap.file.extension":                                     516,
	"mmap.file.filesystem":                                    517,
	"mmap.file.gid":                                           518,
	"mmap.file.group":                                         519,
	"mmap.file.hashes":                                        520,
	"mmap.file.in_upper_layer":                                521,
	"mmap.file.inode":                                         522,
	"mmap.file.mode":                                          523,
	"mmap.file.modification_time":                             524,
	"mmap.file.mount_detached":                                525,
	"mmap.file.mount_id":                                      526,
	"mmap.file.mount_visible":                                 527,
	"mmap.file.name":                                          528,
	"mmap.file.package.epoch":                                 529,
	"mmap.file.package.name":                                  530,
	"mmap.file.package.release":                               531,
	"mmap.file.package.source_epoch":                          532,
	"mmap.file.package.source_release":                        533,
	"mmap.file.package.source_version":                        534,
	"mmap.file.package.version":                               535,
	"mmap.file.path":                                          536,
	"mmap.file.rights":                                        537,
	"mmap.file.uid":                                           538,
	"mmap.file.user":                                          539,
	"mmap.flags":                                              540,
	"mmap.protection":                                         541,
	"mmap.retval":                                             542,
	"mount.detached":                                          543,
	"mount.fs_type":                                           544,
	"mount.mountpoint.path":                                   545,
	"mount.retval":                                            546,
	"mount.root.path":                                         547,
	"mount.source.path":                                       548,
	"mount.syscall.fs_type":                                   549,
	"mount.syscall.mountpoint.path":                           550,
	"mount.syscall.source.path":                               551,
	"mount.visible":                                           552,
	"mprotect.req_protection":                                 553,
	"mprotect.retval":                                         554,
	"mprotect.vm_protection":                                  555,
	"network.destination.ip":                                  556,
	"network.destination.is_public":                           557,
	"network.destination.port":                                558,
	"network.device.ifname":                                   559,
	"network.device.netns":                                    560,
	"network.l3_protocol":                                     561,
	"network.l4_protocol":                                     562,
	"network.network_direction":                               563,
	"network.size":                                            564,
	"network.source.ip":                                       565,
	"network.source.is_public":                                566,
	"network.source.port":                                     567,
	"network.type":                                            568,
	"network_flow_monitor.device.ifname":                      569,
	"network_flow_monitor.device.netns":                       570,
	"network_flow_monitor.flows.destination.ip":               571,
	"network_flow_monitor.flows.destination.is_public":        572,
	"network_flow_monitor.flows.destination.port":             573,
	"network_flow_monitor.flows.egress.data_size":             574,
	"network_flow_monitor.flows.egress.packet_count":          575,
	"network_flow_monitor.flows.ingress.data_size":            576,
	"network_flow_monitor.flows.ingress.packet_count":         577,
	"network_flow_monitor.flows.l3_protocol":                  578,
	"network_flow_monitor.flows.l4_protocol":                  579,
	"network_flow_monitor.flows.source.ip":                    580,
	"network_flow_monitor.flows.source.is_public":             581,
	"network_flow_monitor.flows.source.port":                  582,
	"ondemand.arg1.str":                                       583,
	"ondemand.arg1.uint":                                      584,
	"ondemand.arg2.str":                                       585,
	"ondemand.arg2.uint":                                      586,
	"ondemand.arg3.str":                                       587,
	"ondemand.arg3.uint":                                      588,
	"ondemand.arg4.str":                                       589,
	"ondemand.arg4.uint":                                      590,
	"ondemand.arg5.str":                                       591,
	"ondemand.arg5.uint":                                      592,
	"ondemand.arg6.str":                                       593,
	"ondemand.arg6.uint":                                      594,
	"ondemand.name":                                           595,
	"open.file.change_time":                                   596,
	"open.file.destination.mode":                              597,
	"open.file.extension":                                     598,
	"open.file.filesystem":                                    599,
	"open.file.gid":                                           600,
	"open.file.group":                                         601,
	"open.file.hashes":                                        602,
	"open.file.in_upper_layer":                                603,
	"open.file.inode":                                         604,
	"open.file.mode":                                          605,
	"open.file.modification_time":                             606,
	"open.file.mount_detached":                                607,
	"open.file.mount_id":                                      608,
	"open.file.mount_visible":                                 609,
	"open.file.name":                                          610,
	"open.file.package.epoch":                                 611,
	"open.file.package.name":                                  612,
	"open.file.package.release":                               613,
	"open.file.package.source_epoch":                          614,
	"open.file.package.source_release":                        615,
	"open.file.package.source_version":                        616,
	"open.file.package.version":                               617,
	"open.file.path":                                          618,
	"open.file.rights":                                        619,
	"open.file.uid":                                           620,
	"open.file.user":                                          621,
	"open.flags":                                              622,
	"open.retval":                                             623,
	"open.syscall.flags":                                      624,
	"open.syscall.mode":                                       625,
	"open.syscall.path":                                       626,
	"packet.destination.ip":                                   627,
	"packet.destination.is_public":                            628,
	"packet.destination.port":                                 629,
	"packet.device.ifname":                                    630,
	"packet.device.netns":                                     631,
	"packet.filter":                                           632,
	"packet.l3_protocol":                                      633,
	"packet.l4_protocol":                                      634,
	"packet.network_direction":                                635,
	"packet.size":                                             636,
	"packet.source.ip":                                        637,
	"packet.source.is_public":                                 638,
	"packet.source.port":                                      639,
	"packet.tls.version":                                      640,
	"packet.type":                                             641,
	"prctl.is_name_truncated":                                 642,
	"prctl.new_name":                                          643,
	"prctl.option":                                            644,
	"prctl.retval":                                            645,
	"process.ancestors.args":                                  646,
	"process.ancestors.args_flags":                            647,
	"process.ancestors.args_options":                          648,
	"process.ancestors.args_truncated":                        649,
	"process.ancestors.argv":                                  650,
	"process.ancestors.argv0":                                 651,
	"process.ancestors.auid":                                  652,
	"process.ancestors.cap_effective":                         653,
	"process.ancestors.cap_permitted":                         654,
	"process.ancestors.caps_attempted":                        655,
	"process.ancestors.caps_used":                             656,
	"process.ancestors.cgroup.created_at":                     657,
	"process.ancestors.cgroup.file.inode":                     658,
	"process.ancestors.cgroup.file.mount_id":                  659,
	"process.ancestors.cgroup.id":                             660,
	"process.ancestors.cgroup.version":                        661,
	"process.ancestors.comm":                                  662,
	"process.ancestors.container.created_at":                  663,
	"process.ancestors.container.id":                          664,
	"process.ancestors.container.tags":                        665,
	"process.ancestors.created_at":                            666,
	"process.ancestors.egid":                                  667,
	"process.ancestors.egroup":                                668,
	"process.ancestors.envp":                                  669,
	"process.ancestors.envs":                                  670,
	"process.ancestors.envs_truncated":                        671,
	"process.ancestors.euid":                                  672,
	"process.ancestors.euser":                                 673,
	"process.ancestors.file.change_time":                      674,
	"process.ancestors.file.extension":                        675,
	"process.ancestors.file.filesystem":                       676,
	"process.ancestors.file.gid":                              677,
	"process.ancestors.file.group":                            678,
	"process.ancestors.file.hashes":                           679,
	"process.ancestors.file.in_upper_layer":                   680,
	"process.ancestors.file.inode":                            681,
	"process.ancestors.file.mode":                             682,
	"process.ancestors.file.modification_time":                683,
	"process.ancestors.file.mount_detached":                   684,
	"process.ancestors.file.mount_id":                         685,
	"process.ancestors.file.mount_visible":                    686,
	"process.ancestors.file.name":                             687,
	"process.ancestors.file.package.epoch":                    688,
	"process.ancestors.file.package.name":                     689,
	"process.ancestors.file.package.release":                  690,
	"process.ancestors.file.package.source_epoch":             691,
	"process.ancestors.file.package.source_release":           692,
	"process.ancestors.file.package.source_version":           693,
	"process.ancestors.file.package.version":                  694,
	"process.ancestors.file.path":                             695,
	"process.ancestors.file.rights":                           696,
	"process.ancestors.file.uid":                              697,
	"process.ancestors.file.user":                             698,
	"process.ancestors.fsgid":                                 699,
	"process.ancestors.fsgroup":                               700,
	"process.ancestors.fsuid":                                 701,
	"process.ancestors.fsuser":                                702,
	"process.ancestors.gid":                                   703,
	"process.ancestors.group":                                 704,
	"process.ancestors.interpreter.file.change_time":          705,
	"process.ancestors.interpreter.file.extension":            706,
	"process.ancestors.interpreter.file.filesystem":           707,
	"process.ancestors.interpreter.file.gid":                  708,
	"process.ancestors.interpreter.file.group":                709,
	"process.ancestors.interpreter.file.hashes":               710,
	"process.ancestors.interpreter.file.in_upper_layer":       711,
	"process.ancestors.interpreter.file.inode":                712,
	"process.ancestors.interpreter.file.mode":                 713,
	"process.ancestors.interpreter.file.modification_time":    714,
	"process.ancestors.interpreter.file.mount_detached":       715,
	"process.ancestors.interpreter.file.mount_id":             716,
	"process.ancestors.interpreter.file.mount_visible":        717,
	"process.ancestors.interpreter.file.name":                 718,
	"process.ancestors.interpreter.file.package.epoch":        719,
	"process.ancestors.interpreter.file.package.name":         720,
	"process.ancestors.interpreter.file.package.release":      721,
	"process.ancestors.interpreter.file.package.source_epoch": 722,
	"process.ancestors.interpreter.file.package.source_release": 723,
	"process.ancestors.interpreter.file.package.source_version": 724,
	"process.ancestors.interpreter.file.package.version":        725,
	"process.ancestors.interpreter.file.path":                   726,
	"process.ancestors.interpreter.file.rights":                 727,
	"process.ancestors.interpreter.file.uid":                    728,
	"process.ancestors.interpreter.file.user":                   729,
	"process.ancestors.is_exec":                                 730,
	"process.ancestors.is_kworker":                              731,
	"process.ancestors.is_thread":                               732,
	"process.ancestors.mntns":                                   733,
	"process.ancestors.netns":                                   734,
	"process.ancestors.pid":                                     735,
	"process.ancestors.ppid":                                    736,
	"process.ancestors.sid":                                     737,
	"process.ancestors.tid":                                     738,
	"process.ancestors.tty_name":                                739,
	"process.ancestors.uid":                                     740,
	"process.ancestors.user":                                    741,
	"process.ancestors.user_session.id":                         742,
	"process.ancestors.user_session.identity":                   743,
	"process.ancestors.user_session.k8s_groups":                 744,
	"process.ancestors.user_session.k8s_session_id":             745,
	"process.ancestors.user_session.k8s_uid":                    746,
	"process.ancestors.user_session.k8s_username":               747,
	"process.ancestors.user_session.session_type":               748,
	"process.ancestors.user_session.ssh_auth_method":            749,
	"process.ancestors.user_session.ssh_client_ip":              750,
	"process.ancestors.user_session.ssh_client_port":            751,
	"process.ancestors.user_session.ssh_public_key":             752,
	"process.ancestors.user_session.ssh_session_id":             753,
	"process.args":                                                       754,
	"process.args_flags":                                                 755,
	"process.args_options":                                               756,
	"process.args_truncated":                                             757,
	"process.argv":                                                       758,
	"process.argv0":                                                      759,
	"process.auid":                                                       760,
	"process.cap_effective":                                              761,
	"process.cap_permitted":                                              762,
	"process.caps_attempted":                                             763,
	"process.caps_used":                                                  764,
	"process.cgroup.created_at":                                          765,
	"process.cgroup.file.inode":                                          766,
	"process.cgroup.file.mount_id":                                       767,
	"process.cgroup.id":                                                  768,
	"process.cgroup.version":                                             769,
	"process.comm":                                                       770,
	"process.container.created_at":                                       771,
	"process.container.id":                                               772,
	"process.container.tags":                                             773,
	"process.created_at":                                                 774,
	"process.egid":                                                       775,
	"process.egroup":                                                     776,
	"process.envp":                                                       777,
	"process.envs":                                                       778,
	"process.envs_truncated":                                             779,
	"process.euid":                                                       780,
	"process.euser":                                                      781,
	"process.file.change_time":                                           782,
	"process.file.extension":                                             783,
	"process.file.filesystem":                                            784,
	"process.file.gid":                                                   785,
	"process.file.group":                                                 786,
	"process.file.hashes":                                                787,
	"process.file.in_upper_layer":                                        788,
	"process.file.inode":                                                 789,
	"process.file.mode":                                                  790,
	"process.file.modification_time":                                     791,
	"process.file.mount_detached":                                        792,
	"process.file.mount_id":                                              793,
	"process.file.mount_visible":                                         794,
	"process.file.name":                                                  795,
	"process.file.package.epoch":                                         796,
	"process.file.package.name":                                          797,
	"process.file.package.release":                                       798,
	"process.file.package.source_epoch":                                  799,
	"process.file.package.source_release":                                800,
	"process.file.package.source_version":                                801,
	"process.file.package.version":                                       802,
	"process.file.path":                                                  803,
	"process.file.rights":                                                804,
	"process.file.uid":                                                   805,
	"process.file.user":                                                  806,
	"process.fsgid":                                                      807,
	"process.fsgroup":                                                    808,
	"process.fsuid":                                                      809,
	"process.fsuser":                                                     810,
	"process.gid":                                                        811,
	"process.group":                                                      812,
	"process.interpreter.file.change_time":                               813,
	"process.interpreter.file.extension":                                 814,
	"process.interpreter.file.filesystem":                                815,
	"process.interpreter.file.gid":                                       816,
	"process.interpreter.file.group":                                     817,
	"process.interpreter.file.hashes":                                    818,
	"process.interpreter.file.in_upper_layer":                            819,
	"process.interpreter.file.inode":                                     820,
	"process.interpreter.file.mode":                                      821,
	"process.interpreter.file.modification_time":                         822,
	"process.interpreter.file.mount_detached":                            823,
	"process.interpreter.file.mount_id":                                  824,
	"process.interpreter.file.mount_visible":                             825,
	"process.interpreter.file.name":                                      826,
	"process.interpreter.file.package.epoch":                             827,
	"process.interpreter.file.package.name":                              828,
	"process.interpreter.file.package.release":                           829,
	"process.interpreter.file.package.source_epoch":                      830,
	"process.interpreter.file.package.source_release":                    831,
	"process.interpreter.file.package.source_version":                    832,
	"process.interpreter.file.package.version":                           833,
	"process.interpreter.file.path":                                      834,
	"process.interpreter.file.rights":                                    835,
	"process.interpreter.file.uid":                                       836,
	"process.interpreter.file.user":                                      837,
	"process.is_exec":                                                    838,
	"process.is_kworker":                                                 839,
	"process.is_thread":                                                  840,
	"process.mntns":                                                      841,
	"process.netns":                                                      842,
	"process.parent.args":                                                843,
	"process.parent.args_flags":                                          844,
	"process.parent.args_options":                                        845,
	"process.parent.args_truncated":                                      846,
	"process.parent.argv":                                                847,
	"process.parent.argv0":                                               848,
	"process.parent.auid":                                                849,
	"process.parent.cap_effective":                                       850,
	"process.parent.cap_permitted":                                       851,
	"process.parent.caps_attempted":                                      852,
	"process.parent.caps_used":                                           853,
	"process.parent.cgroup.created_at":                                   854,
	"process.parent.cgroup.file.inode":                                   855,
	"process.parent.cgroup.file.mount_id":                                856,
	"process.parent.cgroup.id":                                           857,
	"process.parent.cgroup.version":                                      858,
	"process.parent.comm":                                                859,
	"process.parent.container.created_at":                                860,
	"process.parent.container.id":                                        861,
	"process.parent.container.tags":                                      862,
	"process.parent.created_at":                                          863,
	"process.parent.egid":                                                864,
	"process.parent.egroup":                                              865,
	"process.parent.envp":                                                866,
	"process.parent.envs":                                                867,
	"process.parent.envs_truncated":                                      868,
	"process.parent.euid":                                                869,
	"process.parent.euser":                                               870,
	"process.parent.file.change_time":                                    871,
	"process.parent.file.extension":                                      872,
	"process.parent.file.filesystem":                                     873,
	"process.parent.file.gid":                                            874,
	"process.parent.file.group":                                          875,
	"process.parent.file.hashes":                                         876,
	"process.parent.file.in_upper_layer":                                 877,
	"process.parent.file.inode":                                          878,
	"process.parent.file.mode":                                           879,
	"process.parent.file.modification_time":                              880,
	"process.parent.file.mount_detached":                                 881,
	"process.parent.file.mount_id":                                       882,
	"process.parent.file.mount_visible":                                  883,
	"process.parent.file.name":                                           884,
	"process.parent.file.package.epoch":                                  885,
	"process.parent.file.package.name":                                   886,
	"process.parent.file.package.release":                                887,
	"process.parent.file.package.source_epoch":                           888,
	"process.parent.file.package.source_release":                         889,
	"process.parent.file.package.source_version":                         890,
	"process.parent.file.package.version":                                891,
	"process.parent.file.path":                                           892,
	"process.parent.file.rights":                                         893,
	"process.parent.file.uid":                                            894,
	"process.parent.file.user":                                           895,
	"process.parent.fsgid":                                               896,
	"process.parent.fsgroup":                                             897,
	"process.parent.fsuid":                                               898,
	"process.parent.fsuser":                                              899,
	"process.parent.gid":                                                 900,
	"process.parent.group":                                               901,
	"process.parent.interpreter.file.change_time":                        902,
	"process.parent.interpreter.file.extension":                          903,
	"process.parent.interpreter.file.filesystem":                         904,
	"process.parent.interpreter.file.gid":                                905,
	"process.parent.interpreter.file.group":                              906,
	"process.parent.interpreter.file.hashes":                             907,
	"process.parent.interpreter.file.in_upper_layer":                     908,
	"process.parent.interpreter.file.inode":                              909,
	"process.parent.interpreter.file.mode":                               910,
	"process.parent.interpreter.file.modification_time":                  911,
	"process.parent.interpreter.file.mount_detached":                     912,
	"process.parent.interpreter.file.mount_id":                           913,
	"process.parent.interpreter.file.mount_visible":                      914,
	"process.parent.interpreter.file.name":                               915,
	"process.parent.interpreter.file.package.epoch":                      916,
	"process.parent.interpreter.file.package.name":                       917,
	"process.parent.interpreter.file.package.release":                    918,
	"process.parent.interpreter.file.package.source_epoch":               919,
	"process.parent.interpreter.file.package.source_release":             920,
	"process.parent.interpreter.file.package.source_version":             921,
	"process.parent.interpreter.file.package.version":                    922,
	"process.parent.interpreter.file.path":                               923,
	"process.parent.interpreter.file.rights":                             924,
	"process.parent.interpreter.file.uid":                                925,
	"process.parent.interpreter.file.user":                               926,
	"process.parent.is_exec":                                             927,
	"process.parent.is_kworker":                                          928,
	"process.parent.is_thread":                                           929,
	"process.parent.mntns":                                               930,
	"process.parent.netns":                                               931,
	"process.parent.pid":                                                 932,
	"process.parent.ppid":                                                933,
	"process.parent.sid":                                                 934,
	"process.parent.tid":                                                 935,
	"process.parent.tty_name":                                            936,
	"process.parent.uid":                                                 937,
	"process.parent.user":                                                938,
	"process.parent.user_session.id":                                     939,
	"process.parent.user_session.identity":                               940,
	"process.parent.user_session.k8s_groups":                             941,
	"process.parent.user_session.k8s_session_id":                         942,
	"process.parent.user_session.k8s_uid":                                943,
	"process.parent.user_session.k8s_username":                           944,
	"process.parent.user_session.session_type":                           945,
	"process.parent.user_session.ssh_auth_method":                        946,
	"process.parent.user_session.ssh_client_ip":                          947,
	"process.parent.user_session.ssh_client_port":                        948,
	"process.parent.user_session.ssh_public_key":                         949,
	"process.parent.user_session.ssh_session_id":                         950,
	"process.pid":                                                        951,
	"process.ppid":                                                       952,
	"process.sid":                                                        953,
	"process.tid":                                                        954,
	"process.tty_name":                                                   955,
	"process.uid":                                                        956,
	"process.user":                                                       957,
	"process.user_session.id":                                            958,
	"process.user_session.identity":                                      959,
	"process.user_session.k8s_groups":                                    960,
	"process.user_session.k8s_session_id":                                961,
	"process.user_session.k8s_uid":                                       962,
	"process.user_session.k8s_username":                                  963,
	"process.user_session.session_type":                                  964,
	"process.user_session.ssh_auth_method":                               965,
	"process.user_session.ssh_client_ip":                                 966,
	"process.user_session.ssh_client_port":                               967,
	"process.user_session.ssh_public_key":                                968,
	"process.user_session.ssh_session_id":                                969,
	"ptrace.request":                                                     970,
	"ptrace.retval":                                                      971,
	"ptrace.tracee.ancestors.args":                                       972,
	"ptrace.tracee.ancestors.args_flags":                                 973,
	"ptrace.tracee.ancestors.args_options":                               974,
	"ptrace.tracee.ancestors.args_truncated":                             975,
	"ptrace.tracee.ancestors.argv":                                       976,
	"ptrace.tracee.ancestors.argv0":                                      977,
	"ptrace.tracee.ancestors.auid":                                       978,
	"ptrace.tracee.ancestors.cap_effective":                              979,
	"ptrace.tracee.ancestors.cap_permitted":                              980,
	"ptrace.tracee.ancestors.caps_attempted":                             981,
	"ptrace.tracee.ancestors.caps_used":                                  982,
	"ptrace.tracee.ancestors.cgroup.created_at":                          983,
	"ptrace.tracee.ancestors.cgroup.file.inode":                          984,
	"ptrace.tracee.ancestors.cgroup.file.mount_id":                       985,
	"ptrace.tracee.ancestors.cgroup.id":                                  986,
	"ptrace.tracee.ancestors.cgroup.version":                             987,
	"ptrace.tracee.ancestors.comm":                                       988,
	"ptrace.tracee.ancestors.container.created_at":                       989,
	"ptrace.tracee.ancestors.container.id":                               990,
	"ptrace.tracee.ancestors.container.tags":                             991,
	"ptrace.tracee.ancestors.created_at":                                 992,
	"ptrace.tracee.ancestors.egid":                                       993,
	"ptrace.tracee.ancestors.egroup":                                     994,
	"ptrace.tracee.ancestors.envp":                                       995,
	"ptrace.tracee.ancestors.envs":                                       996,
	"ptrace.tracee.ancestors.envs_truncated":                             997,
	"ptrace.tracee.ancestors.euid":                                       998,
	"ptrace.tracee.ancestors.euser":                                      999,
	"ptrace.tracee.ancestors.file.change_time":                           1000,
	"ptrace.tracee.ancestors.file.extension":                             1001,
	"ptrace.tracee.ancestors.file.filesystem":                            1002,
	"ptrace.tracee.ancestors.file.gid":                                   1003,
	"ptrace.tracee.ancestors.file.group":                                 1004,
	"ptrace.tracee.ancestors.file.hashes":                                1005,
	"ptrace.tracee.ancestors.file.in_upper_layer":                        1006,
	"ptrace.tracee.ancestors.file.inode":                                 1007,
	"ptrace.tracee.ancestors.file.mode":                                  1008,
	"ptrace.tracee.ancestors.file.modification_time":                     1009,
	"ptrace.tracee.ancestors.file.mount_detached":                        1010,
	"ptrace.tracee.ancestors.file.mount_id":                              1011,
	"ptrace.tracee.ancestors.file.mount_visible":                         1012,
	"ptrace.tracee.ancestors.file.name":                                  1013,
	"ptrace.tracee.ancestors.file.package.epoch":                         1014,
	"ptrace.tracee.ancestors.file.package.name":                          1015,
	"ptrace.tracee.ancestors.file.package.release":                       1016,
	"ptrace.tracee.ancestors.file.package.source_epoch":                  1017,
	"ptrace.tracee.ancestors.file.package.source_release":                1018,
	"ptrace.tracee.ancestors.file.package.source_version":                1019,
	"ptrace.tracee.ancestors.file.package.version":                       1020,
	"ptrace.tracee.ancestors.file.path":                                  1021,
	"ptrace.tracee.ancestors.file.rights":                                1022,
	"ptrace.tracee.ancestors.file.uid":                                   1023,
	"ptrace.tracee.ancestors.file.user":                                  1024,
	"ptrace.tracee.ancestors.fsgid":                                      1025,
	"ptrace.tracee.ancestors.fsgroup":                                    1026,
	"ptrace.tracee.ancestors.fsuid":                                      1027,
	"ptrace.tracee.ancestors.fsuser":                                     1028,
	"ptrace.tracee.ancestors.gid":                                        1029,
	"ptrace.tracee.ancestors.group":                                      1030,
	"ptrace.tracee.ancestors.interpreter.file.change_time":               1031,
	"ptrace.tracee.ancestors.interpreter.file.extension":                 1032,
	"ptrace.tracee.ancestors.interpreter.file.filesystem":                1033,
	"ptrace.tracee.ancestors.interpreter.file.gid":                       1034,
	"ptrace.tracee.ancestors.interpreter.file.group":                     1035,
	"ptrace.tracee.ancestors.interpreter.file.hashes":                    1036,
	"ptrace.tracee.ancestors.interpreter.file.in_upper_layer":            1037,
	"ptrace.tracee.ancestors.interpreter.file.inode":                     1038,
	"ptrace.tracee.ancestors.interpreter.file.mode":                      1039,
	"ptrace.tracee.ancestors.interpreter.file.modification_time":         1040,
	"ptrace.tracee.ancestors.interpreter.file.mount_detached":            1041,
	"ptrace.tracee.ancestors.interpreter.file.mount_id":                  1042,
	"ptrace.tracee.ancestors.interpreter.file.mount_visible":             1043,
	"ptrace.tracee.ancestors.interpreter.file.name":                      1044,
	"ptrace.tracee.ancestors.interpreter.file.package.epoch":             1045,
	"ptrace.tracee.ancestors.interpreter.file.package.name":              1046,
	"ptrace.tracee.ancestors.interpreter.file.package.release":           1047,
	"ptrace.tracee.ancestors.interpreter.file.package.source_epoch":      1048,
	"ptrace.tracee.ancestors.interpreter.file.package.source_release":    1049,
	"ptrace.tracee.ancestors.interpreter.file.package.source_version":    1050,
	"ptrace.tracee.ancestors.interpreter.file.package.version":           1051,
	"ptrace.tracee.ancestors.interpreter.file.path":                      1052,
	"ptrace.tracee.ancestors.interpreter.file.rights":                    1053,
	"ptrace.tracee.ancestors.interpreter.file.uid":                       1054,
	"ptrace.tracee.ancestors.interpreter.file.user":                      1055,
	"ptrace.tracee.ancestors.is_exec":                                    1056,
	"ptrace.tracee.ancestors.is_kworker":                                 1057,
	"ptrace.tracee.ancestors.is_thread":                                  1058,
	"ptrace.tracee.ancestors.mntns":                                      1059,
	"ptrace.tracee.ancestors.netns":                                      1060,
	"ptrace.tracee.ancestors.pid":                                        1061,
	"ptrace.tracee.ancestors.ppid":                                       1062,
	"ptrace.tracee.ancestors.sid":                                        1063,
	"ptrace.tracee.ancestors.tid":                                        1064,
	"ptrace.tracee.ancestors.tty_name":                                   1065,
	"ptrace.tracee.ancestors.uid":                                        1066,
	"ptrace.tracee.ancestors.user":                                       1067,
	"ptrace.tracee.ancestors.user_session.id":                            1068,
	"ptrace.tracee.ancestors.user_session.identity":                      1069,
	"ptrace.tracee.ancestors.user_session.k8s_groups":                    1070,
	"ptrace.tracee.ancestors.user_session.k8s_session_id":                1071,
	"ptrace.tracee.ancestors.user_session.k8s_uid":                       1072,
	"ptrace.tracee.ancestors.user_session.k8s_username":                  1073,
	"ptrace.tracee.ancestors.user_session.session_type":                  1074,
	"ptrace.tracee.ancestors.user_session.ssh_auth_method":               1075,
	"ptrace.tracee.ancestors.user_session.ssh_client_ip":                 1076,
	"ptrace.tracee.ancestors.user_session.ssh_client_port":               1077,
	"ptrace.tracee.ancestors.user_session.ssh_public_key":                1078,
	"ptrace.tracee.ancestors.user_session.ssh_session_id":                1079,
	"ptrace.tracee.args":                                                 1080,
	"ptrace.tracee.args_flags":                                           1081,
	"ptrace.tracee.args_options":                                         1082,
	"ptrace.tracee.args_truncated":                                       1083,
	"ptrace.tracee.argv":                                                 1084,
	"ptrace.tracee.argv0":                                                1085,
	"ptrace.tracee.auid":                                                 1086,
	"ptrace.tracee.cap_effective":                                        1087,
	"ptrace.tracee.cap_permitted":                                        1088,
	"ptrace.tracee.caps_attempted":                                       1089,
	"ptrace.tracee.caps_used":                                            1090,
	"ptrace.tracee.cgroup.created_at":                                    1091,
	"ptrace.tracee.cgroup.file.inode":                                    1092,
	"ptrace.tracee.cgroup.file.mount_id":                                 1093,
	"ptrace.tracee.cgroup.id":                                            1094,
	"ptrace.tracee.cgroup.version":                                       1095,
	"ptrace.tracee.comm":                                                 1096,
	"ptrace.tracee.container.created_at":                                 1097,
	"ptrace.tracee.container.id":                                         1098,
	"ptrace.tracee.container.tags":                                       1099,
	"ptrace.tracee.created_at":                                           1100,
	"ptrace.tracee.egid":                                                 1101,
	"ptrace.tracee.egroup":                                               1102,
	"ptrace.tracee.envp":                                                 1103,
	"ptrace.tracee.envs":                                                 1104,
	"ptrace.tracee.envs_truncated":                                       1105,
	"ptrace.tracee.euid":                                                 1106,
	"ptrace.tracee.euser":                                                1107,
	"ptrace.tracee.file.change_time":                                     1108,
	"ptrace.tracee.file.extension":                                       1109,
	"ptrace.tracee.file.filesystem":                                      1110,
	"ptrace.tracee.file.gid":                                             1111,
	"ptrace.tracee.file.group":                                           1112,
	"ptrace.tracee.file.hashes":                                          1113,
	"ptrace.tracee.file.in_upper_layer":                                  1114,
	"ptrace.tracee.file.inode":                                           1115,
	"ptrace.tracee.file.mode":                                            1116,
	"ptrace.tracee.file.modification_time":                               1117,
	"ptrace.tracee.file.mount_detached":                                  1118,
	"ptrace.tracee.file.mount_id":                                        1119,
	"ptrace.tracee.file.mount_visible":                                   1120,
	"ptrace.tracee.file.name":                                            1121,
	"ptrace.tracee.file.package.epoch":                                   1122,
	"ptrace.tracee.file.package.name":                                    1123,
	"ptrace.tracee.file.package.release":                                 1124,
	"ptrace.tracee.file.package.source_epoch":                            1125,
	"ptrace.tracee.file.package.source_release":                          1126,
	"ptrace.tracee.file.package.source_version":                          1127,
	"ptrace.tracee.file.package.version":                                 1128,
	"ptrace.tracee.file.path":                                            1129,
	"ptrace.tracee.file.rights":                                          1130,
	"ptrace.tracee.file.uid":                                             1131,
	"ptrace.tracee.file.user":                                            1132,
	"ptrace.tracee.fsgid":                                                1133,
	"ptrace.tracee.fsgroup":                                              1134,
	"ptrace.tracee.fsuid":                                                1135,
	"ptrace.tracee.fsuser":                                               1136,
	"ptrace.tracee.gid":                                                  1137,
	"ptrace.tracee.group":                                                1138,
	"ptrace.tracee.interpreter.file.change_time":                         1139,
	"ptrace.tracee.interpreter.file.extension":                           1140,
	"ptrace.tracee.interpreter.file.filesystem":                          1141,
	"ptrace.tracee.interpreter.file.gid":                                 1142,
	"ptrace.tracee.interpreter.file.group":                               1143,
	"ptrace.tracee.interpreter.file.hashes":                              1144,
	"ptrace.tracee.interpreter.file.in_upper_layer":                      1145,
	"ptrace.tracee.interpreter.file.inode":                               1146,
	"ptrace.tracee.interpreter.file.mode":                                1147,
	"ptrace.tracee.interpreter.file.modification_time":                   1148,
	"ptrace.tracee.interpreter.file.mount_detached":                      1149,
	"ptrace.tracee.interpreter.file.mount_id":                            1150,
	"ptrace.tracee.interpreter.file.mount_visible":                       1151,
	"ptrace.tracee.interpreter.file.name":                                1152,
	"ptrace.tracee.interpreter.file.package.epoch":                       1153,
	"ptrace.tracee.interpreter.file.package.name":                        1154,
	"ptrace.tracee.interpreter.file.package.release":                     1155,
	"ptrace.tracee.interpreter.file.package.source_epoch":                1156,
	"ptrace.tracee.interpreter.file.package.source_release":              1157,
	"ptrace.tracee.interpreter.file.package.source_version":              1158,
	"ptrace.tracee.interpreter.file.package.version":                     1159,
	"ptrace.tracee.interpreter.file.path":                                1160,
	"ptrace.tracee.interpreter.file.rights":                              1161,
	"ptrace.tracee.interpreter.file.uid":                                 1162,
	"ptrace.tracee.interpreter.file.user":                                1163,
	"ptrace.tracee.is_exec":                                              1164,
	"ptrace.tracee.is_kworker":                                           1165,
	"ptrace.tracee.is_thread":                                            1166,
	"ptrace.tracee.mntns":                                                1167,
	"ptrace.tracee.netns":                                                1168,
	"ptrace.tracee.parent.args":                                          1169,
	"ptrace.tracee.parent.args_flags":                                    1170,
	"ptrace.tracee.parent.args_options":                                  1171,
	"ptrace.tracee.parent.args_truncated":                                1172,
	"ptrace.tracee.parent.argv":                                          1173,
	"ptrace.tracee.parent.argv0":                                         1174,
	"ptrace.tracee.parent.auid":                                          1175,
	"ptrace.tracee.parent.cap_effective":                                 1176,
	"ptrace.tracee.parent.cap_permitted":                                 1177,
	"ptrace.tracee.parent.caps_attempted":                                1178,
	"ptrace.tracee.parent.caps_used":                                     1179,
	"ptrace.tracee.parent.cgroup.created_at":                             1180,
	"ptrace.tracee.parent.cgroup.file.inode":                             1181,
	"ptrace.tracee.parent.cgroup.file.mount_id":                          1182,
	"ptrace.tracee.parent.cgroup.id":                                     1183,
	"ptrace.tracee.parent.cgroup.version":                                1184,
	"ptrace.tracee.parent.comm":                                          1185,
	"ptrace.tracee.parent.container.created_at":                          1186,
	"ptrace.tracee.parent.container.id":                                  1187,
	"ptrace.tracee.parent.container.tags":                                1188,
	"ptrace.tracee.parent.created_at":                                    1189,
	"ptrace.tracee.parent.egid":                                          1190,
	"ptrace.tracee.parent.egroup":                                        1191,
	"ptrace.tracee.parent.envp":                                          1192,
	"ptrace.tracee.parent.envs":                                          1193,
	"ptrace.tracee.parent.envs_truncated":                                1194,
	"ptrace.tracee.parent.euid":                                          1195,
	"ptrace.tracee.parent.euser":                                         1196,
	"ptrace.tracee.parent.file.change_time":                              1197,
	"ptrace.tracee.parent.file.extension":                                1198,
	"ptrace.tracee.parent.file.filesystem":                               1199,
	"ptrace.tracee.parent.file.gid":                                      1200,
	"ptrace.tracee.parent.file.group":                                    1201,
	"ptrace.tracee.parent.file.hashes":                                   1202,
	"ptrace.tracee.parent.file.in_upper_layer":                           1203,
	"ptrace.tracee.parent.file.inode":                                    1204,
	"ptrace.tracee.parent.file.mode":                                     1205,
	"ptrace.tracee.parent.file.modification_time":                        1206,
	"ptrace.tracee.parent.file.mount_detached":                           1207,
	"ptrace.tracee.parent.file.mount_id":                                 1208,
	"ptrace.tracee.parent.file.mount_visible":                            1209,
	"ptrace.tracee.parent.file.name":                                     1210,
	"ptrace.tracee.parent.file.package.epoch":                            1211,
	"ptrace.tracee.parent.file.package.name":                             1212,
	"ptrace.tracee.parent.file.package.release":                          1213,
	"ptrace.tracee.parent.file.package.source_epoch":                     1214,
	"ptrace.tracee.parent.file.package.source_release":                   1215,
	"ptrace.tracee.parent.file.package.source_version":                   1216,
	"ptrace.tracee.parent.file.package.version":                          1217,
	"ptrace.tracee.parent.file.path":                                     1218,
	"ptrace.tracee.parent.file.rights":                                   1219,
	"ptrace.tracee.parent.file.uid":                                      1220,
	"ptrace.tracee.parent.file.user":                                     1221,
	"ptrace.tracee.parent.fsgid":                                         1222,
	"ptrace.tracee.parent.fsgroup":                                       1223,
	"ptrace.tracee.parent.fsuid":                                         1224,
	"ptrace.tracee.parent.fsuser":                                        1225,
	"ptrace.tracee.parent.gid":                                           1226,
	"ptrace.tracee.parent.group":                                         1227,
	"ptrace.tracee.parent.interpreter.file.change_time":                  1228,
	"ptrace.tracee.parent.interpreter.file.extension":                    1229,
	"ptrace.tracee.parent.interpreter.file.filesystem":                   1230,
	"ptrace.tracee.parent.interpreter.file.gid":                          1231,
	"ptrace.tracee.parent.interpreter.file.group":                        1232,
	"ptrace.tracee.parent.interpreter.file.hashes":                       1233,
	"ptrace.tracee.parent.interpreter.file.in_upper_layer":               1234,
	"ptrace.tracee.parent.interpreter.file.inode":                        1235,
	"ptrace.tracee.parent.interpreter.file.mode":                         1236,
	"ptrace.tracee.parent.interpreter.file.modification_time":            1237,
	"ptrace.tracee.parent.interpreter.file.mount_detached":               1238,
	"ptrace.tracee.parent.interpreter.file.mount_id":                     1239,
	"ptrace.tracee.parent.interpreter.file.mount_visible":                1240,
	"ptrace.tracee.parent.interpreter.file.name":                         1241,
	"ptrace.tracee.parent.interpreter.file.package.epoch":                1242,
	"ptrace.tracee.parent.interpreter.file.package.name":                 1243,
	"ptrace.tracee.parent.interpreter.file.package.release":              1244,
	"ptrace.tracee.parent.interpreter.file.package.source_epoch":         1245,
	"ptrace.tracee.parent.interpreter.file.package.source_release":       1246,
	"ptrace.tracee.parent.interpreter.file.package.source_version":       1247,
	"ptrace.tracee.parent.interpreter.file.package.version":              1248,
	"ptrace.tracee.parent.interpreter.file.path":                         1249,
	"ptrace.tracee.parent.interpreter.file.rights":                       1250,
	"ptrace.tracee.parent.interpreter.file.uid":                          1251,
	"ptrace.tracee.parent.interpreter.file.user":                         1252,
	"ptrace.tracee.parent.is_exec":                                       1253,
	"ptrace.tracee.parent.is_kworker":                                    1254,
	"ptrace.tracee.parent.is_thread":                                     1255,
	"ptrace.tracee.parent.mntns":                                         1256,
	"ptrace.tracee.parent.netns":                                         1257,
	"ptrace.tracee.parent.pid":                                           1258,
	"ptrace.tracee.parent.ppid":                                          1259,
	"ptrace.tracee.parent.sid":                                           1260,
	"ptrace.tracee.parent.tid":                                           1261,
	"ptrace.tracee.parent.tty_name":                                      1262,
	"ptrace.tracee.parent.uid":                                           1263,
	"ptrace.tracee.parent.user":                                          1264,
	"ptrace.tracee.parent.user_session.id":                               1265,
	"ptrace.tracee.parent.user_session.identity":                         1266,
	"ptrace.tracee.parent.user_session.k8s_groups":                       1267,
	"ptrace.tracee.parent.user_session.k8s_session_id":                   1268,
	"ptrace.tracee.parent.user_session.k8s_uid":                          1269,
	"ptrace.tracee.parent.user_session.k8s_username":                     1270,
	"ptrace.tracee.parent.user_session.session_type":                     1271,
	"ptrace.tracee.parent.user_session.ssh_auth_method":                  1272,
	"ptrace.tracee.parent.user_session.ssh_client_ip":                    1273,
	"ptrace.tracee.parent.user_session.ssh_client_port":                  1274,
	"ptrace.tracee.parent.user_session.ssh_public_key":                   1275,
	"ptrace.tracee.parent.user_session.ssh_session_id":                   1276,
	"ptrace.tracee.pid":                                                  1277,
	"ptrace.tracee.ppid":                                                 1278,
	"ptrace.tracee.sid":                                                  1279,
	"ptrace.tracee.tid":                                                  1280,
	"ptrace.tracee.tty_name":                                             1281,
	"ptrace.tracee.uid":                                                  1282,
	"ptrace.tracee.user":                                                 1283,
	"ptrace.tracee.user_session.id":                                      1284,
	"ptrace.tracee.user_session.identity":                                1285,
	"ptrace.tracee.user_session.k8s_groups":                              1286,
	"ptrace.tracee.user_session.k8s_session_id":                          1287,
	"ptrace.tracee.user_session.k8s_uid":                                 1288,
	"ptrace.tracee.user_session.k8s_username":                            1289,
	"ptrace.tracee.user_session.session_type":                            1290,
	"ptrace.tracee.user_session.ssh_auth_method":                         1291,
	"ptrace.tracee.user_session.ssh_client_ip":                           1292,
	"ptrace.tracee.user_session.ssh_client_port":                         1293,
	"ptrace.tracee.user_session.ssh_public_key":                          1294,
	"ptrace.tracee.user_session.ssh_session_id":                          1295,
	"removexattr.file.change_time":                                       1296,
	"removexattr.file.destination.name":                                  1297,
	"removexattr.file.destination.namespace":                             1298,
	"removexattr.file.extension":                                         1299,
	"removexattr.file.filesystem":                                        1300,
	"removexattr.file.gid":                                               1301,
	"removexattr.file.group":                                             1302,
	"removexattr.file.hashes":                                            1303,
	"removexattr.file.in_upper_layer":                                    1304,
	"removexattr.file.inode":                                             1305,
	"removexattr.file.mode":                                              1306,
	"removexattr.file.modification_time":                                 1307,
	"removexattr.file.mount_detached":                                    1308,
	"removexattr.file.mount_id":                                          1309,
	"removexattr.file.mount_visible":                                     1310,
	"removexattr.file.name":                                              1311,
	"removexattr.file.package.epoch":                                     1312,
	"removexattr.file.package.name":                                      1313,
	"removexattr.file.package.release":                                   1314,
	"removexattr.file.package.source_epoch":                              1315,
	"removexattr.file.package.source_release":                            1316,
	"removexattr.file.package.source_version":                            1317,
	"removexattr.file.package.version":                                   1318,
	"removexattr.file.path":                                              1319,
	"removexattr.file.rights":                                            1320,
	"removexattr.file.uid":                                               1321,
	"removexattr.file.user":                                              1322,
	"removexattr.retval":                                                 1323,
	"rename.file.change_time":                                            1324,
	"rename.file.destination.change_time":                                1325,
	"rename.file.destination.extension":                                  1326,
	"rename.file.destination.filesystem":                                 1327,
	"rename.file.destination.gid":                                        1328,
	"rename.file.destination.group":                                      1329,
	"rename.file.destination.hashes":                                     1330,
	"rename.file.destination.in_upper_layer":                             1331,
	"rename.file.destination.inode":                                      1332,
	"rename.file.destination.mode":                                       1333,
	"rename.file.destination.modification_time":                          1334,
	"rename.file.destination.mount_detached":                             1335,
	"rename.file.destination.mount_id":                                   1336,
	"rename.file.destination.mount_visible":                              1337,
	"rename.file.destination.name":                                       1338,
	"rename.file.destination.package.epoch":                              1339,
	"rename.file.destination.package.name":                               1340,
	"rename.file.destination.package.release":                            1341,
	"rename.file.destination.package.source_epoch":                       1342,
	"rename.file.destination.package.source_release":                     1343,
	"rename.file.destination.package.source_version":                     1344,
	"rename.file.destination.package.version":                            1345,
	"rename.file.destination.path":                                       1346,
	"rename.file.destination.rights":                                     1347,
	"rename.file.destination.uid":                                        1348,
	"rename.file.destination.user":                                       1349,
	"rename.file.extension":                                              1350,
	"rename.file.filesystem":                                             1351,
	"rename.file.gid":                                                    1352,
	"rename.file.group":                                                  1353,
	"rename.file.hashes":                                                 1354,
	"rename.file.in_upper_layer":                                         1355,
	"rename.file.inode":                                                  1356,
	"rename.file.mode":                                                   1357,
	"rename.file.modification_time":                                      1358,
	"rename.file.mount_detached":                                         1359,
	"rename.file.mount_id":                                               1360,
	"rename.file.mount_visible":                                          1361,
	"rename.file.name":                                                   1362,
	"rename.file.package.epoch":                                          1363,
	"rename.file.package.name":                                           1364,
	"rename.file.package.release":                                        1365,
	"rename.file.package.source_epoch":                                   1366,
	"rename.file.package.source_release":                                 1367,
	"rename.file.package.source_version":                                 1368,
	"rename.file.package.version":                                        1369,
	"rename.file.path":                                                   1370,
	"rename.file.rights":                                                 1371,
	"rename.file.uid":                                                    1372,
	"rename.file.user":                                                   1373,
	"rename.retval":                                                      1374,
	"rename.syscall.destination.path":                                    1375,
	"rename.syscall.path":                                                1376,
	"rmdir.file.change_time":                                             1377,
	"rmdir.file.extension":                                               1378,
	"rmdir.file.filesystem":                                              1379,
	"rmdir.file.gid":                                                     1380,
	"rmdir.file.group":                                                   1381,
	"rmdir.file.hashes":                                                  1382,
	"rmdir.file.in_upper_layer":                                          1383,
	"rmdir.file.inode":                                                   1384,
	"rmdir.file.mode":                                                    1385,
	"rmdir.file.modification_time":                                       1386,
	"rmdir.file.mount_detached":                                          1387,
	"rmdir.file.mount_id":                                                1388,
	"rmdir.file.mount_visible":                                           1389,
	"rmdir.file.name":                                                    1390,
	"rmdir.file.package.epoch":                                           1391,
	"rmdir.file.package.name":                                            1392,
	"rmdir.file.package.release":                                         1393,
	"rmdir.file.package.source_epoch":                                    1394,
	"rmdir.file.package.source_release":                                  1395,
	"rmdir.file.package.source_version":                                  1396,
	"rmdir.file.package.version":                                         1397,
	"rmdir.file.path":                                                    1398,
	"rmdir.file.rights":                                                  1399,
	"rmdir.file.uid":                                                     1400,
	"rmdir.file.user":                                                    1401,
	"rmdir.retval":                                                       1402,
	"rmdir.syscall.path":                                                 1403,
	"selinux.bool.name":                                                  1404,
	"selinux.bool.state":                                                 1405,
	"selinux.bool_commit.state":                                          1406,
	"selinux.enforce.status":                                             1407,
	"setgid.egid":                                                        1408,
	"setgid.egroup":                                                      1409,
	"setgid.fsgid":                                                       1410,
	"setgid.fsgroup":                                                     1411,
	"setgid.gid":                                                         1412,
	"setgid.group":                                                       1413,
	"setrlimit.resource":                                                 1414,
	"setrlimit.retval":                                                   1415,
	"setrlimit.rlim_cur":                                                 1416,
	"setrlimit.rlim_max":                                                 1417,
	"setrlimit.target.ancestors.args":                                    1418,
	"setrlimit.target.ancestors.args_flags":                              1419,
	"setrlimit.target.ancestors.args_options":                            1420,
	"setrlimit.target.ancestors.args_truncated":                          1421,
	"setrlimit.target.ancestors.argv":                                    1422,
	"setrlimit.target.ancestors.argv0":                                   1423,
	"setrlimit.target.ancestors.auid":                                    1424,
	"setrlimit.target.ancestors.cap_effective":                           1425,
	"setrlimit.target.ancestors.cap_permitted":                           1426,
	"setrlimit.target.ancestors.caps_attempted":                          1427,
	"setrlimit.target.ancestors.caps_used":                               1428,
	"setrlimit.target.ancestors.cgroup.created_at":                       1429,
	"setrlimit.target.ancestors.cgroup.file.inode":                       1430,
	"setrlimit.target.ancestors.cgroup.file.mount_id":                    1431,
	"setrlimit.target.ancestors.cgroup.id":                               1432,
	"setrlimit.target.ancestors.cgroup.version":                          1433,
	"setrlimit.target.ancestors.comm":                                    1434,
	"setrlimit.target.ancestors.container.created_at":                    1435,
	"setrlimit.target.ancestors.container.id":                            1436,
	"setrlimit.target.ancestors.container.tags":                          1437,
	"setrlimit.target.ancestors.created_at":                              1438,
	"setrlimit.target.ancestors.egid":                                    1439,
	"setrlimit.target.ancestors.egroup":                                  1440,
	"setrlimit.target.ancestors.envp":                                    1441,
	"setrlimit.target.ancestors.envs":                                    1442,
	"setrlimit.target.ancestors.envs_truncated":                          1443,
	"setrlimit.target.ancestors.euid":                                    1444,
	"setrlimit.target.ancestors.euser":                                   1445,
	"setrlimit.target.ancestors.file.change_time":                        1446,
	"setrlimit.target.ancestors.file.extension":                          1447,
	"setrlimit.target.ancestors.file.filesystem":                         1448,
	"setrlimit.target.ancestors.file.gid":                                1449,
	"setrlimit.target.ancestors.file.group":                              1450,
	"setrlimit.target.ancestors.file.hashes":                             1451,
	"setrlimit.target.ancestors.file.in_upper_layer":                     1452,
	"setrlimit.target.ancestors.file.inode":                              1453,
	"setrlimit.target.ancestors.file.mode":                               1454,
	"setrlimit.target.ancestors.file.modification_time":                  1455,
	"setrlimit.target.ancestors.file.mount_detached":                     1456,
	"setrlimit.target.ancestors.file.mount_id":                           1457,
	"setrlimit.target.ancestors.file.mount_visible":                      1458,
	"setrlimit.target.ancestors.file.name":                               1459,
	"setrlimit.target.ancestors.file.package.epoch":                      1460,
	"setrlimit.target.ancestors.file.package.name":                       1461,
	"setrlimit.target.ancestors.file.package.release":                    1462,
	"setrlimit.target.ancestors.file.package.source_epoch":               1463,
	"setrlimit.target.ancestors.file.package.source_release":             1464,
	"setrlimit.target.ancestors.file.package.source_version":             1465,
	"setrlimit.target.ancestors.file.package.version":                    1466,
	"setrlimit.target.ancestors.file.path":                               1467,
	"setrlimit.target.ancestors.file.rights":                             1468,
	"setrlimit.target.ancestors.file.uid":                                1469,
	"setrlimit.target.ancestors.file.user":                               1470,
	"setrlimit.target.ancestors.fsgid":                                   1471,
	"setrlimit.target.ancestors.fsgroup":                                 1472,
	"setrlimit.target.ancestors.fsuid":                                   1473,
	"setrlimit.target.ancestors.fsuser":                                  1474,
	"setrlimit.target.ancestors.gid":                                     1475,
	"setrlimit.target.ancestors.group":                                   1476,
	"setrlimit.target.ancestors.interpreter.file.change_time":            1477,
	"setrlimit.target.ancestors.interpreter.file.extension":              1478,
	"setrlimit.target.ancestors.interpreter.file.filesystem":             1479,
	"setrlimit.target.ancestors.interpreter.file.gid":                    1480,
	"setrlimit.target.ancestors.interpreter.file.group":                  1481,
	"setrlimit.target.ancestors.interpreter.file.hashes":                 1482,
	"setrlimit.target.ancestors.interpreter.file.in_upper_layer":         1483,
	"setrlimit.target.ancestors.interpreter.file.inode":                  1484,
	"setrlimit.target.ancestors.interpreter.file.mode":                   1485,
	"setrlimit.target.ancestors.interpreter.file.modification_time":      1486,
	"setrlimit.target.ancestors.interpreter.file.mount_detached":         1487,
	"setrlimit.target.ancestors.interpreter.file.mount_id":               1488,
	"setrlimit.target.ancestors.interpreter.file.mount_visible":          1489,
	"setrlimit.target.ancestors.interpreter.file.name":                   1490,
	"setrlimit.target.ancestors.interpreter.file.package.epoch":          1491,
	"setrlimit.target.ancestors.interpreter.file.package.name":           1492,
	"setrlimit.target.ancestors.interpreter.file.package.release":        1493,
	"setrlimit.target.ancestors.interpreter.file.package.source_epoch":   1494,
	"setrlimit.target.ancestors.interpreter.file.package.source_release": 1495,
	"setrlimit.target.ancestors.interpreter.file.package.source_version": 1496,
	"setrlimit.target.ancestors.interpreter.file.package.version":        1497,
	"setrlimit.target.ancestors.interpreter.file.path":                   1498,
	"setrlimit.target.ancestors.interpreter.file.rights":                 1499,
	"setrlimit.target.ancestors.interpreter.file.uid":                    1500,
	"setrlimit.target.ancestors.interpreter.file.user":                   1501,
	"setrlimit.target.ancestors.is_exec":                                 1502,
	"setrlimit.target.ancestors.is_kworker":                              1503,
	"setrlimit.target.ancestors.is_thread":                               1504,
	"setrlimit.target.ancestors.mntns":                                   1505,
	"setrlimit.target.ancestors.netns":                                   1506,
	"setrlimit.target.ancestors.pid":                                     1507,
	"setrlimit.target.ancestors.ppid":                                    1508,
	"setrlimit.target.ancestors.sid":                                     1509,
	"setrlimit.target.ancestors.tid":                                     1510,
	"setrlimit.target.ancestors.tty_name":                                1511,
	"setrlimit.target.ancestors.uid":                                     1512,
	"setrlimit.target.ancestors.user":                                    1513,
	"setrlimit.target.ancestors.user_session.id":                         1514,
	"setrlimit.target.ancestors.user_session.identity":                   1515,
	"setrlimit.target.ancestors.user_session.k8s_groups":                 1516,
	"setrlimit.target.ancestors.user_session.k8s_session_id":             1517,
	"setrlimit.target.ancestors.user_session.k8s_uid":                    1518,
	"setrlimit.target.ancestors.user_session.k8s_username":               1519,
	"setrlimit.target.ancestors.user_session.session_type":               1520,
	"setrlimit.target.ancestors.user_session.ssh_auth_method":            1521,
	"setrlimit.target.ancestors.user_session.ssh_client_ip":              1522,
	"setrlimit.target.ancestors.user_session.ssh_client_port":            1523,
	"setrlimit.target.ancestors.user_session.ssh_public_key":             1524,
	"setrlimit.target.ancestors.user_session.ssh_session_id":             1525,
	"setrlimit.target.args":                                              1526,
	"setrlimit.target.args_flags":                                        1527,
	"setrlimit.target.args_options":                                      1528,
	"setrlimit.target.args_truncated":                                    1529,
	"setrlimit.target.argv":                                              1530,
	"setrlimit.target.argv0":                                             1531,
	"setrlimit.target.auid":                                              1532,
	"setrlimit.target.cap_effective":                                     1533,
	"setrlimit.target.cap_permitted":                                     1534,
	"setrlimit.target.caps_attempted":                                    1535,
	"setrlimit.target.caps_used":                                         1536,
	"setrlimit.target.cgroup.created_at":                                 1537,
	"setrlimit.target.cgroup.file.inode":                                 1538,
	"setrlimit.target.cgroup.file.mount_id":                              1539,
	"setrlimit.target.cgroup.id":                                         1540,
	"setrlimit.target.cgroup.version":                                    1541,
	"setrlimit.target.comm":                                              1542,
	"setrlimit.target.container.created_at":                              1543,
	"setrlimit.target.container.id":                                      1544,
	"setrlimit.target.container.tags":                                    1545,
	"setrlimit.target.created_at":                                        1546,
	"setrlimit.target.egid":                                              1547,
	"setrlimit.target.egroup":                                            1548,
	"setrlimit.target.envp":                                              1549,
	"setrlimit.target.envs":                                              1550,
	"setrlimit.target.envs_truncated":                                    1551,
	"setrlimit.target.euid":                                              1552,
	"setrlimit.target.euser":                                             1553,
	"setrlimit.target.file.change_time":                                  1554,
	"setrlimit.target.file.extension":                                    1555,
	"setrlimit.target.file.filesystem":                                   1556,
	"setrlimit.target.file.gid":                                          1557,
	"setrlimit.target.file.group":                                        1558,
	"setrlimit.target.file.hashes":                                       1559,
	"setrlimit.target.file.in_upper_layer":                               1560,
	"setrlimit.target.file.inode":                                        1561,
	"setrlimit.target.file.mode":                                         1562,
	"setrlimit.target.file.modification_time":                            1563,
	"setrlimit.target.file.mount_detached":                               1564,
	"setrlimit.target.file.mount_id":                                     1565,
	"setrlimit.target.file.mount_visible":                                1566,
	"setrlimit.target.file.name":                                         1567,
	"setrlimit.target.file.package.epoch":                                1568,
	"setrlimit.target.file.package.name":                                 1569,
	"setrlimit.target.file.package.release":                              1570,
	"setrlimit.target.file.package.source_epoch":                         1571,
	"setrlimit.target.file.package.source_release":                       1572,
	"setrlimit.target.file.package.source_version":                       1573,
	"setrlimit.target.file.package.version":                              1574,
	"setrlimit.target.file.path":                                         1575,
	"setrlimit.target.file.rights":                                       1576,
	"setrlimit.target.file.uid":                                          1577,
	"setrlimit.target.file.user":                                         1578,
	"setrlimit.target.fsgid":                                             1579,
	"setrlimit.target.fsgroup":                                           1580,
	"setrlimit.target.fsuid":                                             1581,
	"setrlimit.target.fsuser":                                            1582,
	"setrlimit.target.gid":                                               1583,
	"setrlimit.target.group":                                             1584,
	"setrlimit.target.interpreter.file.change_time":                      1585,
	"setrlimit.target.interpreter.file.extension":                        1586,
	"setrlimit.target.interpreter.file.filesystem":                       1587,
	"setrlimit.target.interpreter.file.gid":                              1588,
	"setrlimit.target.interpreter.file.group":                            1589,
	"setrlimit.target.interpreter.file.hashes":                           1590,
	"setrlimit.target.interpreter.file.in_upper_layer":                   1591,
	"setrlimit.target.interpreter.file.inode":                            1592,
	"setrlimit.target.interpreter.file.mode":                             1593,
	"setrlimit.target.interpreter.file.modification_time":                1594,
	"setrlimit.target.interpreter.file.mount_detached":                   1595,
	"setrlimit.target.interpreter.file.mount_id":                         1596,
	"setrlimit.target.interpreter.file.mount_visible":                    1597,
	"setrlimit.target.interpreter.file.name":                             1598,
	"setrlimit.target.interpreter.file.package.epoch":                    1599,
	"setrlimit.target.interpreter.file.package.name":                     1600,
	"setrlimit.target.interpreter.file.package.release":                  1601,
	"setrlimit.target.interpreter.file.package.source_epoch":             1602,
	"setrlimit.target.interpreter.file.package.source_release":           1603,
	"setrlimit.target.interpreter.file.package.source_version":           1604,
	"setrlimit.target.interpreter.file.package.version":                  1605,
	"setrlimit.target.interpreter.file.path":                             1606,
	"setrlimit.target.interpreter.file.rights":                           1607,
	"setrlimit.target.interpreter.file.uid":                              1608,
	"setrlimit.target.interpreter.file.user":                             1609,
	"setrlimit.target.is_exec":                                           1610,
	"setrlimit.target.is_kworker":                                        1611,
	"setrlimit.target.is_thread":                                         1612,
	"setrlimit.target.mntns":                                             1613,
	"setrlimit.target.netns":                                             1614,
	"setrlimit.target.parent.args":                                       1615,
	"setrlimit.target.parent.args_flags":                                 1616,
	"setrlimit.target.parent.args_options":                               1617,
	"setrlimit.target.parent.args_truncated":                             1618,
	"setrlimit.target.parent.argv":                                       1619,
	"setrlimit.target.parent.argv0":                                      1620,
	"setrlimit.target.parent.auid":                                       1621,
	"setrlimit.target.parent.cap_effective":                              1622,
	"setrlimit.target.parent.cap_permitted":                              1623,
	"setrlimit.target.parent.caps_attempted":                             1624,
	"setrlimit.target.parent.caps_used":                                  1625,
	"setrlimit.target.parent.cgroup.created_at":                          1626,
	"setrlimit.target.parent.cgroup.file.inode":                          1627,
	"setrlimit.target.parent.cgroup.file.mount_id":                       1628,
	"setrlimit.target.parent.cgroup.id":                                  1629,
	"setrlimit.target.parent.cgroup.version":                             1630,
	"setrlimit.target.parent.comm":                                       1631,
	"setrlimit.target.parent.container.created_at":                       1632,
	"setrlimit.target.parent.container.id":                               1633,
	"setrlimit.target.parent.container.tags":                             1634,
	"setrlimit.target.parent.created_at":                                 1635,
	"setrlimit.target.parent.egid":                                       1636,
	"setrlimit.target.parent.egroup":                                     1637,
	"setrlimit.target.parent.envp":                                       1638,
	"setrlimit.target.parent.envs":                                       1639,
	"setrlimit.target.parent.envs_truncated":                             1640,
	"setrlimit.target.parent.euid":                                       1641,
	"setrlimit.target.parent.euser":                                      1642,
	"setrlimit.target.parent.file.change_time":                           1643,
	"setrlimit.target.parent.file.extension":                             1644,
	"setrlimit.target.parent.file.filesystem":                            1645,
	"setrlimit.target.parent.file.gid":                                   1646,
	"setrlimit.target.parent.file.group":                                 1647,
	"setrlimit.target.parent.file.hashes":                                1648,
	"setrlimit.target.parent.file.in_upper_layer":                        1649,
	"setrlimit.target.parent.file.inode":                                 1650,
	"setrlimit.target.parent.file.mode":                                  1651,
	"setrlimit.target.parent.file.modification_time":                     1652,
	"setrlimit.target.parent.file.mount_detached":                        1653,
	"setrlimit.target.parent.file.mount_id":                              1654,
	"setrlimit.target.parent.file.mount_visible":                         1655,
	"setrlimit.target.parent.file.name":                                  1656,
	"setrlimit.target.parent.file.package.epoch":                         1657,
	"setrlimit.target.parent.file.package.name":                          1658,
	"setrlimit.target.parent.file.package.release":                       1659,
	"setrlimit.target.parent.file.package.source_epoch":                  1660,
	"setrlimit.target.parent.file.package.source_release":                1661,
	"setrlimit.target.parent.file.package.source_version":                1662,
	"setrlimit.target.parent.file.package.version":                       1663,
	"setrlimit.target.parent.file.path":                                  1664,
	"setrlimit.target.parent.file.rights":                                1665,
	"setrlimit.target.parent.file.uid":                                   1666,
	"setrlimit.target.parent.file.user":                                  1667,
	"setrlimit.target.parent.fsgid":                                      1668,
	"setrlimit.target.parent.fsgroup":                                    1669,
	"setrlimit.target.parent.fsuid":                                      1670,
	"setrlimit.target.parent.fsuser":                                     1671,
	"setrlimit.target.parent.gid":                                        1672,
	"setrlimit.target.parent.group":                                      1673,
	"setrlimit.target.parent.interpreter.file.change_time":               1674,
	"setrlimit.target.parent.interpreter.file.extension":                 1675,
	"setrlimit.target.parent.interpreter.file.filesystem":                1676,
	"setrlimit.target.parent.interpreter.file.gid":                       1677,
	"setrlimit.target.parent.interpreter.file.group":                     1678,
	"setrlimit.target.parent.interpreter.file.hashes":                    1679,
	"setrlimit.target.parent.interpreter.file.in_upper_layer":            1680,
	"setrlimit.target.parent.interpreter.file.inode":                     1681,
	"setrlimit.target.parent.interpreter.file.mode":                      1682,
	"setrlimit.target.parent.interpreter.file.modification_time":         1683,
	"setrlimit.target.parent.interpreter.file.mount_detached":            1684,
	"setrlimit.target.parent.interpreter.file.mount_id":                  1685,
	"setrlimit.target.parent.interpreter.file.mount_visible":             1686,
	"setrlimit.target.parent.interpreter.file.name":                      1687,
	"setrlimit.target.parent.interpreter.file.package.epoch":             1688,
	"setrlimit.target.parent.interpreter.file.package.name":              1689,
	"setrlimit.target.parent.interpreter.file.package.release":           1690,
	"setrlimit.target.parent.interpreter.file.package.source_epoch":      1691,
	"setrlimit.target.parent.interpreter.file.package.source_release":    1692,
	"setrlimit.target.parent.interpreter.file.package.source_version":    1693,
	"setrlimit.target.parent.interpreter.file.package.version":           1694,
	"setrlimit.target.parent.interpreter.file.path":                      1695,
	"setrlimit.target.parent.interpreter.file.rights":                    1696,
	"setrlimit.target.parent.interpreter.file.uid":                       1697,
	"setrlimit.target.parent.interpreter.file.user":                      1698,
	"setrlimit.target.parent.is_exec":                                    1699,
	"setrlimit.target.parent.is_kworker":                                 1700,
	"setrlimit.target.parent.is_thread":                                  1701,
	"setrlimit.target.parent.mntns":                                      1702,
	"setrlimit.target.parent.netns":                                      1703,
	"setrlimit.target.parent.pid":                                        1704,
	"setrlimit.target.parent.ppid":                                       1705,
	"setrlimit.target.parent.sid":                                        1706,
	"setrlimit.target.parent.tid":                                        1707,
	"setrlimit.target.parent.tty_name":                                   1708,
	"setrlimit.target.parent.uid":                                        1709,
	"setrlimit.target.parent.user":                                       1710,
	"setrlimit.target.parent.user_session.id":                            1711,
	"setrlimit.target.parent.user_session.identity":                      1712,
	"setrlimit.target.parent.user_session.k8s_groups":                    1713,
	"setrlimit.target.parent.user_session.k8s_session_id":                1714,
	"setrlimit.target.parent.user_session.k8s_uid":                       1715,
	"setrlimit.target.parent.user_session.k8s_username":                  1716,
	"setrlimit.target.parent.user_session.session_type":                  1717,
	"setrlimit.target.parent.user_session.ssh_auth_method":               1718,
	"setrlimit.target.parent.user_session.ssh_client_ip":                 1719,
	"setrlimit.target.parent.user_session.ssh_client_port":               1720,
	"setrlimit.target.parent.user_session.ssh_public_key":                1721,
	"setrlimit.target.parent.user_session.ssh_session_id":                1722,
	"setrlimit.target.pid":                                               1723,
	"setrlimit.target.ppid":                                              1724,
	"setrlimit.target.sid":                                               1725,
	"setrlimit.target.tid":                                               1726,
	"setrlimit.target.tty_name":                                          1727,
	"setrlimit.target.uid":                                               1728,
	"setrlimit.target.user":                                              1729,
	"setrlimit.target.user_session.id":                                   1730,
	"setrlimit.target.user_session.identity":                             1731,
	"setrlimit.target.user_session.k8s_groups":                           1732,
	"setrlimit.target.user_session.k8s_session_id":                       1733,
	"setrlimit.target.user_session.k8s_uid":                              1734,
	"setrlimit.target.user_session.k8s_username":                         1735,
	"setrlimit.target.user_session.session_type":                         1736,
	"setrlimit.target.user_session.ssh_auth_method":                      1737,
	"setrlimit.target.user_session.ssh_client_ip":                        1738,
	"setrlimit.target.user_session.ssh_client_port":                      1739,
	"setrlimit.target.user_session.ssh_public_key":                       1740,
	"setrlimit.target.user_session.ssh_session_id":                       1741,
	"setsockopt.filter_hash":                                             1742,
	"setsockopt.filter_instructions":                                     1743,
	"setsockopt.filter_len":                                              1744,
	"setsockopt.is_filter_truncated":                                     1745,
	"setsockopt.level":                                                   1746,
	"setsockopt.optname":                                                 1747,
	"setsockopt.retval":                                                  1748,
	"setsockopt.socket_family":                                           1749,
	"setsockopt.socket_protocol":                                         1750,
	"setsockopt.socket_type":                                             1751,
	"setsockopt.used_immediates":                                         1752,
	"setuid.euid":                                                        1753,
	"setuid.euser":                                                       1754,
	"setuid.fsuid":                                                       1755,
	"setuid.fsuser":                                                      1756,
	"setuid.uid":                                                         1757,
	"setuid.user":                                                        1758,
	"setxattr.file.change_time":                                          1759,
	"setxattr.file.destination.name":                                     1760,
	"setxattr.file.destination.namespace":                                1761,
	"setxattr.file.extension":                                            1762,
	"setxattr.file.filesystem":                                           1763,
	"setxattr.file.gid":                                                  1764,
	"setxattr.file.group":                                                1765,
	"setxattr.file.hashes":                                               1766,
	"setxattr.file.in_upper_layer":                                       1767,
	"setxattr.file.inode":                                                1768,
	"setxattr.file.mode":                                                 1769,
	"setxattr.file.modification_time":                                    1770,
	"setxattr.file.mount_detached":                                       1771,
	"setxattr.file.mount_id":                                             1772,
	"setxattr.file.mount_visible":                                        1773,
	"setxattr.file.name":                                                 1774,
	"setxattr.file.package.epoch":                                        1775,
	"setxattr.file.package.name":                                         1776,
	"setxattr.file.package.release":                                      1777,
	"setxattr.file.package.source_epoch":                                 1778,
	"setxattr.file.package.source_release":                               1779,
	"setxattr.file.package.source_version":                               1780,
	"setxattr.file.package.version":                                      1781,
	"setxattr.file.path":                                                 1782,
	"setxattr.file.rights":                                               1783,
	"setxattr.file.uid":                                                  1784,
	"setxattr.file.user":                                                 1785,
	"setxattr.retval":                                                    1786,
	"signal.pid":                                                         1787,
	"signal.retval":                                                      1788,
	"signal.target.ancestors.args":                                       1789,
	"signal.target.ancestors.args_flags":                                 1790,
	"signal.target.ancestors.args_options":                               1791,
	"signal.target.ancestors.args_truncated":                             1792,
	"signal.target.ancestors.argv":                                       1793,
	"signal.target.ancestors.argv0":                                      1794,
	"signal.target.ancestors.auid":                                       1795,
	"signal.target.ancestors.cap_effective":                              1796,
	"signal.target.ancestors.cap_permitted":                              1797,
	"signal.target.ancestors.caps_attempted":                             1798,
	"signal.target.ancestors.caps_used":                                  1799,
	"signal.target.ancestors.cgroup.created_at":                          1800,
	"signal.target.ancestors.cgroup.file.inode":                          1801,
	"signal.target.ancestors.cgroup.file.mount_id":                       1802,
	"signal.target.ancestors.cgroup.id":                                  1803,
	"signal.target.ancestors.cgroup.version":                             1804,
	"signal.target.ancestors.comm":                                       1805,
	"signal.target.ancestors.container.created_at":                       1806,
	"signal.target.ancestors.container.id":                               1807,
	"signal.target.ancestors.container.tags":                             1808,
	"signal.target.ancestors.created_at":                                 1809,
	"signal.target.ancestors.egid":                                       1810,
	"signal.target.ancestors.egroup":                                     1811,
	"signal.target.ancestors.envp":                                       1812,
	"signal.target.ancestors.envs":                                       1813,
	"signal.target.ancestors.envs_truncated":                             1814,
	"signal.target.ancestors.euid":                                       1815,
	"signal.target.ancestors.euser":                                      1816,
	"signal.target.ancestors.file.change_time":                           1817,
	"signal.target.ancestors.file.extension":                             1818,
	"signal.target.ancestors.file.filesystem":                            1819,
	"signal.target.ancestors.file.gid":                                   1820,
	"signal.target.ancestors.file.group":                                 1821,
	"signal.target.ancestors.file.hashes":                                1822,
	"signal.target.ancestors.file.in_upper_layer":                        1823,
	"signal.target.ancestors.file.inode":                                 1824,
	"signal.target.ancestors.file.mode":                                  1825,
	"signal.target.ancestors.file.modification_time":                     1826,
	"signal.target.ancestors.file.mount_detached":                        1827,
	"signal.target.ancestors.file.mount_id":                              1828,
	"signal.target.ancestors.file.mount_visible":                         1829,
	"signal.target.ancestors.file.name":                                  1830,
	"signal.target.ancestors.file.package.epoch":                         1831,
	"signal.target.ancestors.file.package.name":                          1832,
	"signal.target.ancestors.file.package.release":                       1833,
	"signal.target.ancestors.file.package.source_epoch":                  1834,
	"signal.target.ancestors.file.package.source_release":                1835,
	"signal.target.ancestors.file.package.source_version":                1836,
	"signal.target.ancestors.file.package.version":                       1837,
	"signal.target.ancestors.file.path":                                  1838,
	"signal.target.ancestors.file.rights":                                1839,
	"signal.target.ancestors.file.uid":                                   1840,
	"signal.target.ancestors.file.user":                                  1841,
	"signal.target.ancestors.fsgid":                                      1842,
	"signal.target.ancestors.fsgroup":                                    1843,
	"signal.target.ancestors.fsuid":                                      1844,
	"signal.target.ancestors.fsuser":                                     1845,
	"signal.target.ancestors.gid":                                        1846,
	"signal.target.ancestors.group":                                      1847,
	"signal.target.ancestors.interpreter.file.change_time":               1848,
	"signal.target.ancestors.interpreter.file.extension":                 1849,
	"signal.target.ancestors.interpreter.file.filesystem":                1850,
	"signal.target.ancestors.interpreter.file.gid":                       1851,
	"signal.target.ancestors.interpreter.file.group":                     1852,
	"signal.target.ancestors.interpreter.file.hashes":                    1853,
	"signal.target.ancestors.interpreter.file.in_upper_layer":            1854,
	"signal.target.ancestors.interpreter.file.inode":                     1855,
	"signal.target.ancestors.interpreter.file.mode":                      1856,
	"signal.target.ancestors.interpreter.file.modification_time":         1857,
	"signal.target.ancestors.interpreter.file.mount_detached":            1858,
	"signal.target.ancestors.interpreter.file.mount_id":                  1859,
	"signal.target.ancestors.interpreter.file.mount_visible":             1860,
	"signal.target.ancestors.interpreter.file.name":                      1861,
	"signal.target.ancestors.interpreter.file.package.epoch":             1862,
	"signal.target.ancestors.interpreter.file.package.name":              1863,
	"signal.target.ancestors.interpreter.file.package.release":           1864,
	"signal.target.ancestors.interpreter.file.package.source_epoch":      1865,
	"signal.target.ancestors.interpreter.file.package.source_release":    1866,
	"signal.target.ancestors.interpreter.file.package.source_version":    1867,
	"signal.target.ancestors.interpreter.file.package.version":           1868,
	"signal.target.ancestors.interpreter.file.path":                      1869,
	"signal.target.ancestors.interpreter.file.rights":                    1870,
	"signal.target.ancestors.interpreter.file.uid":                       1871,
	"signal.target.ancestors.interpreter.file.user":                      1872,
	"signal.target.ancestors.is_exec":                                    1873,
	"signal.target.ancestors.is_kworker":                                 1874,
	"signal.target.ancestors.is_thread":                                  1875,
	"signal.target.ancestors.mntns":                                      1876,
	"signal.target.ancestors.netns":                                      1877,
	"signal.target.ancestors.pid":                                        1878,
	"signal.target.ancestors.ppid":                                       1879,
	"signal.target.ancestors.sid":                                        1880,
	"signal.target.ancestors.tid":                                        1881,
	"signal.target.ancestors.tty_name":                                   1882,
	"signal.target.ancestors.uid":                                        1883,
	"signal.target.ancestors.user":                                       1884,
	"signal.target.ancestors.user_session.id":                            1885,
	"signal.target.ancestors.user_session.identity":                      1886,
	"signal.target.ancestors.user_session.k8s_groups":                    1887,
	"signal.target.ancestors.user_session.k8s_session_id":                1888,
	"signal.target.ancestors.user_session.k8s_uid":                       1889,
	"signal.target.ancestors.user_session.k8s_username":                  1890,
	"signal.target.ancestors.user_session.session_type":                  1891,
	"signal.target.ancestors.user_session.ssh_auth_method":               1892,
	"signal.target.ancestors.user_session.ssh_client_ip":                 1893,
	"signal.target.ancestors.user_session.ssh_client_port":               1894,
	"signal.target.ancestors.user_session.ssh_public_key":                1895,
	"signal.target.ancestors.user_session.ssh_session_id":                1896,
	"signal.target.args":                                                 1897,
	"signal.target.args_flags":                                           1898,
	"signal.target.args_options":                                         1899,
	"signal.target.args_truncated":                                       1900,
	"signal.target.argv":                                                 1901,
	"signal.target.argv0":                                                1902,
	"signal.target.auid":                                                 1903,
	"signal.target.cap_effective":                                        1904,
	"signal.target.cap_permitted":                                        1905,
	"signal.target.caps_attempted":                                       1906,
	"signal.target.caps_used":                                            1907,
	"signal.target.cgroup.created_at":                                    1908,
	"signal.target.cgroup.file.inode":                                    1909,
	"signal.target.cgroup.file.mount_id":                                 1910,
	"signal.target.cgroup.id":                                            1911,
	"signal.target.cgroup.version":                                       1912,
	"signal.target.comm":                                                 1913,
	"signal.target.container.created_at":                                 1914,
	"signal.target.container.id":                                         1915,
	"signal.target.container.tags":                                       1916,
	"signal.target.created_at":                                           1917,
	"signal.target.egid":                                                 1918,
	"signal.target.egroup":                                               1919,
	"signal.target.envp":                                                 1920,
	"signal.target.envs":                                                 1921,
	"signal.target.envs_truncated":                                       1922,
	"signal.target.euid":                                                 1923,
	"signal.target.euser":                                                1924,
	"signal.target.file.change_time":                                     1925,
	"signal.target.file.extension":                                       1926,
	"signal.target.file.filesystem":                                      1927,
	"signal.target.file.gid":                                             1928,
	"signal.target.file.group":                                           1929,
	"signal.target.file.hashes":                                          1930,
	"signal.target.file.in_upper_layer":                                  1931,
	"signal.target.file.inode":                                           1932,
	"signal.target.file.mode":                                            1933,
	"signal.target.file.modification_time":                               1934,
	"signal.target.file.mount_detached":                                  1935,
	"signal.target.file.mount_id":                                        1936,
	"signal.target.file.mount_visible":                                   1937,
	"signal.target.file.name":                                            1938,
	"signal.target.file.package.epoch":                                   1939,
	"signal.target.file.package.name":                                    1940,
	"signal.target.file.package.release":                                 1941,
	"signal.target.file.package.source_epoch":                            1942,
	"signal.target.file.package.source_release":                          1943,
	"signal.target.file.package.source_version":                          1944,
	"signal.target.file.package.version":                                 1945,
	"signal.target.file.path":                                            1946,
	"signal.target.file.rights":                                          1947,
	"signal.target.file.uid":                                             1948,
	"signal.target.file.user":                                            1949,
	"signal.target.fsgid":                                                1950,
	"signal.target.fsgroup":                                              1951,
	"signal.target.fsuid":                                                1952,
	"signal.target.fsuser":                                               1953,
	"signal.target.gid":                                                  1954,
	"signal.target.group":                                                1955,
	"signal.target.interpreter.file.change_time":                         1956,
	"signal.target.interpreter.file.extension":                           1957,
	"signal.target.interpreter.file.filesystem":                          1958,
	"signal.target.interpreter.file.gid":                                 1959,
	"signal.target.interpreter.file.group":                               1960,
	"signal.target.interpreter.file.hashes":                              1961,
	"signal.target.interpreter.file.in_upper_layer":                      1962,
	"signal.target.interpreter.file.inode":                               1963,
	"signal.target.interpreter.file.mode":                                1964,
	"signal.target.interpreter.file.modification_time":                   1965,
	"signal.target.interpreter.file.mount_detached":                      1966,
	"signal.target.interpreter.file.mount_id":                            1967,
	"signal.target.interpreter.file.mount_visible":                       1968,
	"signal.target.interpreter.file.name":                                1969,
	"signal.target.interpreter.file.package.epoch":                       1970,
	"signal.target.interpreter.file.package.name":                        1971,
	"signal.target.interpreter.file.package.release":                     1972,
	"signal.target.interpreter.file.package.source_epoch":                1973,
	"signal.target.interpreter.file.package.source_release":              1974,
	"signal.target.interpreter.file.package.source_version":              1975,
	"signal.target.interpreter.file.package.version":                     1976,
	"signal.target.interpreter.file.path":                                1977,
	"signal.target.interpreter.file.rights":                              1978,
	"signal.target.interpreter.file.uid":                                 1979,
	"signal.target.interpreter.file.user":                                1980,
	"signal.target.is_exec":                                              1981,
	"signal.target.is_kworker":                                           1982,
	"signal.target.is_thread":                                            1983,
	"signal.target.mntns":                                                1984,
	"signal.target.netns":                                                1985,
	"signal.target.parent.args":                                          1986,
	"signal.target.parent.args_flags":                                    1987,
	"signal.target.parent.args_options":                                  1988,
	"signal.target.parent.args_truncated":                                1989,
	"signal.target.parent.argv":                                          1990,
	"signal.target.parent.argv0":                                         1991,
	"signal.target.parent.auid":                                          1992,
	"signal.target.parent.cap_effective":                                 1993,
	"signal.target.parent.cap_permitted":                                 1994,
	"signal.target.parent.caps_attempted":                                1995,
	"signal.target.parent.caps_used":                                     1996,
	"signal.target.parent.cgroup.created_at":                             1997,
	"signal.target.parent.cgroup.file.inode":                             1998,
	"signal.target.parent.cgroup.file.mount_id":                          1999,
	"signal.target.parent.cgroup.id":                                     2000,
	"signal.target.parent.cgroup.version":                                2001,
	"signal.target.parent.comm":                                          2002,
	"signal.target.parent.container.created_at":                          2003,
	"signal.target.parent.container.id":                                  2004,
	"signal.target.parent.container.tags":                                2005,
	"signal.target.parent.created_at":                                    2006,
	"signal.target.parent.egid":                                          2007,
	"signal.target.parent.egroup":                                        2008,
	"signal.target.parent.envp":                                          2009,
	"signal.target.parent.envs":                                          2010,
	"signal.target.parent.envs_truncated":                                2011,
	"signal.target.parent.euid":                                          2012,
	"signal.target.parent.euser":                                         2013,
	"signal.target.parent.file.change_time":                              2014,
	"signal.target.parent.file.extension":                                2015,
	"signal.target.parent.file.filesystem":                               2016,
	"signal.target.parent.file.gid":                                      2017,
	"signal.target.parent.file.group":                                    2018,
	"signal.target.parent.file.hashes":                                   2019,
	"signal.target.parent.file.in_upper_layer":                           2020,
	"signal.target.parent.file.inode":                                    2021,
	"signal.target.parent.file.mode":                                     2022,
	"signal.target.parent.file.modification_time":                        2023,
	"signal.target.parent.file.mount_detached":                           2024,
	"signal.target.parent.file.mount_id":                                 2025,
	"signal.target.parent.file.mount_visible":                            2026,
	"signal.target.parent.file.name":                                     2027,
	"signal.target.parent.file.package.epoch":                            2028,
	"signal.target.parent.file.package.name":                             2029,
	"signal.target.parent.file.package.release":                          2030,
	"signal.target.parent.file.package.source_epoch":                     2031,
	"signal.target.parent.file.package.source_release":                   2032,
	"signal.target.parent.file.package.source_version":                   2033,
	"signal.target.parent.file.package.version":                          2034,
	"signal.target.parent.file.path":                                     2035,
	"signal.target.parent.file.rights":                                   2036,
	"signal.target.parent.file.uid":                                      2037,
	"signal.target.parent.file.user":                                     2038,
	"signal.target.parent.fsgid":                                         2039,
	"signal.target.parent.fsgroup":                                       2040,
	"signal.target.parent.fsuid":                                         2041,
	"signal.target.parent.fsuser":                                        2042,
	"signal.target.parent.gid":                                           2043,
	"signal.target.parent.group":                                         2044,
	"signal.target.parent.interpreter.file.change_time":                  2045,
	"signal.target.parent.interpreter.file.extension":                    2046,
	"signal.target.parent.interpreter.file.filesystem":                   2047,
	"signal.target.parent.interpreter.file.gid":                          2048,
	"signal.target.parent.interpreter.file.group":                        2049,
	"signal.target.parent.interpreter.file.hashes":                       2050,
	"signal.target.parent.interpreter.file.in_upper_layer":               2051,
	"signal.target.parent.interpreter.file.inode":                        2052,
	"signal.target.parent.interpreter.file.mode":                         2053,
	"signal.target.parent.interpreter.file.modification_time":            2054,
	"signal.target.parent.interpreter.file.mount_detached":               2055,
	"signal.target.parent.interpreter.file.mount_id":                     2056,
	"signal.target.parent.interpreter.file.mount_visible":                2057,
	"signal.target.parent.interpreter.file.name":                         2058,
	"signal.target.parent.interpreter.file.package.epoch":                2059,
	"signal.target.parent.interpreter.file.package.name":                 2060,
	"signal.target.parent.interpreter.file.package.release":              2061,
	"signal.target.parent.interpreter.file.package.source_epoch":         2062,
	"signal.target.parent.interpreter.file.package.source_release":       2063,
	"signal.target.parent.interpreter.file.package.source_version":       2064,
	"signal.target.parent.interpreter.file.package.version":              2065,
	"signal.target.parent.interpreter.file.path":                         2066,
	"signal.target.parent.interpreter.file.rights":                       2067,
	"signal.target.parent.interpreter.file.uid":                          2068,
	"signal.target.parent.interpreter.file.user":                         2069,
	"signal.target.parent.is_exec":                                       2070,
	"signal.target.parent.is_kworker":                                    2071,
	"signal.target.parent.is_thread":                                     2072,
	"signal.target.parent.mntns":                                         2073,
	"signal.target.parent.netns":                                         2074,
	"signal.target.parent.pid":                                           2075,
	"signal.target.parent.ppid":                                          2076,
	"signal.target.parent.sid":                                           2077,
	"signal.target.parent.tid":                                           2078,
	"signal.target.parent.tty_name":                                      2079,
	"signal.target.parent.uid":                                           2080,
	"signal.target.parent.user":                                          2081,
	"signal.target.parent.user_session.id":                               2082,
	"signal.target.parent.user_session.identity":                         2083,
	"signal.target.parent.user_session.k8s_groups":                       2084,
	"signal.target.parent.user_session.k8s_session_id":                   2085,
	"signal.target.parent.user_session.k8s_uid":                          2086,
	"signal.target.parent.user_session.k8s_username":                     2087,
	"signal.target.parent.user_session.session_type":                     2088,
	"signal.target.parent.user_session.ssh_auth_method":                  2089,
	"signal.target.parent.user_session.ssh_client_ip":                    2090,
	"signal.target.parent.user_session.ssh_client_port":                  2091,
	"signal.target.parent.user_session.ssh_public_key":                   2092,
	"signal.target.parent.user_session.ssh_session_id":                   2093,
	"signal.target.pid":                                                  2094,
	"signal.target.ppid":                                                 2095,
	"signal.target.sid":                                                  2096,
	"signal.target.tid":                                                  2097,
	"signal.target.tty_name":                                             2098,
	"signal.target.uid":                                                  2099,
	"signal.target.user":                                                 2100,
	"signal.target.user_session.id":                                      2101,
	"signal.target.user_session.identity":                                2102,
	"signal.target.user_session.k8s_groups":                              2103,
	"signal.target.user_session.k8s_session_id":                          2104,
	"signal.target.user_session.k8s_uid":                                 2105,
	"signal.target.user_session.k8s_username":                            2106,
	"signal.target.user_session.session_type":                            2107,
	"signal.target.user_session.ssh_auth_method":                         2108,
	"signal.target.user_session.ssh_client_ip":                           2109,
	"signal.target.user_session.ssh_client_port":                         2110,
	"signal.target.user_session.ssh_public_key":                          2111,
	"signal.target.user_session.ssh_session_id":                          2112,
	"signal.type":                        2113,
	"socket.domain":                      2114,
	"socket.protocol":                    2115,
	"socket.retval":                      2116,
	"socket.type":                        2117,
	"splice.file.change_time":            2118,
	"splice.file.extension":              2119,
	"splice.file.filesystem":             2120,
	"splice.file.gid":                    2121,
	"splice.file.group":                  2122,
	"splice.file.hashes":                 2123,
	"splice.file.in_upper_layer":         2124,
	"splice.file.inode":                  2125,
	"splice.file.mode":                   2126,
	"splice.file.modification_time":      2127,
	"splice.file.mount_detached":         2128,
	"splice.file.mount_id":               2129,
	"splice.file.mount_visible":          2130,
	"splice.file.name":                   2131,
	"splice.file.package.epoch":          2132,
	"splice.file.package.name":           2133,
	"splice.file.package.release":        2134,
	"splice.file.package.source_epoch":   2135,
	"splice.file.package.source_release": 2136,
	"splice.file.package.source_version": 2137,
	"splice.file.package.version":        2138,
	"splice.file.path":                   2139,
	"splice.file.rights":                 2140,
	"splice.file.uid":                    2141,
	"splice.file.user":                   2142,
	"splice.pipe_entry_flag":             2143,
	"splice.pipe_exit_flag":              2144,
	"splice.retval":                      2145,
	"sysctl.action":                      2146,
	"sysctl.file_position":               2147,
	"sysctl.name":                        2148,
	"sysctl.name_truncated":              2149,
	"sysctl.old_value":                   2150,
	"sysctl.old_value_truncated":         2151,
	"sysctl.value":                       2152,
	"sysctl.value_truncated":             2153,
	"unlink.file.change_time":            2154,
	"unlink.file.extension":              2155,
	"unlink.file.filesystem":             2156,
	"unlink.file.gid":                    2157,
	"unlink.file.group":                  2158,
	"unlink.file.hashes":                 2159,
	"unlink.file.in_upper_layer":         2160,
	"unlink.file.inode":                  2161,
	"unlink.file.mode":                   2162,
	"unlink.file.modification_time":      2163,
	"unlink.file.mount_detached":         2164,
	"unlink.file.mount_id":               2165,
	"unlink.file.mount_visible":          2166,
	"unlink.file.name":                   2167,
	"unlink.file.package.epoch":          2168,
	"unlink.file.package.name":           2169,
	"unlink.file.package.release":        2170,
	"unlink.file.package.source_epoch":   2171,
	"unlink.file.package.source_release": 2172,
	"unlink.file.package.source_version": 2173,
	"unlink.file.package.version":        2174,
	"unlink.file.path":                   2175,
	"unlink.file.rights":                 2176,
	"unlink.file.uid":                    2177,
	"unlink.file.user":                   2178,
	"unlink.flags":                       2179,
	"unlink.retval":                      2180,
	"unlink.syscall.dirfd":               2181,
	"unlink.syscall.flags":               2182,
	"unlink.syscall.path":                2183,
	"unload_module.name":                 2184,
	"unload_module.retval":               2185,
	"utimes.file.change_time":            2186,
	"utimes.file.extension":              2187,
	"utimes.file.filesystem":             2188,
	"utimes.file.gid":                    2189,
	"utimes.file.group":                  2190,
	"utimes.file.hashes":                 2191,
	"utimes.file.in_upper_layer":         2192,
	"utimes.file.inode":                  2193,
	"utimes.file.mode":                   2194,
	"utimes.file.modification_time":      2195,
	"utimes.file.mount_detached":         2196,
	"utimes.file.mount_id":               2197,
	"utimes.file.mount_visible":          2198,
	"utimes.file.name":                   2199,
	"utimes.file.package.epoch":          2200,
	"utimes.file.package.name":           2201,
	"utimes.file.package.release":        2202,
	"utimes.file.package.source_epoch":   2203,
	"utimes.file.package.source_release": 2204,
	"utimes.file.package.source_version": 2205,
	"utimes.file.package.version":        2206,
	"utimes.file.path":                   2207,
	"utimes.file.rights":                 2208,
	"utimes.file.uid":                    2209,
	"utimes.file.user":                   2210,
	"utimes.retval":                      2211,
	"utimes.syscall.path":                2212,
}

// celIterators opens a cursor over an iterated SECL field, indexed by its place
// in the layout.
//
// A cursor yields the elements one at a time, through the same model iterator
// the accessors use but through its Front/Next pair rather than its indexed At.
// Nothing asks how many elements there are, so a quantifier walks exactly as far
// as its predicate needs, and an element is reached once rather than once per
// position before it.
//
// A cursor reads its root from the event, which is where every iterator in the
// model starts; an iterator nested inside an iterated element is rejected by the
// generator rather than read from the wrong root.
var celIterators = []celIterator{
	// 0: network_flow_monitor.flows
	func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.Flow]{
			iterator: &model.FlowsIterator{Root: ev.NetworkFlowMonitor.Flows},
			ctx:      ctx,
		}
	},
	// 1: process.ancestors
	func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor},
			ctx:      ctx,
		}
	},
	// 2: ptrace.tracee.ancestors
	func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.PTrace.Tracee.Ancestor},
			ctx:      ctx,
		}
	},
	// 3: setrlimit.target.ancestors
	func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.Setrlimit.Target.Ancestor},
			ctx:      ctx,
		}
	},
	// 4: signal.target.ancestors
	func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.Signal.Target.Ancestor},
			ctx:      ctx,
		}
	},
}

// celIteratorIndex gives the index of each iterated field, for the same reason
// celReaderIndex does.
var celIteratorIndex = map[string]int{
	"network_flow_monitor.flows": 0,
	"process.ancestors":          1,
	"ptrace.tracee.ancestors":    2,
	"setrlimit.target.ancestors": 3,
	"signal.target.ancestors":    4,
}

// celGlobFields names the fields whose `~"…"` and `=~` patterns SECL compiles as a
// glob rather than as a pattern: `*` stops at a path separator and `**` is allowed.
//
// It is a property of the *field*, not of the literal — SECL reads it off the operator
// override the field carries and rewrites the value type of whatever it is compared
// against (eval.GlobCmp) — so the translation has to consult it wherever it turns a
// pattern into a call. TestGlobFieldsAgreeWithSECL checks this table against SECL
// field by field.
var celGlobFields = map[string]struct{}{
	"cgroup_write.file.path":                           {},
	"chdir.file.path":                                  {},
	"chmod.file.path":                                  {},
	"chown.file.path":                                  {},
	"exec.file.path":                                   {},
	"exec.interpreter.file.path":                       {},
	"exit.file.path":                                   {},
	"exit.interpreter.file.path":                       {},
	"link.file.destination.path":                       {},
	"link.file.path":                                   {},
	"load_module.file.path":                            {},
	"mkdir.file.path":                                  {},
	"mmap.file.path":                                   {},
	"open.file.path":                                   {},
	"process.ancestors.file.path":                      {},
	"process.ancestors.interpreter.file.path":          {},
	"process.file.path":                                {},
	"process.interpreter.file.path":                    {},
	"process.parent.file.path":                         {},
	"process.parent.interpreter.file.path":             {},
	"ptrace.tracee.ancestors.file.path":                {},
	"ptrace.tracee.ancestors.interpreter.file.path":    {},
	"ptrace.tracee.file.path":                          {},
	"ptrace.tracee.interpreter.file.path":              {},
	"ptrace.tracee.parent.file.path":                   {},
	"ptrace.tracee.parent.interpreter.file.path":       {},
	"removexattr.file.path":                            {},
	"rename.file.destination.path":                     {},
	"rename.file.path":                                 {},
	"rmdir.file.path":                                  {},
	"setrlimit.target.ancestors.file.path":             {},
	"setrlimit.target.ancestors.interpreter.file.path": {},
	"setrlimit.target.file.path":                       {},
	"setrlimit.target.interpreter.file.path":           {},
	"setrlimit.target.parent.file.path":                {},
	"setrlimit.target.parent.interpreter.file.path":    {},
	"setxattr.file.path":                               {},
	"signal.target.ancestors.file.path":                {},
	"signal.target.ancestors.interpreter.file.path":    {},
	"signal.target.file.path":                          {},
	"signal.target.interpreter.file.path":              {},
	"signal.target.parent.file.path":                   {},
	"signal.target.parent.interpreter.file.path":       {},
	"splice.file.path":                                 {},
	"unlink.file.path":                                 {},
	"utimes.file.path":                                 {},
}
