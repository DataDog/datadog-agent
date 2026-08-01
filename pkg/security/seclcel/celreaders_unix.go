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

// celReaders reads one SECL field, keyed by its SECL name.
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
var celReaders = map[string]celReader{
	"accept.addr.family": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Accept.AddrFamily))
	},
	"accept.addr.hostname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.hostname")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveAcceptHostnames(ev, &ev.Accept))
	},
	"accept.addr.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Accept.Addr.IPNet)
	},
	"accept.addr.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.Accept.Addr))
	},
	"accept.addr.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.addr.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Accept.Addr.Port))
	},
	"accept.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("accept.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Accept.SyscallEvent.Retval))
	},
	"bind.addr.family": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.AddrFamily))
	},
	"bind.addr.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Bind.Addr.IPNet)
	},
	"bind.addr.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.Bind.Addr))
	},
	"bind.addr.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.addr.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.Addr.Port))
	},
	"bind.protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.Protocol))
	},
	"bind.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bind.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Bind.SyscallEvent.Retval))
	},
	"bpf.cmd": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.cmd")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Cmd))
	},
	"bpf.map.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.map.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BPF.Map.Name)
	},
	"bpf.map.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.map.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Map.Type))
	},
	"bpf.prog.attach_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.attach_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Program.AttachType))
	},
	"bpf.prog.helpers": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.helpers")
		ev := ctx.Event.(*model.Event)
		result := make([]int, len(ev.BPF.Program.Helpers))
		for i, v := range ev.BPF.Program.Helpers {
			result[i] = int(v)
		}
		return intsToVal(result)
	},
	"bpf.prog.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BPF.Program.Name)
	},
	"bpf.prog.tag": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.tag")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BPF.Program.Tag)
	},
	"bpf.prog.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.prog.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.Program.Type))
	},
	"bpf.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("bpf.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BPF.SyscallEvent.Retval))
	},
	"capabilities.attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capabilities.attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCapabilitiesAttempted(ev, &ev.CapabilitiesUsage)))
	},
	"capabilities.used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capabilities.used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCapabilitiesUsed(ev, &ev.CapabilitiesUsage)))
	},
	"capset.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capset.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Capset.CapEffective))
	},
	"capset.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("capset.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Capset.CapPermitted))
	},
	"cgroup_write.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.CTime))
	},
	"cgroup_write.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.GID))
	},
	"cgroup_write.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.CgroupWrite.File.FileFields))
	},
	"cgroup_write.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.CgroupWrite.File.FileFields))
	},
	"cgroup_write.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.PathKey.Inode))
	},
	"cgroup_write.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.Mode))
	},
	"cgroup_write.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.MTime))
	},
	"cgroup_write.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.CgroupWrite.File.MountDetached)
	},
	"cgroup_write.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.PathKey.MountID))
	},
	"cgroup_write.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.CgroupWrite.File.MountVisible)
	},
	"cgroup_write.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.CgroupWrite.File))
	},
	"cgroup_write.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.CgroupWrite.File.FileFields)))
	},
	"cgroup_write.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.File.FileFields.UID))
	},
	"cgroup_write.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.CgroupWrite.File.FileFields))
	},
	"cgroup_write.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("cgroup_write.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.CgroupWrite.Pid))
	},
	"chdir.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.CTime))
	},
	"chdir.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Chdir.File))
	},
	"chdir.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chdir.File))
	},
	"chdir.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.GID))
	},
	"chdir.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chdir.File.FileFields))
	},
	"chdir.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chdir.File))
	},
	"chdir.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chdir.File.FileFields))
	},
	"chdir.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.PathKey.Inode))
	},
	"chdir.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.Mode))
	},
	"chdir.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.MTime))
	},
	"chdir.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chdir.File.MountDetached)
	},
	"chdir.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.PathKey.MountID))
	},
	"chdir.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chdir.File.MountVisible)
	},
	"chdir.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chdir.File))
	},
	"chdir.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Chdir.File))
	},
	"chdir.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Chdir.File))
	},
	"chdir.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Chdir.File))
	},
	"chdir.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Chdir.File))
	},
	"chdir.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Chdir.File))
	},
	"chdir.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chdir.File))
	},
	"chdir.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chdir.File))
	},
	"chdir.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chdir.File))
	},
	"chdir.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Chdir.File.FileFields)))
	},
	"chdir.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.File.FileFields.UID))
	},
	"chdir.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chdir.File.FileFields))
	},
	"chdir.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chdir.SyscallEvent.Retval))
	},
	"chdir.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chdir.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chdir.SyscallContext))
	},
	"chmod.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.CTime))
	},
	"chmod.file.destination.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.Mode))
	},
	"chmod.file.destination.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.Mode))
	},
	"chmod.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Chmod.File))
	},
	"chmod.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chmod.File))
	},
	"chmod.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.GID))
	},
	"chmod.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chmod.File.FileFields))
	},
	"chmod.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chmod.File))
	},
	"chmod.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chmod.File.FileFields))
	},
	"chmod.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.PathKey.Inode))
	},
	"chmod.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.Mode))
	},
	"chmod.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.MTime))
	},
	"chmod.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chmod.File.MountDetached)
	},
	"chmod.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.PathKey.MountID))
	},
	"chmod.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chmod.File.MountVisible)
	},
	"chmod.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chmod.File))
	},
	"chmod.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Chmod.File))
	},
	"chmod.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Chmod.File))
	},
	"chmod.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Chmod.File))
	},
	"chmod.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Chmod.File))
	},
	"chmod.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Chmod.File))
	},
	"chmod.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chmod.File))
	},
	"chmod.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chmod.File))
	},
	"chmod.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chmod.File))
	},
	"chmod.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Chmod.File.FileFields)))
	},
	"chmod.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.File.FileFields.UID))
	},
	"chmod.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chmod.File.FileFields))
	},
	"chmod.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chmod.SyscallEvent.Retval))
	},
	"chmod.syscall.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.syscall.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chmod.SyscallContext)))
	},
	"chmod.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chmod.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chmod.SyscallContext))
	},
	"chown.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.CTime))
	},
	"chown.file.destination.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.GID))
	},
	"chown.file.destination.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveChownGID(ev, &ev.Chown))
	},
	"chown.file.destination.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.UID))
	},
	"chown.file.destination.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.destination.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveChownUID(ev, &ev.Chown))
	},
	"chown.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Chown.File))
	},
	"chown.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Chown.File))
	},
	"chown.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.GID))
	},
	"chown.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Chown.File.FileFields))
	},
	"chown.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Chown.File))
	},
	"chown.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Chown.File.FileFields))
	},
	"chown.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.PathKey.Inode))
	},
	"chown.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.Mode))
	},
	"chown.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.MTime))
	},
	"chown.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chown.File.MountDetached)
	},
	"chown.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.PathKey.MountID))
	},
	"chown.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Chown.File.MountVisible)
	},
	"chown.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Chown.File))
	},
	"chown.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Chown.File))
	},
	"chown.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Chown.File))
	},
	"chown.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Chown.File))
	},
	"chown.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Chown.File))
	},
	"chown.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Chown.File))
	},
	"chown.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Chown.File))
	},
	"chown.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Chown.File))
	},
	"chown.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Chown.File))
	},
	"chown.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Chown.File.FileFields)))
	},
	"chown.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.File.FileFields.UID))
	},
	"chown.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Chown.File.FileFields))
	},
	"chown.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Chown.SyscallEvent.Retval))
	},
	"chown.syscall.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.syscall.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Chown.SyscallContext)))
	},
	"chown.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Chown.SyscallContext))
	},
	"chown.syscall.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("chown.syscall.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Chown.SyscallContext)))
	},
	"connect.addr.family": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.AddrFamily))
	},
	"connect.addr.hostname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.hostname")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveConnectHostnames(ev, &ev.Connect))
	},
	"connect.addr.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Connect.Addr.IPNet)
	},
	"connect.addr.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.Connect.Addr))
	},
	"connect.addr.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.addr.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.Addr.Port))
	},
	"connect.protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.Protocol))
	},
	"connect.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("connect.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Connect.SyscallEvent.Retval))
	},
	"dns.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.ID))
	},
	"dns.question.class": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.class")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Class))
	},
	"dns.question.count": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.count")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Count))
	},
	"dns.question.length": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.length")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Size))
	},
	"dns.question.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DNS.Question.Name)
	},
	"dns.question.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.question.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.DNS.Question.Type))
	},
	"dns.response.cnames": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.response.cnames")
		ev := ctx.Event.(*model.Event)
		if !ev.DNS.HasResponse() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.DNS.Response.CNames)
	},
	"dns.response.code": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.response.code")
		ev := ctx.Event.(*model.Event)
		if !ev.DNS.HasResponse() {
			return types.Int(-1)
		}
		return types.Int(int(ev.DNS.Response.ResponseCode))
	},
	"dns.response.ips": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("dns.response.ips")
		ev := ctx.Event.(*model.Event)
		if !ev.DNS.HasResponse() {
			return cidrsToVal([]net.IPNet{})
		}
		return cidrsToVal(ev.DNS.Response.IPs)
	},
	"event.async": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.async")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveAsync(ev))
	},
	"event.hostname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.hostname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent))
	},
	"event.origin": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.origin")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.Origin)
	},
	"event.os": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.os")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.Os)
	},
	"event.rule.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.rule.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.BaseEvent.RuleTags)
	},
	"event.service": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.service")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent))
	},
	"event.signature": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.signature")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSignature(ev))
	},
	"event.source": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.source")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSource(ev, &ev.BaseEvent))
	},
	"event.timestamp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.timestamp")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)))
	},
	"exec.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exec.Process))
	},
	"exec.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exec.Process))
	},
	"exec.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exec.Process))
	},
	"exec.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exec.Process))
	},
	"exec.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exec.Process))
	},
	"exec.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exec.Process))
	},
	"exec.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.AUID))
	},
	"exec.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.CapEffective))
	},
	"exec.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.CapPermitted))
	},
	"exec.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CapsAttempted))
	},
	"exec.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CapsUsed))
	},
	"exec.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CGroup.CreatedAt))
	},
	"exec.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CGroup.CGroupPathKey.Inode))
	},
	"exec.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.CGroup.CGroupPathKey.MountID))
	},
	"exec.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exec.Process.CGroup.CGroupID))
	},
	"exec.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Exec.Process.CGroup))
	},
	"exec.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Comm)
	},
	"exec.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.ContainerContext.CreatedAt))
	},
	"exec.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exec.Process.ContainerContext.ContainerID))
	},
	"exec.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exec.Process.ContainerContext))
	},
	"exec.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)))
	},
	"exec.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.EGID))
	},
	"exec.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.EGroup)
	},
	"exec.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process))
	},
	"exec.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process))
	},
	"exec.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exec.Process))
	},
	"exec.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.EUID))
	},
	"exec.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.EUser)
	},
	"exec.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.CTime))
	},
	"exec.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.GID))
	},
	"exec.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.FileEvent.FileFields))
	},
	"exec.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.FileEvent.FileFields))
	},
	"exec.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"exec.file.metadata.abi": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.abi")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataABI(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.metadata.architecture": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.architecture")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataArchitecture(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.metadata.compression": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.compression")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataCompression(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.metadata.is_executable": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.is_executable")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileMetadataIsExecutable(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.metadata.is_garble_obfuscated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.is_garble_obfuscated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileMetadataIsGarbleObfuscated(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.metadata.is_upx_packed": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.is_upx_packed")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileMetadataIsUPXPacked(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.metadata.size": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.size")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveFileMetadataSize(ev, &ev.Exec.FileMetadata)))
	},
	"exec.file.metadata.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.metadata.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveFileMetadataType(ev, &ev.Exec.FileMetadata))
	},
	"exec.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.Mode))
	},
	"exec.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.MTime))
	},
	"exec.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.FileEvent.MountDetached)
	},
	"exec.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"exec.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.FileEvent.MountVisible)
	},
	"exec.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.FileEvent.FileFields)))
	},
	"exec.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.FileEvent.FileFields.UID))
	},
	"exec.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.FileEvent.FileFields))
	},
	"exec.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.FSGID))
	},
	"exec.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.FSGroup)
	},
	"exec.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.FSUID))
	},
	"exec.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.FSUser)
	},
	"exec.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.GID))
	},
	"exec.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.Group)
	},
	"exec.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"exec.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"exec.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"exec.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"exec.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"exec.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"exec.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"exec.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"exec.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"exec.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exec.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"exec.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.LinuxBinprm.FileEvent))
	},
	"exec.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"exec.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exec.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"exec.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exec.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exec.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"exec.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exec.Process.IsExec)
	},
	"exec.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exec.Process.PIDContext.IsKworker)
	},
	"exec.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exec.Process))
	},
	"exec.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.MntNS))
	},
	"exec.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.NetNS))
	},
	"exec.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.Pid))
	},
	"exec.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PPid))
	},
	"exec.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.SID))
	},
	"exec.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Exec.SyscallContext))
	},
	"exec.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.Tid))
	},
	"exec.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.TTYName)
	},
	"exec.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.Credentials.UID))
	},
	"exec.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.Credentials.User)
	},
	"exec.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Exec.Process.UserSession))
	},
	"exec.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Exec.Process.UserSession))
	},
	"exec.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exec.Process.UserSession.K8SSessionContext))
	},
	"exec.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"exec.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exec.Process.UserSession.K8SSessionContext))
	},
	"exec.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exec.Process.UserSession.K8SSessionContext))
	},
	"exec.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Exec.Process.UserSession))
	},
	"exec.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exec.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"exec.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Exec.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"exec.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exec.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	"exec.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"exec.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"exit.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Exit.Process))
	},
	"exit.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Exit.Process))
	},
	"exit.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Exit.Process))
	},
	"exit.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Exit.Process))
	},
	"exit.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Exit.Process))
	},
	"exit.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Exit.Process))
	},
	"exit.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.AUID))
	},
	"exit.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.CapEffective))
	},
	"exit.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.CapPermitted))
	},
	"exit.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CapsAttempted))
	},
	"exit.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CapsUsed))
	},
	"exit.cause": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cause")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Cause))
	},
	"exit.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CGroup.CreatedAt))
	},
	"exit.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CGroup.CGroupPathKey.Inode))
	},
	"exit.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.CGroup.CGroupPathKey.MountID))
	},
	"exit.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exit.Process.CGroup.CGroupID))
	},
	"exit.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Exit.Process.CGroup))
	},
	"exit.code": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.code")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Code))
	},
	"exit.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Comm)
	},
	"exit.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.ContainerContext.CreatedAt))
	},
	"exit.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exit.Process.ContainerContext.ContainerID))
	},
	"exit.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exit.Process.ContainerContext))
	},
	"exit.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)))
	},
	"exit.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.EGID))
	},
	"exit.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.EGroup)
	},
	"exit.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process))
	},
	"exit.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process))
	},
	"exit.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Exit.Process))
	},
	"exit.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.EUID))
	},
	"exit.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.EUser)
	},
	"exit.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.CTime))
	},
	"exit.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.GID))
	},
	"exit.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.FileEvent.FileFields))
	},
	"exit.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.FileEvent.FileFields))
	},
	"exit.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"exit.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.Mode))
	},
	"exit.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.MTime))
	},
	"exit.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.FileEvent.MountDetached)
	},
	"exit.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"exit.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.FileEvent.MountVisible)
	},
	"exit.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.FileEvent.FileFields)))
	},
	"exit.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.FileEvent.FileFields.UID))
	},
	"exit.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.FileEvent.FileFields))
	},
	"exit.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.FSGID))
	},
	"exit.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.FSGroup)
	},
	"exit.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.FSUID))
	},
	"exit.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.FSUser)
	},
	"exit.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.GID))
	},
	"exit.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.Group)
	},
	"exit.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"exit.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"exit.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"exit.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"exit.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"exit.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"exit.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"exit.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"exit.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"exit.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Exit.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"exit.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.LinuxBinprm.FileEvent))
	},
	"exit.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"exit.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Exit.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"exit.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Exit.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Exit.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"exit.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exit.Process.IsExec)
	},
	"exit.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Exit.Process.PIDContext.IsKworker)
	},
	"exit.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Exit.Process))
	},
	"exit.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.MntNS))
	},
	"exit.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.NetNS))
	},
	"exit.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.Pid))
	},
	"exit.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PPid))
	},
	"exit.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.SID))
	},
	"exit.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.Tid))
	},
	"exit.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.TTYName)
	},
	"exit.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.Credentials.UID))
	},
	"exit.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.Credentials.User)
	},
	"exit.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Exit.Process.UserSession))
	},
	"exit.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Exit.Process.UserSession))
	},
	"exit.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Exit.Process.UserSession.K8SSessionContext))
	},
	"exit.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"exit.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Exit.Process.UserSession.K8SSessionContext))
	},
	"exit.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Exit.Process.UserSession.K8SSessionContext))
	},
	"exit.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Exit.Process.UserSession))
	},
	"exit.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exit.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"exit.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Exit.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"exit.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Exit.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	"exit.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"exit.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"imds.aws.is_imds_v2": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.aws.is_imds_v2")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.IMDS.AWS.IsIMDSv2)
	},
	"imds.aws.security_credentials.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.aws.security_credentials.type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.AWS.SecurityCredentials.Type)
	},
	"imds.cloud_provider": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.cloud_provider")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.CloudProvider)
	},
	"imds.host": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.host")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.Host)
	},
	"imds.server": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.server")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.Server)
	},
	"imds.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.Type)
	},
	"imds.url": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.url")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.URL)
	},
	"imds.user_agent": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("imds.user_agent")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.IMDS.UserAgent)
	},
	"link.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.CTime))
	},
	"link.file.destination.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.CTime))
	},
	"link.file.destination.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Link.Target))
	},
	"link.file.destination.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Target))
	},
	"link.file.destination.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.GID))
	},
	"link.file.destination.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Target.FileFields))
	},
	"link.file.destination.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Target))
	},
	"link.file.destination.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Target.FileFields))
	},
	"link.file.destination.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.PathKey.Inode))
	},
	"link.file.destination.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.Mode))
	},
	"link.file.destination.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.MTime))
	},
	"link.file.destination.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Target.MountDetached)
	},
	"link.file.destination.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.PathKey.MountID))
	},
	"link.file.destination.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Target.MountVisible)
	},
	"link.file.destination.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Target))
	},
	"link.file.destination.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Link.Target))
	},
	"link.file.destination.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Target))
	},
	"link.file.destination.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Link.Target))
	},
	"link.file.destination.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Link.Target))
	},
	"link.file.destination.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Link.Target))
	},
	"link.file.destination.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Target))
	},
	"link.file.destination.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Target))
	},
	"link.file.destination.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Target))
	},
	"link.file.destination.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Link.Target.FileFields)))
	},
	"link.file.destination.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Target.FileFields.UID))
	},
	"link.file.destination.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.destination.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Target.FileFields))
	},
	"link.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Link.Source))
	},
	"link.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Link.Source))
	},
	"link.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.GID))
	},
	"link.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Link.Source.FileFields))
	},
	"link.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Link.Source))
	},
	"link.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Link.Source.FileFields))
	},
	"link.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.PathKey.Inode))
	},
	"link.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.Mode))
	},
	"link.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.MTime))
	},
	"link.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Source.MountDetached)
	},
	"link.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.PathKey.MountID))
	},
	"link.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Link.Source.MountVisible)
	},
	"link.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Link.Source))
	},
	"link.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Link.Source))
	},
	"link.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Link.Source))
	},
	"link.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Link.Source))
	},
	"link.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Link.Source))
	},
	"link.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Link.Source))
	},
	"link.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Link.Source))
	},
	"link.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Link.Source))
	},
	"link.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Link.Source))
	},
	"link.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Link.Source.FileFields)))
	},
	"link.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.Source.FileFields.UID))
	},
	"link.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Link.Source.FileFields))
	},
	"link.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Link.SyscallEvent.Retval))
	},
	"link.syscall.destination.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.syscall.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Link.SyscallContext))
	},
	"link.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("link.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Link.SyscallContext))
	},
	"load_module.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveModuleArgs(ev, &ev.LoadModule))
	},
	"load_module.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.ArgsTruncated)
	},
	"load_module.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveModuleArgv(ev, &ev.LoadModule))
	},
	"load_module.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.CTime))
	},
	"load_module.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.LoadModule.File))
	},
	"load_module.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.LoadModule.File))
	},
	"load_module.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.GID))
	},
	"load_module.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.LoadModule.File.FileFields))
	},
	"load_module.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.LoadModule.File))
	},
	"load_module.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.LoadModule.File.FileFields))
	},
	"load_module.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.PathKey.Inode))
	},
	"load_module.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.Mode))
	},
	"load_module.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.MTime))
	},
	"load_module.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.File.MountDetached)
	},
	"load_module.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.PathKey.MountID))
	},
	"load_module.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.File.MountVisible)
	},
	"load_module.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.LoadModule.File))
	},
	"load_module.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.LoadModule.File))
	},
	"load_module.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.LoadModule.File))
	},
	"load_module.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.LoadModule.File.FileFields)))
	},
	"load_module.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.File.FileFields.UID))
	},
	"load_module.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.LoadModule.File.FileFields))
	},
	"load_module.loaded_from_memory": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.loaded_from_memory")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.LoadModule.LoadedFromMemory)
	},
	"load_module.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.LoadModule.Name)
	},
	"load_module.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("load_module.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.LoadModule.SyscallEvent.Retval))
	},
	"mkdir.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.CTime))
	},
	"mkdir.file.destination.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.Mode))
	},
	"mkdir.file.destination.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.Mode))
	},
	"mkdir.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Mkdir.File))
	},
	"mkdir.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Mkdir.File))
	},
	"mkdir.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.GID))
	},
	"mkdir.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Mkdir.File.FileFields))
	},
	"mkdir.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Mkdir.File))
	},
	"mkdir.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Mkdir.File.FileFields))
	},
	"mkdir.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.PathKey.Inode))
	},
	"mkdir.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.Mode))
	},
	"mkdir.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.MTime))
	},
	"mkdir.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mkdir.File.MountDetached)
	},
	"mkdir.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.PathKey.MountID))
	},
	"mkdir.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mkdir.File.MountVisible)
	},
	"mkdir.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Mkdir.File))
	},
	"mkdir.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Mkdir.File))
	},
	"mkdir.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Mkdir.File))
	},
	"mkdir.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Mkdir.File.FileFields)))
	},
	"mkdir.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.File.FileFields.UID))
	},
	"mkdir.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Mkdir.File.FileFields))
	},
	"mkdir.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mkdir.SyscallEvent.Retval))
	},
	"mkdir.syscall.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.syscall.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Mkdir.SyscallContext)))
	},
	"mkdir.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mkdir.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mkdir.SyscallContext))
	},
	"mmap.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.CTime))
	},
	"mmap.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.MMap.File))
	},
	"mmap.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.MMap.File))
	},
	"mmap.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.GID))
	},
	"mmap.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.MMap.File.FileFields))
	},
	"mmap.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.MMap.File))
	},
	"mmap.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.MMap.File.FileFields))
	},
	"mmap.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.PathKey.Inode))
	},
	"mmap.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.Mode))
	},
	"mmap.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.MTime))
	},
	"mmap.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.MMap.File.MountDetached)
	},
	"mmap.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.PathKey.MountID))
	},
	"mmap.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.MMap.File.MountVisible)
	},
	"mmap.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.MMap.File))
	},
	"mmap.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.MMap.File))
	},
	"mmap.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.MMap.File))
	},
	"mmap.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.MMap.File))
	},
	"mmap.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.MMap.File))
	},
	"mmap.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.MMap.File))
	},
	"mmap.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.MMap.File))
	},
	"mmap.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.MMap.File))
	},
	"mmap.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.MMap.File))
	},
	"mmap.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.MMap.File.FileFields)))
	},
	"mmap.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.File.FileFields.UID))
	},
	"mmap.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.MMap.File.FileFields))
	},
	"mmap.flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.Flags))
	},
	"mmap.protection": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.protection")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.Protection))
	},
	"mmap.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mmap.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MMap.SyscallEvent.Retval))
	},
	"mount.detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mount.Mount.Detached)
	},
	"mount.fs_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.fs_type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Mount.Mount.FSType)
	},
	"mount.mountpoint.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.mountpoint.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveMountPointPath(ev, &ev.Mount))
	},
	"mount.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Mount.SyscallEvent.Retval))
	},
	"mount.root.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.root.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveMountRootPath(ev, &ev.Mount))
	},
	"mount.source.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.source.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveMountSourcePath(ev, &ev.Mount))
	},
	"mount.syscall.fs_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.syscall.fs_type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr3(ev, &ev.Mount.SyscallContext))
	},
	"mount.syscall.mountpoint.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.syscall.mountpoint.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Mount.SyscallContext))
	},
	"mount.syscall.source.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.syscall.source.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Mount.SyscallContext))
	},
	"mount.visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mount.visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Mount.Mount.Visible)
	},
	"mprotect.req_protection": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mprotect.req_protection")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.MProtect.ReqProtection)
	},
	"mprotect.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mprotect.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.MProtect.SyscallEvent.Retval))
	},
	"mprotect.vm_protection": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("mprotect.vm_protection")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.MProtect.VMProtection)
	},
	"network.destination.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.destination.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.NetworkContext.Destination.IPNet)
	},
	"network.destination.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.destination.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.NetworkContext.Destination))
	},
	"network.destination.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.destination.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Destination.Port))
	},
	"network.device.ifname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.device.ifname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkContext.Device))
	},
	"network.device.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.device.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Device.NetNS))
	},
	"network.l3_protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.l3_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.L3Protocol))
	},
	"network.l4_protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.l4_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.L4Protocol))
	},
	"network.network_direction": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.network_direction")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.NetworkDirection))
	},
	"network.size": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.size")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Size))
	},
	"network.source.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.source.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.NetworkContext.Source.IPNet)
	},
	"network.source.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.source.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.NetworkContext.Source))
	},
	"network.source.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.source.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Source.Port))
	},
	"network.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkContext.Type))
	},
	"network_flow_monitor.device.ifname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.device.ifname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.NetworkFlowMonitor.Device))
	},
	"network_flow_monitor.device.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.device.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.NetworkFlowMonitor.Device.NetNS))
	},
	"network_flow_monitor.flows.destination.ip": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.destination.ip")
		element := *(e.(*model.Flow))
		return cidrToVal(element.Destination.IPNet)
	},
	"network_flow_monitor.flows.destination.is_public": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.destination.is_public")
		element := *(e.(*model.Flow))
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &element.Destination))
	},
	"network_flow_monitor.flows.destination.port": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.destination.port")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Destination.Port))
	},
	"network_flow_monitor.flows.egress.data_size": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.egress.data_size")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Egress.DataSize))
	},
	"network_flow_monitor.flows.egress.packet_count": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.egress.packet_count")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Egress.PacketCount))
	},
	"network_flow_monitor.flows.ingress.data_size": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.ingress.data_size")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Ingress.DataSize))
	},
	"network_flow_monitor.flows.ingress.packet_count": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.ingress.packet_count")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Ingress.PacketCount))
	},
	"network_flow_monitor.flows.l3_protocol": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.l3_protocol")
		element := *(e.(*model.Flow))
		return types.Int(int(element.L3Protocol))
	},
	"network_flow_monitor.flows.l4_protocol": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.l4_protocol")
		element := *(e.(*model.Flow))
		return types.Int(int(element.L4Protocol))
	},
	"network_flow_monitor.flows.source.ip": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.source.ip")
		element := *(e.(*model.Flow))
		return cidrToVal(element.Source.IPNet)
	},
	"network_flow_monitor.flows.source.is_public": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.source.is_public")
		element := *(e.(*model.Flow))
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &element.Source))
	},
	"network_flow_monitor.flows.source.port": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("network_flow_monitor.flows.source.port")
		element := *(e.(*model.Flow))
		return types.Int(int(element.Source.Port))
	},
	"ondemand.arg1.str": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg1.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg1Str(ev, &ev.OnDemand))
	},
	"ondemand.arg1.uint": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg1.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg1Uint(ev, &ev.OnDemand)))
	},
	"ondemand.arg2.str": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg2.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg2Str(ev, &ev.OnDemand))
	},
	"ondemand.arg2.uint": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg2.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg2Uint(ev, &ev.OnDemand)))
	},
	"ondemand.arg3.str": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg3.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg3Str(ev, &ev.OnDemand))
	},
	"ondemand.arg3.uint": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg3.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg3Uint(ev, &ev.OnDemand)))
	},
	"ondemand.arg4.str": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg4.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg4Str(ev, &ev.OnDemand))
	},
	"ondemand.arg4.uint": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg4.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg4Uint(ev, &ev.OnDemand)))
	},
	"ondemand.arg5.str": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg5.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg5Str(ev, &ev.OnDemand))
	},
	"ondemand.arg5.uint": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg5.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg5Uint(ev, &ev.OnDemand)))
	},
	"ondemand.arg6.str": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg6.str")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandArg6Str(ev, &ev.OnDemand))
	},
	"ondemand.arg6.uint": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.arg6.uint")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveOnDemandArg6Uint(ev, &ev.OnDemand)))
	},
	"ondemand.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ondemand.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOnDemandName(ev, &ev.OnDemand))
	},
	"open.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.CTime))
	},
	"open.file.destination.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.Mode))
	},
	"open.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Open.File))
	},
	"open.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Open.File))
	},
	"open.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.GID))
	},
	"open.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Open.File.FileFields))
	},
	"open.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Open.File))
	},
	"open.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Open.File.FileFields))
	},
	"open.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.PathKey.Inode))
	},
	"open.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.Mode))
	},
	"open.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.MTime))
	},
	"open.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Open.File.MountDetached)
	},
	"open.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.PathKey.MountID))
	},
	"open.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Open.File.MountVisible)
	},
	"open.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Open.File))
	},
	"open.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Open.File))
	},
	"open.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Open.File))
	},
	"open.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Open.File))
	},
	"open.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Open.File))
	},
	"open.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Open.File))
	},
	"open.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Open.File))
	},
	"open.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Open.File))
	},
	"open.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Open.File))
	},
	"open.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Open.File.FileFields)))
	},
	"open.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.File.FileFields.UID))
	},
	"open.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Open.File.FileFields))
	},
	"open.flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.Flags))
	},
	"open.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Open.SyscallEvent.Retval))
	},
	"open.syscall.flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.syscall.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt2(ev, &ev.Open.SyscallContext)))
	},
	"open.syscall.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.syscall.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Open.SyscallContext)))
	},
	"open.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Open.SyscallContext))
	},
	"packet.destination.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.destination.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.RawPacket.NetworkContext.Destination.IPNet)
	},
	"packet.destination.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.destination.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.RawPacket.NetworkContext.Destination))
	},
	"packet.destination.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.destination.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Destination.Port))
	},
	"packet.device.ifname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.device.ifname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNetworkDeviceIfName(ev, &ev.RawPacket.NetworkContext.Device))
	},
	"packet.device.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.device.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Device.NetNS))
	},
	"packet.filter": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.filter")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.RawPacket.Filter)
	},
	"packet.l3_protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.l3_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.L3Protocol))
	},
	"packet.l4_protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.l4_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.L4Protocol))
	},
	"packet.network_direction": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.network_direction")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.NetworkDirection))
	},
	"packet.size": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.size")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Size))
	},
	"packet.source.ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.source.ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.RawPacket.NetworkContext.Source.IPNet)
	},
	"packet.source.is_public": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.source.is_public")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveIsIPPublic(ev, &ev.RawPacket.NetworkContext.Source))
	},
	"packet.source.port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.source.port")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Source.Port))
	},
	"packet.tls.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.tls.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.TLSContext.Version))
	},
	"packet.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("packet.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RawPacket.NetworkContext.Type))
	},
	"prctl.is_name_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.is_name_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.PrCtl.IsNameTruncated)
	},
	"prctl.new_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.new_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PrCtl.NewName)
	},
	"prctl.option": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.option")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.PrCtl.Option)
	},
	"prctl.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("prctl.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PrCtl.SyscallEvent.Retval))
	},
	"process.ancestors.args": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.args_flags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.args_options": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.args_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.argv": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.argv0": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.auid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	"process.ancestors.cap_effective": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	"process.ancestors.cap_permitted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	"process.ancestors.caps_attempted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	"process.ancestors.caps_used": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	"process.ancestors.cgroup.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	"process.ancestors.cgroup.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	"process.ancestors.cgroup.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	"process.ancestors.cgroup.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	"process.ancestors.cgroup.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	"process.ancestors.comm": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	"process.ancestors.container.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	"process.ancestors.container.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	"process.ancestors.container.tags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	"process.ancestors.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	"process.ancestors.egid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	"process.ancestors.egroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	"process.ancestors.envp": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.envs": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.envs_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.euid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	"process.ancestors.euser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	"process.ancestors.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	"process.ancestors.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	"process.ancestors.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"process.ancestors.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"process.ancestors.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"process.ancestors.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	"process.ancestors.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	"process.ancestors.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	"process.ancestors.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"process.ancestors.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	"process.ancestors.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"process.ancestors.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"process.ancestors.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	"process.ancestors.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	"process.ancestors.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"process.ancestors.fsgid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	"process.ancestors.fsgroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	"process.ancestors.fsuid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	"process.ancestors.fsuser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	"process.ancestors.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	"process.ancestors.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	"process.ancestors.interpreter.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"process.ancestors.interpreter.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"process.ancestors.interpreter.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"process.ancestors.interpreter.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"process.ancestors.interpreter.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"process.ancestors.interpreter.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"process.ancestors.interpreter.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"process.ancestors.interpreter.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"process.ancestors.interpreter.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"process.ancestors.interpreter.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"process.ancestors.interpreter.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"process.ancestors.interpreter.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"process.ancestors.interpreter.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.ancestors.interpreter.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"process.ancestors.interpreter.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"process.ancestors.interpreter.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"process.ancestors.is_exec": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	"process.ancestors.is_kworker": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	"process.ancestors.is_thread": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.mntns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	"process.ancestors.netns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	"process.ancestors.pid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	"process.ancestors.ppid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	"process.ancestors.sid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	"process.ancestors.tid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	"process.ancestors.tty_name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	"process.ancestors.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	"process.ancestors.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	"process.ancestors.user_session.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	"process.ancestors.user_session.identity": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	"process.ancestors.user_session.k8s_groups": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"process.ancestors.user_session.k8s_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"process.ancestors.user_session.k8s_uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"process.ancestors.user_session.k8s_username": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"process.ancestors.user_session.session_type": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	"process.ancestors.user_session.ssh_auth_method": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	"process.ancestors.user_session.ssh_client_ip": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"process.ancestors.user_session.ssh_client_port": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	"process.ancestors.user_session.ssh_public_key": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"process.ancestors.user_session.ssh_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"process.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.AUID))
	},
	"process.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.CapEffective))
	},
	"process.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.CapPermitted))
	},
	"process.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CapsAttempted))
	},
	"process.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CapsUsed))
	},
	"process.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CGroup.CreatedAt))
	},
	"process.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	"process.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	"process.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.BaseEvent.ProcessContext.Process.CGroup.CGroupID))
	},
	"process.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.BaseEvent.ProcessContext.Process.CGroup))
	},
	"process.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Comm)
	},
	"process.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	"process.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.BaseEvent.ProcessContext.Process.ContainerContext.ContainerID))
	},
	"process.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Process.ContainerContext))
	},
	"process.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)))
	},
	"process.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.EGID))
	},
	"process.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.EGroup)
	},
	"process.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.EUID))
	},
	"process.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.EUser)
	},
	"process.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	"process.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	"process.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
	},
	"process.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
	},
	"process.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"process.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	"process.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	"process.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.FileEvent.MountDetached)
	},
	"process.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"process.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.FileEvent.MountVisible)
	},
	"process.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields)))
	},
	"process.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	"process.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent.FileFields))
	},
	"process.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.FSGID))
	},
	"process.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.FSGroup)
	},
	"process.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.FSUID))
	},
	"process.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.FSUser)
	},
	"process.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.GID))
	},
	"process.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.Group)
	},
	"process.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"process.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"process.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"process.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"process.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"process.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"process.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"process.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"process.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"process.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"process.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"process.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"process.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"process.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.BaseEvent.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"process.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.BaseEvent.ProcessContext.Process.IsExec)
	},
	"process.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.BaseEvent.ProcessContext.Process.PIDContext.IsKworker)
	},
	"process.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.MntNS))
	},
	"process.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.NetNS))
	},
	"process.parent.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.AUID))
	},
	"process.parent.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.CapEffective))
	},
	"process.parent.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.CapPermitted))
	},
	"process.parent.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CapsAttempted))
	},
	"process.parent.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CapsUsed))
	},
	"process.parent.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CGroup.CreatedAt))
	},
	"process.parent.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupPathKey.Inode))
	},
	"process.parent.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupPathKey.MountID))
	},
	"process.parent.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.BaseEvent.ProcessContext.Parent.CGroup.CGroupID))
	},
	"process.parent.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.BaseEvent.ProcessContext.Parent.CGroup))
	},
	"process.parent.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Comm)
	},
	"process.parent.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.ContainerContext.CreatedAt))
	},
	"process.parent.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.BaseEvent.ProcessContext.Parent.ContainerContext.ContainerID))
	},
	"process.parent.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Parent.ContainerContext))
	},
	"process.parent.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)))
	},
	"process.parent.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.EGID))
	},
	"process.parent.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.EGroup)
	},
	"process.parent.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.EUID))
	},
	"process.parent.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.EUser)
	},
	"process.parent.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.FSGID))
	},
	"process.parent.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.FSGroup)
	},
	"process.parent.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.FSUID))
	},
	"process.parent.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.FSUser)
	},
	"process.parent.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.GID))
	},
	"process.parent.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.Group)
	},
	"process.parent.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"process.parent.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.IsExec)
	},
	"process.parent.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.BaseEvent.ProcessContext.Parent.PIDContext.IsKworker)
	},
	"process.parent.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.MntNS))
	},
	"process.parent.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.NetNS))
	},
	"process.parent.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid))
	},
	"process.parent.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PPid))
	},
	"process.parent.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.SID))
	},
	"process.parent.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Tid))
	},
	"process.parent.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.TTYName)
	},
	"process.parent.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.Credentials.UID))
	},
	"process.parent.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.Credentials.User)
	},
	"process.parent.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession))
	},
	"process.parent.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession))
	},
	"process.parent.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext))
	},
	"process.parent.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	"process.parent.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext))
	},
	"process.parent.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession.K8SSessionContext))
	},
	"process.parent.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.BaseEvent.ProcessContext.Parent.UserSession))
	},
	"process.parent.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"process.parent.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	"process.parent.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	"process.parent.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"process.parent.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	"process.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid))
	},
	"process.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PPid))
	},
	"process.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.SID))
	},
	"process.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.Tid))
	},
	"process.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.TTYName)
	},
	"process.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.Credentials.UID))
	},
	"process.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.Credentials.User)
	},
	"process.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.BaseEvent.ProcessContext.Process.UserSession))
	},
	"process.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.BaseEvent.ProcessContext.Process.UserSession))
	},
	"process.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"process.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"process.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"process.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.BaseEvent.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"process.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.BaseEvent.ProcessContext.Process.UserSession))
	},
	"process.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"process.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"process.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	"process.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"process.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"ptrace.request": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.request")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Request))
	},
	"ptrace.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.SyscallEvent.Retval))
	},
	"ptrace.tracee.ancestors.args": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.args_flags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.args_options": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.args_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.argv": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.argv0": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.auid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	"ptrace.tracee.ancestors.cap_effective": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	"ptrace.tracee.ancestors.cap_permitted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	"ptrace.tracee.ancestors.caps_attempted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	"ptrace.tracee.ancestors.caps_used": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	"ptrace.tracee.ancestors.cgroup.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	"ptrace.tracee.ancestors.cgroup.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	"ptrace.tracee.ancestors.cgroup.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	"ptrace.tracee.ancestors.cgroup.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	"ptrace.tracee.ancestors.cgroup.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	"ptrace.tracee.ancestors.comm": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	"ptrace.tracee.ancestors.container.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	"ptrace.tracee.ancestors.container.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	"ptrace.tracee.ancestors.container.tags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	"ptrace.tracee.ancestors.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	"ptrace.tracee.ancestors.egid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	"ptrace.tracee.ancestors.egroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	"ptrace.tracee.ancestors.envp": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.envs": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.envs_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.euid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	"ptrace.tracee.ancestors.euser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	"ptrace.tracee.ancestors.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	"ptrace.tracee.ancestors.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	"ptrace.tracee.ancestors.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"ptrace.tracee.ancestors.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"ptrace.tracee.ancestors.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"ptrace.tracee.ancestors.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	"ptrace.tracee.ancestors.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	"ptrace.tracee.ancestors.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	"ptrace.tracee.ancestors.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"ptrace.tracee.ancestors.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	"ptrace.tracee.ancestors.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"ptrace.tracee.ancestors.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"ptrace.tracee.ancestors.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	"ptrace.tracee.ancestors.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	"ptrace.tracee.ancestors.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	"ptrace.tracee.ancestors.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"ptrace.tracee.ancestors.fsgid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	"ptrace.tracee.ancestors.fsgroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	"ptrace.tracee.ancestors.fsuid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	"ptrace.tracee.ancestors.fsuser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	"ptrace.tracee.ancestors.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	"ptrace.tracee.ancestors.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	"ptrace.tracee.ancestors.interpreter.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"ptrace.tracee.ancestors.interpreter.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"ptrace.tracee.ancestors.interpreter.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"ptrace.tracee.ancestors.interpreter.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"ptrace.tracee.ancestors.interpreter.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"ptrace.tracee.ancestors.interpreter.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"ptrace.tracee.ancestors.interpreter.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"ptrace.tracee.ancestors.interpreter.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"ptrace.tracee.ancestors.interpreter.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"ptrace.tracee.ancestors.interpreter.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"ptrace.tracee.ancestors.interpreter.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.ancestors.interpreter.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"ptrace.tracee.ancestors.interpreter.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"ptrace.tracee.ancestors.interpreter.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"ptrace.tracee.ancestors.is_exec": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	"ptrace.tracee.ancestors.is_kworker": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	"ptrace.tracee.ancestors.is_thread": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	"ptrace.tracee.ancestors.mntns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	"ptrace.tracee.ancestors.netns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	"ptrace.tracee.ancestors.pid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	"ptrace.tracee.ancestors.ppid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	"ptrace.tracee.ancestors.sid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	"ptrace.tracee.ancestors.tid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	"ptrace.tracee.ancestors.tty_name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	"ptrace.tracee.ancestors.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	"ptrace.tracee.ancestors.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	"ptrace.tracee.ancestors.user_session.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	"ptrace.tracee.ancestors.user_session.identity": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	"ptrace.tracee.ancestors.user_session.k8s_groups": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.ancestors.user_session.k8s_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"ptrace.tracee.ancestors.user_session.k8s_uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.ancestors.user_session.k8s_username": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.ancestors.user_session.session_type": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	"ptrace.tracee.ancestors.user_session.ssh_auth_method": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	"ptrace.tracee.ancestors.user_session.ssh_client_ip": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"ptrace.tracee.ancestors.user_session.ssh_client_port": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	"ptrace.tracee.ancestors.user_session.ssh_public_key": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"ptrace.tracee.ancestors.user_session.ssh_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"ptrace.tracee.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.AUID))
	},
	"ptrace.tracee.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.CapEffective))
	},
	"ptrace.tracee.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.CapPermitted))
	},
	"ptrace.tracee.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CapsAttempted))
	},
	"ptrace.tracee.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CapsUsed))
	},
	"ptrace.tracee.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CGroup.CreatedAt))
	},
	"ptrace.tracee.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CGroup.CGroupPathKey.Inode))
	},
	"ptrace.tracee.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.CGroup.CGroupPathKey.MountID))
	},
	"ptrace.tracee.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.PTrace.Tracee.Process.CGroup.CGroupID))
	},
	"ptrace.tracee.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.PTrace.Tracee.Process.CGroup))
	},
	"ptrace.tracee.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Comm)
	},
	"ptrace.tracee.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.ContainerContext.CreatedAt))
	},
	"ptrace.tracee.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.PTrace.Tracee.Process.ContainerContext.ContainerID))
	},
	"ptrace.tracee.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.PTrace.Tracee.Process.ContainerContext))
	},
	"ptrace.tracee.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.PTrace.Tracee.Process)))
	},
	"ptrace.tracee.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.EGID))
	},
	"ptrace.tracee.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.EGroup)
	},
	"ptrace.tracee.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.EUID))
	},
	"ptrace.tracee.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.EUser)
	},
	"ptrace.tracee.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.CTime))
	},
	"ptrace.tracee.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.GID))
	},
	"ptrace.tracee.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
	},
	"ptrace.tracee.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
	},
	"ptrace.tracee.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"ptrace.tracee.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.Mode))
	},
	"ptrace.tracee.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.MTime))
	},
	"ptrace.tracee.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.FileEvent.MountDetached)
	},
	"ptrace.tracee.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"ptrace.tracee.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.FileEvent.MountVisible)
	},
	"ptrace.tracee.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.FileEvent))
	},
	"ptrace.tracee.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields)))
	},
	"ptrace.tracee.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.FileEvent.FileFields.UID))
	},
	"ptrace.tracee.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.FileEvent.FileFields))
	},
	"ptrace.tracee.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.FSGID))
	},
	"ptrace.tracee.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.FSGroup)
	},
	"ptrace.tracee.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.FSUID))
	},
	"ptrace.tracee.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.FSUser)
	},
	"ptrace.tracee.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.GID))
	},
	"ptrace.tracee.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.Group)
	},
	"ptrace.tracee.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"ptrace.tracee.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"ptrace.tracee.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"ptrace.tracee.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"ptrace.tracee.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"ptrace.tracee.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"ptrace.tracee.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"ptrace.tracee.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"ptrace.tracee.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"ptrace.tracee.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"ptrace.tracee.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent))
	},
	"ptrace.tracee.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"ptrace.tracee.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"ptrace.tracee.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.PTrace.Tracee.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"ptrace.tracee.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.PTrace.Tracee.Process.IsExec)
	},
	"ptrace.tracee.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.PTrace.Tracee.Process.PIDContext.IsKworker)
	},
	"ptrace.tracee.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.PTrace.Tracee.Process))
	},
	"ptrace.tracee.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.MntNS))
	},
	"ptrace.tracee.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.NetNS))
	},
	"ptrace.tracee.parent.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.AUID))
	},
	"ptrace.tracee.parent.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.CapEffective))
	},
	"ptrace.tracee.parent.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.CapPermitted))
	},
	"ptrace.tracee.parent.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CapsAttempted))
	},
	"ptrace.tracee.parent.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CapsUsed))
	},
	"ptrace.tracee.parent.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CGroup.CreatedAt))
	},
	"ptrace.tracee.parent.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CGroup.CGroupPathKey.Inode))
	},
	"ptrace.tracee.parent.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.CGroup.CGroupPathKey.MountID))
	},
	"ptrace.tracee.parent.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.PTrace.Tracee.Parent.CGroup.CGroupID))
	},
	"ptrace.tracee.parent.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.PTrace.Tracee.Parent.CGroup))
	},
	"ptrace.tracee.parent.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Comm)
	},
	"ptrace.tracee.parent.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.ContainerContext.CreatedAt))
	},
	"ptrace.tracee.parent.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.PTrace.Tracee.Parent.ContainerContext.ContainerID))
	},
	"ptrace.tracee.parent.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.PTrace.Tracee.Parent.ContainerContext))
	},
	"ptrace.tracee.parent.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.PTrace.Tracee.Parent)))
	},
	"ptrace.tracee.parent.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.EGID))
	},
	"ptrace.tracee.parent.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.EGroup)
	},
	"ptrace.tracee.parent.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.EUID))
	},
	"ptrace.tracee.parent.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.EUser)
	},
	"ptrace.tracee.parent.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.FSGID))
	},
	"ptrace.tracee.parent.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.FSGroup)
	},
	"ptrace.tracee.parent.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.FSUID))
	},
	"ptrace.tracee.parent.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.FSUser)
	},
	"ptrace.tracee.parent.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.GID))
	},
	"ptrace.tracee.parent.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.Group)
	},
	"ptrace.tracee.parent.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"ptrace.tracee.parent.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.IsExec)
	},
	"ptrace.tracee.parent.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.PTrace.Tracee.Parent.PIDContext.IsKworker)
	},
	"ptrace.tracee.parent.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.PTrace.Tracee.Parent))
	},
	"ptrace.tracee.parent.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.MntNS))
	},
	"ptrace.tracee.parent.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.NetNS))
	},
	"ptrace.tracee.parent.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.Pid))
	},
	"ptrace.tracee.parent.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PPid))
	},
	"ptrace.tracee.parent.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.SID))
	},
	"ptrace.tracee.parent.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.PIDContext.Tid))
	},
	"ptrace.tracee.parent.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.TTYName)
	},
	"ptrace.tracee.parent.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.Credentials.UID))
	},
	"ptrace.tracee.parent.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.Credentials.User)
	},
	"ptrace.tracee.parent.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.PTrace.Tracee.Parent.UserSession))
	},
	"ptrace.tracee.parent.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.PTrace.Tracee.Parent.UserSession))
	},
	"ptrace.tracee.parent.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.parent.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	"ptrace.tracee.parent.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.parent.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Parent.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.parent.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.PTrace.Tracee.Parent.UserSession))
	},
	"ptrace.tracee.parent.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"ptrace.tracee.parent.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	"ptrace.tracee.parent.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	"ptrace.tracee.parent.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.String("")
		}
		return types.String(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"ptrace.tracee.parent.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.PTrace.Tracee.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.PTrace.Tracee.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	"ptrace.tracee.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.Pid))
	},
	"ptrace.tracee.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PPid))
	},
	"ptrace.tracee.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.SID))
	},
	"ptrace.tracee.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.PIDContext.Tid))
	},
	"ptrace.tracee.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.TTYName)
	},
	"ptrace.tracee.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.Credentials.UID))
	},
	"ptrace.tracee.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.Credentials.User)
	},
	"ptrace.tracee.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.PTrace.Tracee.Process.UserSession))
	},
	"ptrace.tracee.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.PTrace.Tracee.Process.UserSession))
	},
	"ptrace.tracee.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.PTrace.Tracee.Process.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"ptrace.tracee.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.PTrace.Tracee.Process.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.PTrace.Tracee.Process.UserSession.K8SSessionContext))
	},
	"ptrace.tracee.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.PTrace.Tracee.Process.UserSession))
	},
	"ptrace.tracee.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"ptrace.tracee.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"ptrace.tracee.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	"ptrace.tracee.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"ptrace.tracee.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("ptrace.tracee.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.PTrace.Tracee.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"removexattr.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.CTime))
	},
	"removexattr.file.destination.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrName(ev, &ev.RemoveXAttr))
	},
	"removexattr.file.destination.namespace": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.destination.namespace")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.RemoveXAttr))
	},
	"removexattr.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.GID))
	},
	"removexattr.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.RemoveXAttr.File.FileFields))
	},
	"removexattr.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.RemoveXAttr.File.FileFields))
	},
	"removexattr.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.PathKey.Inode))
	},
	"removexattr.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.Mode))
	},
	"removexattr.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.MTime))
	},
	"removexattr.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.RemoveXAttr.File.MountDetached)
	},
	"removexattr.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.PathKey.MountID))
	},
	"removexattr.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.RemoveXAttr.File.MountVisible)
	},
	"removexattr.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.RemoveXAttr.File))
	},
	"removexattr.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.RemoveXAttr.File.FileFields)))
	},
	"removexattr.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.File.FileFields.UID))
	},
	"removexattr.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.RemoveXAttr.File.FileFields))
	},
	"removexattr.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("removexattr.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.RemoveXAttr.SyscallEvent.Retval))
	},
	"rename.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.CTime))
	},
	"rename.file.destination.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.CTime))
	},
	"rename.file.destination.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Rename.New))
	},
	"rename.file.destination.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.New))
	},
	"rename.file.destination.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.GID))
	},
	"rename.file.destination.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.New.FileFields))
	},
	"rename.file.destination.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.New))
	},
	"rename.file.destination.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.New.FileFields))
	},
	"rename.file.destination.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.PathKey.Inode))
	},
	"rename.file.destination.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.Mode))
	},
	"rename.file.destination.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.MTime))
	},
	"rename.file.destination.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.New.MountDetached)
	},
	"rename.file.destination.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.PathKey.MountID))
	},
	"rename.file.destination.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.New.MountVisible)
	},
	"rename.file.destination.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.New))
	},
	"rename.file.destination.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.New))
	},
	"rename.file.destination.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.New))
	},
	"rename.file.destination.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Rename.New.FileFields)))
	},
	"rename.file.destination.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.New.FileFields.UID))
	},
	"rename.file.destination.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.New.FileFields))
	},
	"rename.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Rename.Old))
	},
	"rename.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rename.Old))
	},
	"rename.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.GID))
	},
	"rename.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rename.Old.FileFields))
	},
	"rename.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rename.Old))
	},
	"rename.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rename.Old.FileFields))
	},
	"rename.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.PathKey.Inode))
	},
	"rename.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.Mode))
	},
	"rename.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.MTime))
	},
	"rename.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.Old.MountDetached)
	},
	"rename.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.PathKey.MountID))
	},
	"rename.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rename.Old.MountVisible)
	},
	"rename.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rename.Old))
	},
	"rename.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Rename.Old))
	},
	"rename.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Rename.Old))
	},
	"rename.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Rename.Old))
	},
	"rename.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Rename.Old))
	},
	"rename.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Rename.Old))
	},
	"rename.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rename.Old))
	},
	"rename.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rename.Old))
	},
	"rename.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rename.Old))
	},
	"rename.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Rename.Old.FileFields)))
	},
	"rename.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.Old.FileFields.UID))
	},
	"rename.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rename.Old.FileFields))
	},
	"rename.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rename.SyscallEvent.Retval))
	},
	"rename.syscall.destination.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.syscall.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Rename.SyscallContext))
	},
	"rename.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rename.SyscallContext))
	},
	"rmdir.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.CTime))
	},
	"rmdir.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Rmdir.File))
	},
	"rmdir.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Rmdir.File))
	},
	"rmdir.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.GID))
	},
	"rmdir.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Rmdir.File.FileFields))
	},
	"rmdir.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Rmdir.File))
	},
	"rmdir.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Rmdir.File.FileFields))
	},
	"rmdir.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.PathKey.Inode))
	},
	"rmdir.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.Mode))
	},
	"rmdir.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.MTime))
	},
	"rmdir.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rmdir.File.MountDetached)
	},
	"rmdir.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.PathKey.MountID))
	},
	"rmdir.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Rmdir.File.MountVisible)
	},
	"rmdir.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Rmdir.File))
	},
	"rmdir.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Rmdir.File))
	},
	"rmdir.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Rmdir.File))
	},
	"rmdir.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Rmdir.File.FileFields)))
	},
	"rmdir.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.File.FileFields.UID))
	},
	"rmdir.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Rmdir.File.FileFields))
	},
	"rmdir.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Rmdir.SyscallEvent.Retval))
	},
	"rmdir.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rmdir.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Rmdir.SyscallContext))
	},
	"selinux.bool.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.bool.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSELinuxBoolName(ev, &ev.SELinux))
	},
	"selinux.bool.state": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.bool.state")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SELinux.BoolChangeValue)
	},
	"selinux.bool_commit.state": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.bool_commit.state")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SELinux.BoolCommitValue)
	},
	"selinux.enforce.status": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("selinux.enforce.status")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SELinux.EnforceStatus)
	},
	"setgid.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetGID.EGID))
	},
	"setgid.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetgidEGroup(ev, &ev.SetGID))
	},
	"setgid.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetGID.FSGID))
	},
	"setgid.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetgidFSGroup(ev, &ev.SetGID))
	},
	"setgid.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetGID.GID))
	},
	"setgid.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setgid.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetgidGroup(ev, &ev.SetGID))
	},
	"setrlimit.resource": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.resource")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Setrlimit.Resource)
	},
	"setrlimit.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.SyscallEvent.Retval))
	},
	"setrlimit.rlim_cur": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.rlim_cur")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.RlimCur))
	},
	"setrlimit.rlim_max": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.rlim_max")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.RlimMax))
	},
	"setrlimit.target.ancestors.args": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.args_flags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.args_options": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.args_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.argv": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.argv0": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.auid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	"setrlimit.target.ancestors.cap_effective": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	"setrlimit.target.ancestors.cap_permitted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	"setrlimit.target.ancestors.caps_attempted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	"setrlimit.target.ancestors.caps_used": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	"setrlimit.target.ancestors.cgroup.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	"setrlimit.target.ancestors.cgroup.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	"setrlimit.target.ancestors.cgroup.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	"setrlimit.target.ancestors.cgroup.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	"setrlimit.target.ancestors.cgroup.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	"setrlimit.target.ancestors.comm": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	"setrlimit.target.ancestors.container.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	"setrlimit.target.ancestors.container.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	"setrlimit.target.ancestors.container.tags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	"setrlimit.target.ancestors.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	"setrlimit.target.ancestors.egid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	"setrlimit.target.ancestors.egroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	"setrlimit.target.ancestors.envp": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.envs": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.envs_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.euid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	"setrlimit.target.ancestors.euser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	"setrlimit.target.ancestors.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	"setrlimit.target.ancestors.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	"setrlimit.target.ancestors.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"setrlimit.target.ancestors.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"setrlimit.target.ancestors.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"setrlimit.target.ancestors.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	"setrlimit.target.ancestors.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	"setrlimit.target.ancestors.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	"setrlimit.target.ancestors.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"setrlimit.target.ancestors.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	"setrlimit.target.ancestors.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"setrlimit.target.ancestors.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"setrlimit.target.ancestors.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	"setrlimit.target.ancestors.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	"setrlimit.target.ancestors.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	"setrlimit.target.ancestors.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"setrlimit.target.ancestors.fsgid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	"setrlimit.target.ancestors.fsgroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	"setrlimit.target.ancestors.fsuid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	"setrlimit.target.ancestors.fsuser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	"setrlimit.target.ancestors.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	"setrlimit.target.ancestors.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	"setrlimit.target.ancestors.interpreter.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"setrlimit.target.ancestors.interpreter.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"setrlimit.target.ancestors.interpreter.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"setrlimit.target.ancestors.interpreter.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"setrlimit.target.ancestors.interpreter.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"setrlimit.target.ancestors.interpreter.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"setrlimit.target.ancestors.interpreter.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"setrlimit.target.ancestors.interpreter.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"setrlimit.target.ancestors.interpreter.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"setrlimit.target.ancestors.interpreter.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"setrlimit.target.ancestors.interpreter.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"setrlimit.target.ancestors.interpreter.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"setrlimit.target.ancestors.interpreter.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.ancestors.interpreter.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"setrlimit.target.ancestors.interpreter.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"setrlimit.target.ancestors.interpreter.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"setrlimit.target.ancestors.is_exec": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	"setrlimit.target.ancestors.is_kworker": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	"setrlimit.target.ancestors.is_thread": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	"setrlimit.target.ancestors.mntns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	"setrlimit.target.ancestors.netns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	"setrlimit.target.ancestors.pid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	"setrlimit.target.ancestors.ppid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	"setrlimit.target.ancestors.sid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	"setrlimit.target.ancestors.tid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	"setrlimit.target.ancestors.tty_name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	"setrlimit.target.ancestors.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	"setrlimit.target.ancestors.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	"setrlimit.target.ancestors.user_session.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	"setrlimit.target.ancestors.user_session.identity": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	"setrlimit.target.ancestors.user_session.k8s_groups": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"setrlimit.target.ancestors.user_session.k8s_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"setrlimit.target.ancestors.user_session.k8s_uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"setrlimit.target.ancestors.user_session.k8s_username": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"setrlimit.target.ancestors.user_session.session_type": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	"setrlimit.target.ancestors.user_session.ssh_auth_method": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	"setrlimit.target.ancestors.user_session.ssh_client_ip": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"setrlimit.target.ancestors.user_session.ssh_client_port": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	"setrlimit.target.ancestors.user_session.ssh_public_key": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"setrlimit.target.ancestors.user_session.ssh_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"setrlimit.target.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.AUID))
	},
	"setrlimit.target.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.CapEffective))
	},
	"setrlimit.target.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.CapPermitted))
	},
	"setrlimit.target.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CapsAttempted))
	},
	"setrlimit.target.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CapsUsed))
	},
	"setrlimit.target.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CGroup.CreatedAt))
	},
	"setrlimit.target.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CGroup.CGroupPathKey.Inode))
	},
	"setrlimit.target.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.CGroup.CGroupPathKey.MountID))
	},
	"setrlimit.target.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Setrlimit.Target.Process.CGroup.CGroupID))
	},
	"setrlimit.target.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Setrlimit.Target.Process.CGroup))
	},
	"setrlimit.target.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Comm)
	},
	"setrlimit.target.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.ContainerContext.CreatedAt))
	},
	"setrlimit.target.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Setrlimit.Target.Process.ContainerContext.ContainerID))
	},
	"setrlimit.target.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Setrlimit.Target.Process.ContainerContext))
	},
	"setrlimit.target.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Setrlimit.Target.Process)))
	},
	"setrlimit.target.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.EGID))
	},
	"setrlimit.target.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.EGroup)
	},
	"setrlimit.target.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.EUID))
	},
	"setrlimit.target.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.EUser)
	},
	"setrlimit.target.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.CTime))
	},
	"setrlimit.target.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.GID))
	},
	"setrlimit.target.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields))
	},
	"setrlimit.target.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields))
	},
	"setrlimit.target.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"setrlimit.target.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.Mode))
	},
	"setrlimit.target.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.MTime))
	},
	"setrlimit.target.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.FileEvent.MountDetached)
	},
	"setrlimit.target.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"setrlimit.target.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.FileEvent.MountVisible)
	},
	"setrlimit.target.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Setrlimit.Target.Process.FileEvent))
	},
	"setrlimit.target.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields)))
	},
	"setrlimit.target.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.FileEvent.FileFields.UID))
	},
	"setrlimit.target.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Setrlimit.Target.Process.FileEvent.FileFields))
	},
	"setrlimit.target.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.FSGID))
	},
	"setrlimit.target.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.FSGroup)
	},
	"setrlimit.target.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.FSUID))
	},
	"setrlimit.target.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.FSUser)
	},
	"setrlimit.target.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.GID))
	},
	"setrlimit.target.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.Group)
	},
	"setrlimit.target.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"setrlimit.target.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"setrlimit.target.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"setrlimit.target.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"setrlimit.target.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"setrlimit.target.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"setrlimit.target.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"setrlimit.target.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"setrlimit.target.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"setrlimit.target.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"setrlimit.target.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent))
	},
	"setrlimit.target.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"setrlimit.target.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"setrlimit.target.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Setrlimit.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"setrlimit.target.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Setrlimit.Target.Process.IsExec)
	},
	"setrlimit.target.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Setrlimit.Target.Process.PIDContext.IsKworker)
	},
	"setrlimit.target.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.Setrlimit.Target.Process))
	},
	"setrlimit.target.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.MntNS))
	},
	"setrlimit.target.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.NetNS))
	},
	"setrlimit.target.parent.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.AUID))
	},
	"setrlimit.target.parent.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.CapEffective))
	},
	"setrlimit.target.parent.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.CapPermitted))
	},
	"setrlimit.target.parent.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CapsAttempted))
	},
	"setrlimit.target.parent.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CapsUsed))
	},
	"setrlimit.target.parent.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CGroup.CreatedAt))
	},
	"setrlimit.target.parent.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CGroup.CGroupPathKey.Inode))
	},
	"setrlimit.target.parent.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.CGroup.CGroupPathKey.MountID))
	},
	"setrlimit.target.parent.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Setrlimit.Target.Parent.CGroup.CGroupID))
	},
	"setrlimit.target.parent.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Setrlimit.Target.Parent.CGroup))
	},
	"setrlimit.target.parent.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Comm)
	},
	"setrlimit.target.parent.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.ContainerContext.CreatedAt))
	},
	"setrlimit.target.parent.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Setrlimit.Target.Parent.ContainerContext.ContainerID))
	},
	"setrlimit.target.parent.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Setrlimit.Target.Parent.ContainerContext))
	},
	"setrlimit.target.parent.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Setrlimit.Target.Parent)))
	},
	"setrlimit.target.parent.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.EGID))
	},
	"setrlimit.target.parent.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.EGroup)
	},
	"setrlimit.target.parent.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.EUID))
	},
	"setrlimit.target.parent.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.EUser)
	},
	"setrlimit.target.parent.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.FSGID))
	},
	"setrlimit.target.parent.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.FSGroup)
	},
	"setrlimit.target.parent.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.FSUID))
	},
	"setrlimit.target.parent.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.FSUser)
	},
	"setrlimit.target.parent.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.GID))
	},
	"setrlimit.target.parent.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.Group)
	},
	"setrlimit.target.parent.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"setrlimit.target.parent.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.IsExec)
	},
	"setrlimit.target.parent.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Setrlimit.Target.Parent.PIDContext.IsKworker)
	},
	"setrlimit.target.parent.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Setrlimit.Target.Parent))
	},
	"setrlimit.target.parent.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.MntNS))
	},
	"setrlimit.target.parent.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.NetNS))
	},
	"setrlimit.target.parent.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.Pid))
	},
	"setrlimit.target.parent.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PPid))
	},
	"setrlimit.target.parent.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.SID))
	},
	"setrlimit.target.parent.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.PIDContext.Tid))
	},
	"setrlimit.target.parent.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.TTYName)
	},
	"setrlimit.target.parent.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.Credentials.UID))
	},
	"setrlimit.target.parent.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.Credentials.User)
	},
	"setrlimit.target.parent.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Setrlimit.Target.Parent.UserSession))
	},
	"setrlimit.target.parent.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Setrlimit.Target.Parent.UserSession))
	},
	"setrlimit.target.parent.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext))
	},
	"setrlimit.target.parent.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	"setrlimit.target.parent.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext))
	},
	"setrlimit.target.parent.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Setrlimit.Target.Parent.UserSession.K8SSessionContext))
	},
	"setrlimit.target.parent.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Setrlimit.Target.Parent.UserSession))
	},
	"setrlimit.target.parent.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"setrlimit.target.parent.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	"setrlimit.target.parent.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	"setrlimit.target.parent.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"setrlimit.target.parent.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Setrlimit.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Setrlimit.Target.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	"setrlimit.target.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.Pid))
	},
	"setrlimit.target.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PPid))
	},
	"setrlimit.target.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.SID))
	},
	"setrlimit.target.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.PIDContext.Tid))
	},
	"setrlimit.target.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.TTYName)
	},
	"setrlimit.target.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.Credentials.UID))
	},
	"setrlimit.target.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.Credentials.User)
	},
	"setrlimit.target.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Setrlimit.Target.Process.UserSession))
	},
	"setrlimit.target.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Setrlimit.Target.Process.UserSession))
	},
	"setrlimit.target.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Setrlimit.Target.Process.UserSession.K8SSessionContext))
	},
	"setrlimit.target.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"setrlimit.target.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Setrlimit.Target.Process.UserSession.K8SSessionContext))
	},
	"setrlimit.target.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Setrlimit.Target.Process.UserSession.K8SSessionContext))
	},
	"setrlimit.target.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Setrlimit.Target.Process.UserSession))
	},
	"setrlimit.target.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"setrlimit.target.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"setrlimit.target.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	"setrlimit.target.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"setrlimit.target.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setrlimit.target.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Setrlimit.Target.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"setsockopt.filter_hash": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.filter_hash")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetSockOptFilterHash(ev, &ev.SetSockOpt))
	},
	"setsockopt.filter_instructions": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.filter_instructions")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetSockOptFilterInstructions(ev, &ev.SetSockOpt))
	},
	"setsockopt.filter_len": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.filter_len")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.FilterLen))
	},
	"setsockopt.is_filter_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.is_filter_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SetSockOpt.IsFilterTruncated)
	},
	"setsockopt.level": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.level")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.Level))
	},
	"setsockopt.optname": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.optname")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.OptName))
	},
	"setsockopt.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SyscallEvent.Retval))
	},
	"setsockopt.socket_family": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.socket_family")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SocketFamily))
	},
	"setsockopt.socket_protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.socket_protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SocketProtocol))
	},
	"setsockopt.socket_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.socket_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetSockOpt.SocketType))
	},
	"setsockopt.used_immediates": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setsockopt.used_immediates")
		ev := ctx.Event.(*model.Event)
		return intsToVal(ev.FieldHandlers.ResolveSetSockOptUsedImmediates(ev, &ev.SetSockOpt))
	},
	"setuid.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetUID.EUID))
	},
	"setuid.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetuidEUser(ev, &ev.SetUID))
	},
	"setuid.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetUID.FSUID))
	},
	"setuid.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetuidFSUser(ev, &ev.SetUID))
	},
	"setuid.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetUID.UID))
	},
	"setuid.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setuid.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSetuidUser(ev, &ev.SetUID))
	},
	"setxattr.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.CTime))
	},
	"setxattr.file.destination.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrName(ev, &ev.SetXAttr))
	},
	"setxattr.file.destination.namespace": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.destination.namespace")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveXAttrNamespace(ev, &ev.SetXAttr))
	},
	"setxattr.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.GID))
	},
	"setxattr.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.SetXAttr.File.FileFields))
	},
	"setxattr.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.SetXAttr.File.FileFields))
	},
	"setxattr.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.PathKey.Inode))
	},
	"setxattr.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.Mode))
	},
	"setxattr.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.MTime))
	},
	"setxattr.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SetXAttr.File.MountDetached)
	},
	"setxattr.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.PathKey.MountID))
	},
	"setxattr.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SetXAttr.File.MountVisible)
	},
	"setxattr.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.SetXAttr.File))
	},
	"setxattr.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.SetXAttr.File.FileFields)))
	},
	"setxattr.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.File.FileFields.UID))
	},
	"setxattr.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.SetXAttr.File.FileFields))
	},
	"setxattr.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("setxattr.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SetXAttr.SyscallEvent.Retval))
	},
	"signal.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.PID))
	},
	"signal.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.SyscallEvent.Retval))
	},
	"signal.target.ancestors.args": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.args_flags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args_flags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.args_options": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args_options")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.args_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.args_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.argv": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.argv")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.argv0": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.argv0")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.auid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.auid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.AUID))
	},
	"signal.target.ancestors.cap_effective": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cap_effective")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapEffective))
	},
	"signal.target.ancestors.cap_permitted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cap_permitted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.CapPermitted))
	},
	"signal.target.ancestors.caps_attempted": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.caps_attempted")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsAttempted))
	},
	"signal.target.ancestors.caps_used": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.caps_used")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CapsUsed))
	},
	"signal.target.ancestors.cgroup.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CreatedAt))
	},
	"signal.target.ancestors.cgroup.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.file.inode")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.Inode))
	},
	"signal.target.ancestors.cgroup.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.CGroup.CGroupPathKey.MountID))
	},
	"signal.target.ancestors.cgroup.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.CGroup.CGroupID))
	},
	"signal.target.ancestors.cgroup.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.cgroup.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveCGroupVersion(ev, &element.ProcessContext.Process.CGroup)))
	},
	"signal.target.ancestors.comm": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.comm")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Comm)
	},
	"signal.target.ancestors.container.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	"signal.target.ancestors.container.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	"signal.target.ancestors.container.tags": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	"signal.target.ancestors.created_at": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	"signal.target.ancestors.egid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.egid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EGID))
	},
	"signal.target.ancestors.egroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.egroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EGroup)
	},
	"signal.target.ancestors.envp": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.envs": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.envs_truncated": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.envs_truncated")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.euid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.euid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.EUID))
	},
	"signal.target.ancestors.euser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.euser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.EUser)
	},
	"signal.target.ancestors.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.CTime))
	},
	"signal.target.ancestors.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.GID))
	},
	"signal.target.ancestors.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"signal.target.ancestors.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"signal.target.ancestors.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"signal.target.ancestors.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.Mode))
	},
	"signal.target.ancestors.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.MTime))
	},
	"signal.target.ancestors.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountDetached)
	},
	"signal.target.ancestors.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"signal.target.ancestors.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.FileEvent.MountVisible)
	},
	"signal.target.ancestors.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"signal.target.ancestors.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.FileEvent)))
	},
	"signal.target.ancestors.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	"signal.target.ancestors.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.FileEvent.FileFields)))
	},
	"signal.target.ancestors.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.FileEvent.FileFields.UID))
	},
	"signal.target.ancestors.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.FileEvent.FileFields))
	},
	"signal.target.ancestors.fsgid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsgid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSGID))
	},
	"signal.target.ancestors.fsgroup": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsgroup")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSGroup)
	},
	"signal.target.ancestors.fsuid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsuid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.FSUID))
	},
	"signal.target.ancestors.fsuser": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.fsuser")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.FSUser)
	},
	"signal.target.ancestors.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.gid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.GID))
	},
	"signal.target.ancestors.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.group")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.Group)
	},
	"signal.target.ancestors.interpreter.file.change_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.change_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"signal.target.ancestors.interpreter.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.filesystem": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.filesystem")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.gid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.gid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"signal.target.ancestors.interpreter.file.group": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.group")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"signal.target.ancestors.interpreter.file.hashes": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.hashes")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return stringsToVal([]string{""})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.in_upper_layer": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.in_upper_layer")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"signal.target.ancestors.interpreter.file.inode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.inode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"signal.target.ancestors.interpreter.file.mode": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mode")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"signal.target.ancestors.interpreter.file.modification_time": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.modification_time")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"signal.target.ancestors.interpreter.file.mount_detached": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mount_detached")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"signal.target.ancestors.interpreter.file.mount_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mount_id")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"signal.target.ancestors.interpreter.file.mount_visible": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.mount_visible")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(element.ProcessContext.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"signal.target.ancestors.interpreter.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.package.epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"signal.target.ancestors.interpreter.file.package.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.package.release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.package.source_epoch": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.source_epoch")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent)))
	},
	"signal.target.ancestors.interpreter.file.package.source_release": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.source_release")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.package.source_version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.source_version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.package.version": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.package.version")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.ancestors.interpreter.file.rights": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.rights")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"signal.target.ancestors.interpreter.file.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.uid")
		element := e.(*model.ProcessCacheEntry)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"signal.target.ancestors.interpreter.file.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.interpreter.file.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		if !element.ProcessContext.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &element.ProcessContext.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"signal.target.ancestors.is_exec": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.is_exec")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.IsExec)
	},
	"signal.target.ancestors.is_kworker": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.is_kworker")
		element := e.(*model.ProcessCacheEntry)
		return types.Bool(element.ProcessContext.Process.PIDContext.IsKworker)
	},
	"signal.target.ancestors.is_thread": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.is_thread")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &element.ProcessContext.Process))
	},
	"signal.target.ancestors.mntns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.mntns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.MntNS))
	},
	"signal.target.ancestors.netns": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.netns")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.NetNS))
	},
	"signal.target.ancestors.pid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	"signal.target.ancestors.ppid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	"signal.target.ancestors.sid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.sid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.SID))
	},
	"signal.target.ancestors.tid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.tid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Tid))
	},
	"signal.target.ancestors.tty_name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.tty_name")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.TTYName)
	},
	"signal.target.ancestors.uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.uid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.Credentials.UID))
	},
	"signal.target.ancestors.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.Credentials.User)
	},
	"signal.target.ancestors.user_session.id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.id")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &element.ProcessContext.Process.UserSession))
	},
	"signal.target.ancestors.user_session.identity": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.identity")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &element.ProcessContext.Process.UserSession))
	},
	"signal.target.ancestors.user_session.k8s_groups": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_groups")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"signal.target.ancestors.user_session.k8s_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"signal.target.ancestors.user_session.k8s_uid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_uid")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"signal.target.ancestors.user_session.k8s_username": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.k8s_username")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &element.ProcessContext.Process.UserSession.K8SSessionContext))
	},
	"signal.target.ancestors.user_session.session_type": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.session_type")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSessionType(ev, &element.ProcessContext.Process.UserSession)))
	},
	"signal.target.ancestors.user_session.ssh_auth_method": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_auth_method")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHAuthMethod))
	},
	"signal.target.ancestors.user_session.ssh_client_ip": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_client_ip")
		element := e.(*model.ProcessCacheEntry)
		return cidrToVal(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"signal.target.ancestors.user_session.ssh_client_port": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_client_port")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHClientPort))
	},
	"signal.target.ancestors.user_session.ssh_public_key": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_public_key")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"signal.target.ancestors.user_session.ssh_session_id": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("signal.target.ancestors.user_session.ssh_session_id")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"signal.target.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, &ev.Signal.Target.Process))
	},
	"signal.target.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args_flags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, &ev.Signal.Target.Process))
	},
	"signal.target.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args_options")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, &ev.Signal.Target.Process))
	},
	"signal.target.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.args_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, &ev.Signal.Target.Process))
	},
	"signal.target.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.argv")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, &ev.Signal.Target.Process))
	},
	"signal.target.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.argv0")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, &ev.Signal.Target.Process))
	},
	"signal.target.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.auid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.AUID))
	},
	"signal.target.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cap_effective")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.CapEffective))
	},
	"signal.target.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cap_permitted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.CapPermitted))
	},
	"signal.target.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.caps_attempted")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CapsAttempted))
	},
	"signal.target.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.caps_used")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CapsUsed))
	},
	"signal.target.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CGroup.CreatedAt))
	},
	"signal.target.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CGroup.CGroupPathKey.Inode))
	},
	"signal.target.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.CGroup.CGroupPathKey.MountID))
	},
	"signal.target.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Signal.Target.Process.CGroup.CGroupID))
	},
	"signal.target.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.cgroup.version")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Signal.Target.Process.CGroup))
	},
	"signal.target.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.comm")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Comm)
	},
	"signal.target.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.ContainerContext.CreatedAt))
	},
	"signal.target.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Signal.Target.Process.ContainerContext.ContainerID))
	},
	"signal.target.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Signal.Target.Process.ContainerContext))
	},
	"signal.target.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.Signal.Target.Process)))
	},
	"signal.target.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.egid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.EGID))
	},
	"signal.target.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.egroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.EGroup)
	},
	"signal.target.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.Signal.Target.Process))
	},
	"signal.target.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.Signal.Target.Process))
	},
	"signal.target.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.envs_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, &ev.Signal.Target.Process))
	},
	"signal.target.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.euid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.EUID))
	},
	"signal.target.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.euser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.EUser)
	},
	"signal.target.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.CTime))
	},
	"signal.target.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.GID))
	},
	"signal.target.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
	},
	"signal.target.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
	},
	"signal.target.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.PathKey.Inode))
	},
	"signal.target.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.Mode))
	},
	"signal.target.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.MTime))
	},
	"signal.target.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.FileEvent.MountDetached)
	},
	"signal.target.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.PathKey.MountID))
	},
	"signal.target.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.FileEvent.MountVisible)
	},
	"signal.target.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.FileEvent))
	},
	"signal.target.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.FileEvent.FileFields)))
	},
	"signal.target.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.FileEvent.FileFields.UID))
	},
	"signal.target.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.IsNotKworker() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.FileEvent.FileFields))
	},
	"signal.target.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsgid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.FSGID))
	},
	"signal.target.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsgroup")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.FSGroup)
	},
	"signal.target.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsuid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.FSUID))
	},
	"signal.target.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.fsuser")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.FSUser)
	},
	"signal.target.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.GID))
	},
	"signal.target.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.Group)
	},
	"signal.target.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.change_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.CTime))
	},
	"signal.target.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.filesystem")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.GID))
	},
	"signal.target.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"signal.target.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.hashes")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"signal.target.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.Inode))
	},
	"signal.target.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.Mode))
	},
	"signal.target.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.modification_time")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.MTime))
	},
	"signal.target.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.LinuxBinprm.FileEvent.MountDetached)
	},
	"signal.target.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.PathKey.MountID))
	},
	"signal.target.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Process.LinuxBinprm.FileEvent.MountVisible)
	},
	"signal.target.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.package.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent))
	},
	"signal.target.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.rights")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields)))
	},
	"signal.target.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields.UID))
	},
	"signal.target.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.interpreter.file.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.Process.HasInterpreter() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Signal.Target.Process.LinuxBinprm.FileEvent.FileFields))
	},
	"signal.target.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.is_exec")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Signal.Target.Process.IsExec)
	},
	"signal.target.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.is_kworker")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Signal.Target.Process.PIDContext.IsKworker)
	},
	"signal.target.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.is_thread")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, &ev.Signal.Target.Process))
	},
	"signal.target.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.mntns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.MntNS))
	},
	"signal.target.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.netns")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.NetNS))
	},
	"signal.target.parent.args": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgs(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.args_flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args_flags")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsFlags(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.args_options": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args_options")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgsOptions(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.args_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.args_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessArgsTruncated(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.argv": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.argv")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessArgv(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.argv0": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.argv0")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessArgv0(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.auid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.auid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.AUID))
	},
	"signal.target.parent.cap_effective": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cap_effective")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.CapEffective))
	},
	"signal.target.parent.cap_permitted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cap_permitted")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.CapPermitted))
	},
	"signal.target.parent.caps_attempted": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.caps_attempted")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CapsAttempted))
	},
	"signal.target.parent.caps_used": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.caps_used")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CapsUsed))
	},
	"signal.target.parent.cgroup.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CGroup.CreatedAt))
	},
	"signal.target.parent.cgroup.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.file.inode")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CGroup.CGroupPathKey.Inode))
	},
	"signal.target.parent.cgroup.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.file.mount_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.CGroup.CGroupPathKey.MountID))
	},
	"signal.target.parent.cgroup.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Signal.Target.Parent.CGroup.CGroupID))
	},
	"signal.target.parent.cgroup.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.cgroup.version")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveCGroupVersion(ev, &ev.Signal.Target.Parent.CGroup))
	},
	"signal.target.parent.comm": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.comm")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Comm)
	},
	"signal.target.parent.container.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.ContainerContext.CreatedAt))
	},
	"signal.target.parent.container.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.Signal.Target.Parent.ContainerContext.ContainerID))
	},
	"signal.target.parent.container.tags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Signal.Target.Parent.ContainerContext))
	},
	"signal.target.parent.created_at": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Signal.Target.Parent)))
	},
	"signal.target.parent.egid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.egid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.EGID))
	},
	"signal.target.parent.egroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.egroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.EGroup)
	},
	"signal.target.parent.envp": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.envs": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.envs_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.envs_truncated")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessEnvsTruncated(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.euid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.euid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.EUID))
	},
	"signal.target.parent.euser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.euser")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.EUser)
	},
	"signal.target.parent.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.fsgid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsgid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.FSGID))
	},
	"signal.target.parent.fsgroup": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsgroup")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.FSGroup)
	},
	"signal.target.parent.fsuid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsuid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.FSUID))
	},
	"signal.target.parent.fsuser": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.fsuser")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.FSUser)
	},
	"signal.target.parent.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.gid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.GID))
	},
	"signal.target.parent.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.group")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.Group)
	},
	"signal.target.parent.interpreter.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.extension": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.gid": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.group": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.inode": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.mode": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.path": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.rights": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.uid": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.interpreter.file.user": func(ctx *eval.Context, _ any) ref.Val {
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
	"signal.target.parent.is_exec": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.is_exec")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.IsExec)
	},
	"signal.target.parent.is_kworker": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.is_kworker")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.Signal.Target.Parent.PIDContext.IsKworker)
	},
	"signal.target.parent.is_thread": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.is_thread")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Bool(false)
		}
		return types.Bool(ev.FieldHandlers.ResolveProcessIsThread(ev, ev.Signal.Target.Parent))
	},
	"signal.target.parent.mntns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.mntns")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.MntNS))
	},
	"signal.target.parent.netns": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.netns")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.NetNS))
	},
	"signal.target.parent.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.Pid))
	},
	"signal.target.parent.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PPid))
	},
	"signal.target.parent.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.sid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.SID))
	},
	"signal.target.parent.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.tid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.PIDContext.Tid))
	},
	"signal.target.parent.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.tty_name")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.TTYName)
	},
	"signal.target.parent.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.Credentials.UID))
	},
	"signal.target.parent.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.Credentials.User)
	},
	"signal.target.parent.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Signal.Target.Parent.UserSession))
	},
	"signal.target.parent.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.identity")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Signal.Target.Parent.UserSession))
	},
	"signal.target.parent.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Parent.UserSession.K8SSessionContext))
	},
	"signal.target.parent.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.UserSession.K8SSessionContext.K8SSessionID))
	},
	"signal.target.parent.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Parent.UserSession.K8SSessionContext))
	},
	"signal.target.parent.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Parent.UserSession.K8SSessionContext))
	},
	"signal.target.parent.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Signal.Target.Parent.UserSession))
	},
	"signal.target.parent.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"signal.target.parent.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return cidrToVal(net.IPNet{})
		}
		return cidrToVal(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHClientIP)
	},
	"signal.target.parent.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHClientPort)
	},
	"signal.target.parent.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.String("")
		}
		return types.String(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"signal.target.parent.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.parent.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		if !ev.Signal.Target.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.Signal.Target.Parent.UserSession.SSHSessionContext.SSHSessionID))
	},
	"signal.target.pid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.Pid))
	},
	"signal.target.ppid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PPid))
	},
	"signal.target.sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.sid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.SID))
	},
	"signal.target.tid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.tid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.PIDContext.Tid))
	},
	"signal.target.tty_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.tty_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.TTYName)
	},
	"signal.target.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.Credentials.UID))
	},
	"signal.target.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.Credentials.User)
	},
	"signal.target.user_session.id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.id")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionID(ev, &ev.Signal.Target.Process.UserSession))
	},
	"signal.target.user_session.identity": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.identity")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSessionIdentity(ev, &ev.Signal.Target.Process.UserSession))
	},
	"signal.target.user_session.k8s_groups": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_groups")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveK8SGroups(ev, &ev.Signal.Target.Process.UserSession.K8SSessionContext))
	},
	"signal.target.user_session.k8s_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.UserSession.K8SSessionContext.K8SSessionID))
	},
	"signal.target.user_session.k8s_uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_uid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUID(ev, &ev.Signal.Target.Process.UserSession.K8SSessionContext))
	},
	"signal.target.user_session.k8s_username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.k8s_username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveK8SUsername(ev, &ev.Signal.Target.Process.UserSession.K8SSessionContext))
	},
	"signal.target.user_session.session_type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.session_type")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolveSessionType(ev, &ev.Signal.Target.Process.UserSession))
	},
	"signal.target.user_session.ssh_auth_method": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_auth_method")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHAuthMethod)
	},
	"signal.target.user_session.ssh_client_ip": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_client_ip")
		ev := ctx.Event.(*model.Event)
		return cidrToVal(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHClientIP)
	},
	"signal.target.user_session.ssh_client_port": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_client_port")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHClientPort)
	},
	"signal.target.user_session.ssh_public_key": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_public_key")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHPublicKey)
	},
	"signal.target.user_session.ssh_session_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.target.user_session.ssh_session_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Target.Process.UserSession.SSHSessionContext.SSHSessionID))
	},
	"signal.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("signal.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Signal.Type))
	},
	"socket.domain": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.domain")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.Domain))
	},
	"socket.protocol": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.protocol")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.Protocol))
	},
	"socket.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.SyscallEvent.Retval))
	},
	"socket.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("socket.type")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Socket.Type))
	},
	"splice.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.CTime))
	},
	"splice.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Splice.File))
	},
	"splice.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Splice.File))
	},
	"splice.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.GID))
	},
	"splice.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Splice.File.FileFields))
	},
	"splice.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Splice.File))
	},
	"splice.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Splice.File.FileFields))
	},
	"splice.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.PathKey.Inode))
	},
	"splice.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.Mode))
	},
	"splice.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.MTime))
	},
	"splice.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Splice.File.MountDetached)
	},
	"splice.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.PathKey.MountID))
	},
	"splice.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Splice.File.MountVisible)
	},
	"splice.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Splice.File))
	},
	"splice.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Splice.File))
	},
	"splice.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Splice.File))
	},
	"splice.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Splice.File))
	},
	"splice.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Splice.File))
	},
	"splice.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Splice.File))
	},
	"splice.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Splice.File))
	},
	"splice.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Splice.File))
	},
	"splice.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Splice.File))
	},
	"splice.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Splice.File.FileFields)))
	},
	"splice.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.File.FileFields.UID))
	},
	"splice.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Splice.File.FileFields))
	},
	"splice.pipe_entry_flag": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.pipe_entry_flag")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.PipeEntryFlag))
	},
	"splice.pipe_exit_flag": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.pipe_exit_flag")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.PipeExitFlag))
	},
	"splice.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("splice.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Splice.SyscallEvent.Retval))
	},
	"sysctl.action": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.action")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SysCtl.Action))
	},
	"sysctl.file_position": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.file_position")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.SysCtl.FilePosition))
	},
	"sysctl.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SysCtl.Name)
	},
	"sysctl.name_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.name_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SysCtl.NameTruncated)
	},
	"sysctl.old_value": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.old_value")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SysCtl.OldValue)
	},
	"sysctl.old_value_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.old_value_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SysCtl.OldValueTruncated)
	},
	"sysctl.value": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.value")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SysCtl.Value)
	},
	"sysctl.value_truncated": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("sysctl.value_truncated")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.SysCtl.ValueTruncated)
	},
	"unlink.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.CTime))
	},
	"unlink.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Unlink.File))
	},
	"unlink.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Unlink.File))
	},
	"unlink.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.GID))
	},
	"unlink.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Unlink.File.FileFields))
	},
	"unlink.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Unlink.File))
	},
	"unlink.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Unlink.File.FileFields))
	},
	"unlink.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.PathKey.Inode))
	},
	"unlink.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.Mode))
	},
	"unlink.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.MTime))
	},
	"unlink.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Unlink.File.MountDetached)
	},
	"unlink.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.PathKey.MountID))
	},
	"unlink.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Unlink.File.MountVisible)
	},
	"unlink.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Unlink.File))
	},
	"unlink.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Unlink.File))
	},
	"unlink.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Unlink.File))
	},
	"unlink.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Unlink.File))
	},
	"unlink.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Unlink.File))
	},
	"unlink.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Unlink.File))
	},
	"unlink.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Unlink.File))
	},
	"unlink.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Unlink.File))
	},
	"unlink.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Unlink.File))
	},
	"unlink.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Unlink.File.FileFields)))
	},
	"unlink.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.File.FileFields.UID))
	},
	"unlink.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Unlink.File.FileFields))
	},
	"unlink.flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.Flags))
	},
	"unlink.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Unlink.SyscallEvent.Retval))
	},
	"unlink.syscall.dirfd": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.syscall.dirfd")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt1(ev, &ev.Unlink.SyscallContext)))
	},
	"unlink.syscall.flags": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.syscall.flags")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveSyscallCtxArgsInt3(ev, &ev.Unlink.SyscallContext)))
	},
	"unlink.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unlink.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr2(ev, &ev.Unlink.SyscallContext))
	},
	"unload_module.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unload_module.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.UnloadModule.Name)
	},
	"unload_module.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("unload_module.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.UnloadModule.SyscallEvent.Retval))
	},
	"utimes.file.change_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.change_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.CTime))
	},
	"utimes.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Utimes.File))
	},
	"utimes.file.filesystem": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.filesystem")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFilesystem(ev, &ev.Utimes.File))
	},
	"utimes.file.gid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.gid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.GID))
	},
	"utimes.file.group": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.group")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsGroup(ev, &ev.Utimes.File.FileFields))
	},
	"utimes.file.hashes": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.hashes")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveHashesFromEvent(ev, &ev.Utimes.File))
	},
	"utimes.file.in_upper_layer": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.in_upper_layer")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.FieldHandlers.ResolveFileFieldsInUpperLayer(ev, &ev.Utimes.File.FileFields))
	},
	"utimes.file.inode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.inode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.PathKey.Inode))
	},
	"utimes.file.mode": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mode")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.Mode))
	},
	"utimes.file.modification_time": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.modification_time")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.MTime))
	},
	"utimes.file.mount_detached": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mount_detached")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Utimes.File.MountDetached)
	},
	"utimes.file.mount_id": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mount_id")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.PathKey.MountID))
	},
	"utimes.file.mount_visible": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.mount_visible")
		ev := ctx.Event.(*model.Event)
		return types.Bool(ev.Utimes.File.MountVisible)
	},
	"utimes.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Utimes.File))
	},
	"utimes.file.package.epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageEpoch(ev, &ev.Utimes.File))
	},
	"utimes.file.package.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageName(ev, &ev.Utimes.File))
	},
	"utimes.file.package.release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageRelease(ev, &ev.Utimes.File))
	},
	"utimes.file.package.source_epoch": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.source_epoch")
		ev := ctx.Event.(*model.Event)
		return types.Int(ev.FieldHandlers.ResolvePackageSourceEpoch(ev, &ev.Utimes.File))
	},
	"utimes.file.package.source_release": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.source_release")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceRelease(ev, &ev.Utimes.File))
	},
	"utimes.file.package.source_version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.source_version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageSourceVersion(ev, &ev.Utimes.File))
	},
	"utimes.file.package.version": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.package.version")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolvePackageVersion(ev, &ev.Utimes.File))
	},
	"utimes.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Utimes.File))
	},
	"utimes.file.rights": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.rights")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveRights(ev, &ev.Utimes.File.FileFields)))
	},
	"utimes.file.uid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.uid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.File.FileFields.UID))
	},
	"utimes.file.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.file.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileFieldsUser(ev, &ev.Utimes.File.FileFields))
	},
	"utimes.retval": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.retval")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Utimes.SyscallEvent.Retval))
	},
	"utimes.syscall.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("utimes.syscall.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSyscallCtxArgsStr1(ev, &ev.Utimes.SyscallContext))
	},
}

// celIterators opens a cursor over an iterated SECL field, keyed by its SECL
// name.
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
var celIterators = map[string]celIterator{
	"network_flow_monitor.flows": func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.Flow]{
			iterator: &model.FlowsIterator{Root: ev.NetworkFlowMonitor.Flows},
			ctx:      ctx,
		}
	},
	"process.ancestors": func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor},
			ctx:      ctx,
		}
	},
	"ptrace.tracee.ancestors": func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.PTrace.Tracee.Ancestor},
			ctx:      ctx,
		}
	},
	"setrlimit.target.ancestors": func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.Setrlimit.Target.Ancestor},
			ctx:      ctx,
		}
	},
	"signal.target.ancestors": func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.Signal.Target.Ancestor},
			ctx:      ctx,
		}
	},
}
