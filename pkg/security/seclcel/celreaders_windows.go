// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

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
	// 0: change_permission.new_sd
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.new_sd")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission))
	},
	// 1: change_permission.old_sd
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.old_sd")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission))
	},
	// 2: change_permission.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.ObjectName)
	},
	// 3: change_permission.type
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.ObjectType)
	},
	// 4: change_permission.user_domain
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.user_domain")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.UserDomain)
	},
	// 5: change_permission.username
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.UserName)
	},
	// 6: create.file.device_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File))
	},
	// 7: create.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.CreateNewFile.File))
	},
	// 8: create.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File))
	},
	// 9: create.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File))
	},
	// 10: create.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyName)
	},
	// 11: create.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyPath)
	},
	// 12: create_key.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create_key.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyName)
	},
	// 13: create_key.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create_key.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyPath)
	},
	// 14: delete.file.device_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File))
	},
	// 15: delete.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.DeleteFile.File))
	},
	// 16: delete.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File))
	},
	// 17: delete.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File))
	},
	// 18: delete.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyName)
	},
	// 19: delete.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyPath)
	},
	// 20: delete_key.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete_key.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyName)
	},
	// 21: delete_key.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete_key.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyPath)
	},
	// 22: event.hostname
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.hostname")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent))
	},
	// 23: event.origin
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.origin")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.Origin)
	},
	// 24: event.os
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.os")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.Os)
	},
	// 25: event.rule.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.rule.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.BaseEvent.RuleTags)
	},
	// 26: event.service
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.service")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent))
	},
	// 27: event.source
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.source")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveSource(ev, &ev.BaseEvent))
	},
	// 28: event.timestamp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("event.timestamp")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)))
	},
	// 29: exec.cmdline
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cmdline")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process))
	},
	// 30: exec.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.ContainerContext.CreatedAt))
	},
	// 31: exec.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exec.Process.ContainerContext.ContainerID))
	},
	// 32: exec.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exec.Process.ContainerContext))
	},
	// 33: exec.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)))
	},
	// 34: exec.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process))
	},
	// 35: exec.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process))
	},
	// 36: exec.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.FileEvent))
	},
	// 37: exec.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
	},
	// 38: exec.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
	},
	// 39: exec.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PIDContext.Pid))
	},
	// 40: exec.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exec.Process.PPid))
	},
	// 41: exec.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process))
	},
	// 42: exec.user_sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_sid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.OwnerSidString)
	},
	// 43: exit.cause
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cause")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Cause))
	},
	// 44: exit.cmdline
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cmdline")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process))
	},
	// 45: exit.code
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.code")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Code))
	},
	// 46: exit.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.ContainerContext.CreatedAt))
	},
	// 47: exit.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.Exit.Process.ContainerContext.ContainerID))
	},
	// 48: exit.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exit.Process.ContainerContext))
	},
	// 49: exit.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)))
	},
	// 50: exit.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process))
	},
	// 51: exit.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process))
	},
	// 52: exit.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.FileEvent))
	},
	// 53: exit.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
	},
	// 54: exit.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
	},
	// 55: exit.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PIDContext.Pid))
	},
	// 56: exit.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Process.PPid))
	},
	// 57: exit.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process))
	},
	// 58: exit.user_sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_sid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.OwnerSidString)
	},
	// 59: open.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyName)
	},
	// 60: open.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyPath)
	},
	// 61: open_key.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open_key.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyName)
	},
	// 62: open_key.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open_key.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyPath)
	},
	// 63: process.ancestors.cmdline
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cmdline")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process))
	},
	// 64: process.ancestors.container.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.created_at")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 65: process.ancestors.container.id
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.id")
		element := e.(*model.ProcessCacheEntry)
		return types.String(string(element.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 66: process.ancestors.container.tags
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.container.tags")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext))
	},
	// 67: process.ancestors.created_at
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.created_at")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process)))
	},
	// 68: process.ancestors.envp
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envp")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process))
	},
	// 69: process.ancestors.envs
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.envs")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process))
	},
	// 70: process.ancestors.file.extension
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 71: process.ancestors.file.name
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 72: process.ancestors.file.path
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
	},
	// 73: process.ancestors.pid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.pid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PIDContext.Pid))
	},
	// 74: process.ancestors.ppid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.ppid")
		element := e.(*model.ProcessCacheEntry)
		return types.Int(int(element.ProcessContext.Process.PPid))
	},
	// 75: process.ancestors.user
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process))
	},
	// 76: process.ancestors.user_sid
	func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_sid")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.OwnerSidString)
	},
	// 77: process.cmdline
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cmdline")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 78: process.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.ContainerContext.CreatedAt))
	},
	// 79: process.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.id")
		ev := ctx.Event.(*model.Event)
		return types.String(string(ev.BaseEvent.ProcessContext.Process.ContainerContext.ContainerID))
	},
	// 80: process.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.container.tags")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Process.ContainerContext))
	},
	// 81: process.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.created_at")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)))
	},
	// 82: process.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envp")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 83: process.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.envs")
		ev := ctx.Event.(*model.Event)
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 84: process.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 85: process.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 86: process.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	// 87: process.parent.cmdline
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cmdline")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 88: process.parent.container.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.ContainerContext.CreatedAt))
	},
	// 89: process.parent.container.id
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.id")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(string(ev.BaseEvent.ProcessContext.Parent.ContainerContext.ContainerID))
	},
	// 90: process.parent.container.tags
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.container.tags")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Parent.ContainerContext))
	},
	// 91: process.parent.created_at
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.created_at")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)))
	},
	// 92: process.parent.envp
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envp")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 93: process.parent.envs
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.envs")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return stringsToVal([]string{})
		}
		return stringsToVal(ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 94: process.parent.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 95: process.parent.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 96: process.parent.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	// 97: process.parent.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.pid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid))
	},
	// 98: process.parent.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.ppid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.Int(0)
		}
		return types.Int(int(ev.BaseEvent.ProcessContext.Parent.PPid))
	},
	// 99: process.parent.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	// 100: process.parent.user_sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_sid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.OwnerSidString)
	},
	// 101: process.pid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.pid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid))
	},
	// 102: process.ppid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.ppid")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.BaseEvent.ProcessContext.Process.PPid))
	},
	// 103: process.user
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	// 104: process.user_sid
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_sid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.OwnerSidString)
	},
	// 105: rename.file.destination.device_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New))
	},
	// 106: rename.file.destination.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.New))
	},
	// 107: rename.file.destination.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New))
	},
	// 108: rename.file.destination.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New))
	},
	// 109: rename.file.device_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old))
	},
	// 110: rename.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.Old))
	},
	// 111: rename.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old))
	},
	// 112: rename.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old))
	},
	// 113: set.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyName)
	},
	// 114: set.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyPath)
	},
	// 115: set.registry.value_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.registry.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	// 116: set.value_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	// 117: set_key_value.registry.key_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyName)
	},
	// 118: set_key_value.registry.key_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyPath)
	},
	// 119: set_key_value.registry.value_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.registry.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	// 120: set_key_value.value_name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	// 121: write.file.device_path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File))
	},
	// 122: write.file.extension
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.WriteFile.File))
	},
	// 123: write.file.name
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File))
	},
	// 124: write.file.path
	func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File))
	},
}

// celReaderIndex gives the index of each field in the layout above. It is what
// the optimization pass resolves a field name through, once per rule.
var celReaderIndex = map[string]int{
	"change_permission.new_sd":               0,
	"change_permission.old_sd":               1,
	"change_permission.path":                 2,
	"change_permission.type":                 3,
	"change_permission.user_domain":          4,
	"change_permission.username":             5,
	"create.file.device_path":                6,
	"create.file.extension":                  7,
	"create.file.name":                       8,
	"create.file.path":                       9,
	"create.registry.key_name":               10,
	"create.registry.key_path":               11,
	"create_key.registry.key_name":           12,
	"create_key.registry.key_path":           13,
	"delete.file.device_path":                14,
	"delete.file.extension":                  15,
	"delete.file.name":                       16,
	"delete.file.path":                       17,
	"delete.registry.key_name":               18,
	"delete.registry.key_path":               19,
	"delete_key.registry.key_name":           20,
	"delete_key.registry.key_path":           21,
	"event.hostname":                         22,
	"event.origin":                           23,
	"event.os":                               24,
	"event.rule.tags":                        25,
	"event.service":                          26,
	"event.source":                           27,
	"event.timestamp":                        28,
	"exec.cmdline":                           29,
	"exec.container.created_at":              30,
	"exec.container.id":                      31,
	"exec.container.tags":                    32,
	"exec.created_at":                        33,
	"exec.envp":                              34,
	"exec.envs":                              35,
	"exec.file.extension":                    36,
	"exec.file.name":                         37,
	"exec.file.path":                         38,
	"exec.pid":                               39,
	"exec.ppid":                              40,
	"exec.user":                              41,
	"exec.user_sid":                          42,
	"exit.cause":                             43,
	"exit.cmdline":                           44,
	"exit.code":                              45,
	"exit.container.created_at":              46,
	"exit.container.id":                      47,
	"exit.container.tags":                    48,
	"exit.created_at":                        49,
	"exit.envp":                              50,
	"exit.envs":                              51,
	"exit.file.extension":                    52,
	"exit.file.name":                         53,
	"exit.file.path":                         54,
	"exit.pid":                               55,
	"exit.ppid":                              56,
	"exit.user":                              57,
	"exit.user_sid":                          58,
	"open.registry.key_name":                 59,
	"open.registry.key_path":                 60,
	"open_key.registry.key_name":             61,
	"open_key.registry.key_path":             62,
	"process.ancestors.cmdline":              63,
	"process.ancestors.container.created_at": 64,
	"process.ancestors.container.id":         65,
	"process.ancestors.container.tags":       66,
	"process.ancestors.created_at":           67,
	"process.ancestors.envp":                 68,
	"process.ancestors.envs":                 69,
	"process.ancestors.file.extension":       70,
	"process.ancestors.file.name":            71,
	"process.ancestors.file.path":            72,
	"process.ancestors.pid":                  73,
	"process.ancestors.ppid":                 74,
	"process.ancestors.user":                 75,
	"process.ancestors.user_sid":             76,
	"process.cmdline":                        77,
	"process.container.created_at":           78,
	"process.container.id":                   79,
	"process.container.tags":                 80,
	"process.created_at":                     81,
	"process.envp":                           82,
	"process.envs":                           83,
	"process.file.extension":                 84,
	"process.file.name":                      85,
	"process.file.path":                      86,
	"process.parent.cmdline":                 87,
	"process.parent.container.created_at":    88,
	"process.parent.container.id":            89,
	"process.parent.container.tags":          90,
	"process.parent.created_at":              91,
	"process.parent.envp":                    92,
	"process.parent.envs":                    93,
	"process.parent.file.extension":          94,
	"process.parent.file.name":               95,
	"process.parent.file.path":               96,
	"process.parent.pid":                     97,
	"process.parent.ppid":                    98,
	"process.parent.user":                    99,
	"process.parent.user_sid":                100,
	"process.pid":                            101,
	"process.ppid":                           102,
	"process.user":                           103,
	"process.user_sid":                       104,
	"rename.file.destination.device_path":    105,
	"rename.file.destination.extension":      106,
	"rename.file.destination.name":           107,
	"rename.file.destination.path":           108,
	"rename.file.device_path":                109,
	"rename.file.extension":                  110,
	"rename.file.name":                       111,
	"rename.file.path":                       112,
	"set.registry.key_name":                  113,
	"set.registry.key_path":                  114,
	"set.registry.value_name":                115,
	"set.value_name":                         116,
	"set_key_value.registry.key_name":        117,
	"set_key_value.registry.key_path":        118,
	"set_key_value.registry.value_name":      119,
	"set_key_value.value_name":               120,
	"write.file.device_path":                 121,
	"write.file.extension":                   122,
	"write.file.name":                        123,
	"write.file.path":                        124,
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
	// 0: process.ancestors
	func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor},
			ctx:      ctx,
		}
	},
}

// celIteratorIndex gives the index of each iterated field, for the same reason
// celReaderIndex does.
var celIteratorIndex = map[string]int{
	"process.ancestors": 0,
}
