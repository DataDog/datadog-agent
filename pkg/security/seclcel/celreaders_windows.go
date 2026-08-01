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
	"change_permission.new_sd": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.new_sd")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission))
	},
	"change_permission.old_sd": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.old_sd")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission))
	},
	"change_permission.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.ObjectName)
	},
	"change_permission.type": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.type")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.ObjectType)
	},
	"change_permission.user_domain": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.user_domain")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.UserDomain)
	},
	"change_permission.username": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("change_permission.username")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.ChangePermission.UserName)
	},
	"create.file.device_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File))
	},
	"create.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.CreateNewFile.File))
	},
	"create.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File))
	},
	"create.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File))
	},
	"create.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyName)
	},
	"create.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyPath)
	},
	"create_key.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create_key.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyName)
	},
	"create_key.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("create_key.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.CreateRegistryKey.Registry.KeyPath)
	},
	"delete.file.device_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File))
	},
	"delete.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.DeleteFile.File))
	},
	"delete.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File))
	},
	"delete.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File))
	},
	"delete.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyName)
	},
	"delete.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyPath)
	},
	"delete_key.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete_key.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyName)
	},
	"delete_key.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("delete_key.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.DeleteRegistryKey.Registry.KeyPath)
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
	"exec.cmdline": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.cmdline")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process))
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
	"exec.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
	},
	"exec.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
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
	"exec.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process))
	},
	"exec.user_sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exec.user_sid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exec.Process.OwnerSidString)
	},
	"exit.cause": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cause")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Cause))
	},
	"exit.cmdline": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.cmdline")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process))
	},
	"exit.code": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.code")
		ev := ctx.Event.(*model.Event)
		return types.Int(int(ev.Exit.Code))
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
	"exit.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
	},
	"exit.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
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
	"exit.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process))
	},
	"exit.user_sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("exit.user_sid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.Exit.Process.OwnerSidString)
	},
	"open.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyName)
	},
	"open.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyPath)
	},
	"open_key.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open_key.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyName)
	},
	"open_key.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("open_key.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.OpenRegistryKey.Registry.KeyPath)
	},
	"process.ancestors.cmdline": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.cmdline")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process))
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
	"process.ancestors.file.extension": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.extension")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.name": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.name")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
	},
	"process.ancestors.file.path": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.file.path")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
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
	"process.ancestors.user": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user")
		element := e.(*model.ProcessCacheEntry)
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process))
	},
	"process.ancestors.user_sid": func(ctx *eval.Context, e any) ref.Val {
		ctx.AppendResolvedField("process.ancestors.user_sid")
		element := e.(*model.ProcessCacheEntry)
		return types.String(element.ProcessContext.Process.OwnerSidString)
	},
	"process.cmdline": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.cmdline")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process))
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
	"process.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
	},
	"process.parent.cmdline": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.cmdline")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent))
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
	"process.parent.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.extension")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	"process.parent.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.name")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
	},
	"process.parent.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.file.path")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
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
	"process.parent.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent))
	},
	"process.parent.user_sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.parent.user_sid")
		ev := ctx.Event.(*model.Event)
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return types.String("")
		}
		return types.String(ev.BaseEvent.ProcessContext.Parent.OwnerSidString)
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
	"process.user": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process))
	},
	"process.user_sid": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("process.user_sid")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.BaseEvent.ProcessContext.Process.OwnerSidString)
	},
	"rename.file.destination.device_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New))
	},
	"rename.file.destination.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.New))
	},
	"rename.file.destination.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New))
	},
	"rename.file.destination.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.destination.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New))
	},
	"rename.file.device_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old))
	},
	"rename.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.Old))
	},
	"rename.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old))
	},
	"rename.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("rename.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old))
	},
	"set.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyName)
	},
	"set.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyPath)
	},
	"set.registry.value_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.registry.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	"set.value_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	"set_key_value.registry.key_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.registry.key_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyName)
	},
	"set_key_value.registry.key_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.registry.key_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.Registry.KeyPath)
	},
	"set_key_value.registry.value_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.registry.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	"set_key_value.value_name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("set_key_value.value_name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.SetRegistryKeyValue.ValueName)
	},
	"write.file.device_path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.device_path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File))
	},
	"write.file.extension": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.extension")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.WriteFile.File))
	},
	"write.file.name": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.name")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File))
	},
	"write.file.path": func(ctx *eval.Context, _ any) ref.Val {
		ctx.AppendResolvedField("write.file.path")
		ev := ctx.Event.(*model.Event)
		return types.String(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File))
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
	"process.ancestors": func(ctx *eval.Context) celCursor {
		ev := ctx.Event.(*model.Event)
		return &modelCursor[*model.ProcessCacheEntry]{
			iterator: &model.ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor},
			ctx:      ctx,
		}
	},
}
