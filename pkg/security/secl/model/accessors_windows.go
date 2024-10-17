// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"math"
	"reflect"
)

// to always require the math package
var _ = math.MaxUint16

func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{
		eval.EventType("change_permission"),
		eval.EventType("create"),
		eval.EventType("create_key"),
		eval.EventType("delete"),
		eval.EventType("delete_key"),
		eval.EventType("exec"),
		eval.EventType("exit"),
		eval.EventType("open_key"),
		eval.EventType("rename"),
		eval.EventType("set_key_value"),
		eval.EventType("write"),
	}
}
func (m *Model) GetFieldRestrictions(field eval.Field) []eval.EventType {
	switch field {
	}
	return nil
}
func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	case "change_permission.new_sd":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "change_permission.old_sd":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "change_permission.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.ObjectName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "change_permission.type":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.ObjectType
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "change_permission.user_domain":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.UserDomain
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "change_permission.username":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.UserName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "container.runtime":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerRuntime(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
		}, nil
	case "create.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "delete.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "delete.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "delete.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "delete.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "delete.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "delete.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "event.hostname":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "event.origin":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.Origin
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "event.os":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.Os
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "event.service":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "event.timestamp":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
		}, nil
	case "exec.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exec.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cause":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Cause)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
		}, nil
	case "exit.code":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Code)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "exit.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "exit.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ancestors.cmdline":
		return &eval.StringArrayEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process)
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 200 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := element.ProcessContext.Process.ContainerID
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := element.ProcessContext.Process.ContainerID
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
					results = append(results, result...)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				iterator := &ProcessAncestorsIterator{}
				return iterator.Len(ctx)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := int(element.ProcessContext.Process.PPid)
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := int(element.ProcessContext.Process.PPid)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process)
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process)
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.ancestors.user_sid":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				if regID != "" {
					value := iterator.At(ctx, regID, ctx.Registers[regID])
					if value == nil {
						return results
					}
					element := value
					result := element.ProcessContext.Process.OwnerSidString
					results = append(results, result)
					return results
				}
				value := iterator.Front(ctx)
				for value != nil {
					element := value
					result := element.ProcessContext.Process.OwnerSidString
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
		}, nil
	case "process.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
		}, nil
	case "process.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.ProcessContext.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
		}, nil
	case "process.parent.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.BaseEvent.ProcessContext.Parent.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.parent.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
		}, nil
	case "process.parent.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.BaseEvent.ProcessContext.Parent.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.parent.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.parent.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.BaseEvent.ProcessContext.Parent.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.ProcessContext.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.ProcessContext.Process.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "rename.file.destination.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.destination.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "rename.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "set.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set.registry.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set.registry.value_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.ValueName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.registry.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.registry.value_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.ValueName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "write.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "write.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "write.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "write.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "write.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "write.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFields() []eval.Field {
	return []eval.Field{
		"change_permission.new_sd",
		"change_permission.old_sd",
		"change_permission.path",
		"change_permission.type",
		"change_permission.user_domain",
		"change_permission.username",
		"container.created_at",
		"container.id",
		"container.runtime",
		"container.tags",
		"create.file.device_path",
		"create.file.device_path.length",
		"create.file.name",
		"create.file.name.length",
		"create.file.path",
		"create.file.path.length",
		"create.registry.key_name",
		"create.registry.key_name.length",
		"create.registry.key_path",
		"create.registry.key_path.length",
		"create_key.registry.key_name",
		"create_key.registry.key_name.length",
		"create_key.registry.key_path",
		"create_key.registry.key_path.length",
		"delete.file.device_path",
		"delete.file.device_path.length",
		"delete.file.name",
		"delete.file.name.length",
		"delete.file.path",
		"delete.file.path.length",
		"delete.registry.key_name",
		"delete.registry.key_name.length",
		"delete.registry.key_path",
		"delete.registry.key_path.length",
		"delete_key.registry.key_name",
		"delete_key.registry.key_name.length",
		"delete_key.registry.key_path",
		"delete_key.registry.key_path.length",
		"event.hostname",
		"event.origin",
		"event.os",
		"event.service",
		"event.timestamp",
		"exec.cmdline",
		"exec.container.id",
		"exec.created_at",
		"exec.envp",
		"exec.envs",
		"exec.file.name",
		"exec.file.name.length",
		"exec.file.path",
		"exec.file.path.length",
		"exec.pid",
		"exec.ppid",
		"exec.user",
		"exec.user_sid",
		"exit.cause",
		"exit.cmdline",
		"exit.code",
		"exit.container.id",
		"exit.created_at",
		"exit.envp",
		"exit.envs",
		"exit.file.name",
		"exit.file.name.length",
		"exit.file.path",
		"exit.file.path.length",
		"exit.pid",
		"exit.ppid",
		"exit.user",
		"exit.user_sid",
		"open.registry.key_name",
		"open.registry.key_name.length",
		"open.registry.key_path",
		"open.registry.key_path.length",
		"open_key.registry.key_name",
		"open_key.registry.key_name.length",
		"open_key.registry.key_path",
		"open_key.registry.key_path.length",
		"process.ancestors.cmdline",
		"process.ancestors.container.id",
		"process.ancestors.created_at",
		"process.ancestors.envp",
		"process.ancestors.envs",
		"process.ancestors.file.name",
		"process.ancestors.file.name.length",
		"process.ancestors.file.path",
		"process.ancestors.file.path.length",
		"process.ancestors.length",
		"process.ancestors.pid",
		"process.ancestors.ppid",
		"process.ancestors.user",
		"process.ancestors.user_sid",
		"process.cmdline",
		"process.container.id",
		"process.created_at",
		"process.envp",
		"process.envs",
		"process.file.name",
		"process.file.name.length",
		"process.file.path",
		"process.file.path.length",
		"process.parent.cmdline",
		"process.parent.container.id",
		"process.parent.created_at",
		"process.parent.envp",
		"process.parent.envs",
		"process.parent.file.name",
		"process.parent.file.name.length",
		"process.parent.file.path",
		"process.parent.file.path.length",
		"process.parent.pid",
		"process.parent.ppid",
		"process.parent.user",
		"process.parent.user_sid",
		"process.pid",
		"process.ppid",
		"process.user",
		"process.user_sid",
		"rename.file.destination.device_path",
		"rename.file.destination.device_path.length",
		"rename.file.destination.name",
		"rename.file.destination.name.length",
		"rename.file.destination.path",
		"rename.file.destination.path.length",
		"rename.file.device_path",
		"rename.file.device_path.length",
		"rename.file.name",
		"rename.file.name.length",
		"rename.file.path",
		"rename.file.path.length",
		"set.registry.key_name",
		"set.registry.key_name.length",
		"set.registry.key_path",
		"set.registry.key_path.length",
		"set.registry.value_name",
		"set.registry.value_name.length",
		"set.value_name",
		"set_key_value.registry.key_name",
		"set_key_value.registry.key_name.length",
		"set_key_value.registry.key_path",
		"set_key_value.registry.key_path.length",
		"set_key_value.registry.value_name",
		"set_key_value.registry.value_name.length",
		"set_key_value.value_name",
		"write.file.device_path",
		"write.file.device_path.length",
		"write.file.name",
		"write.file.name.length",
		"write.file.path",
		"write.file.path.length",
	}
}
func (ev *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "change_permission.new_sd":
		return ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission), nil
	case "change_permission.old_sd":
		return ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission), nil
	case "change_permission.path":
		return ev.ChangePermission.ObjectName, nil
	case "change_permission.type":
		return ev.ChangePermission.ObjectType, nil
	case "change_permission.user_domain":
		return ev.ChangePermission.UserDomain, nil
	case "change_permission.username":
		return ev.ChangePermission.UserName, nil
	case "container.created_at":
		return int(ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext)), nil
	case "container.id":
		return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext), nil
	case "container.runtime":
		return ev.FieldHandlers.ResolveContainerRuntime(ev, ev.BaseEvent.ContainerContext), nil
	case "container.tags":
		return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext), nil
	case "create.file.device_path":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File), nil
	case "create.file.device_path.length":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File), nil
	case "create.file.name":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File), nil
	case "create.file.name.length":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File), nil
	case "create.file.path":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File), nil
	case "create.file.path.length":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File), nil
	case "create.registry.key_name":
		return ev.CreateRegistryKey.Registry.KeyName, nil
	case "create.registry.key_name.length":
		return len(ev.CreateRegistryKey.Registry.KeyName), nil
	case "create.registry.key_path":
		return ev.CreateRegistryKey.Registry.KeyPath, nil
	case "create.registry.key_path.length":
		return len(ev.CreateRegistryKey.Registry.KeyPath), nil
	case "create_key.registry.key_name":
		return ev.CreateRegistryKey.Registry.KeyName, nil
	case "create_key.registry.key_name.length":
		return len(ev.CreateRegistryKey.Registry.KeyName), nil
	case "create_key.registry.key_path":
		return ev.CreateRegistryKey.Registry.KeyPath, nil
	case "create_key.registry.key_path.length":
		return len(ev.CreateRegistryKey.Registry.KeyPath), nil
	case "delete.file.device_path":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File), nil
	case "delete.file.device_path.length":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File), nil
	case "delete.file.name":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File), nil
	case "delete.file.name.length":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File), nil
	case "delete.file.path":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File), nil
	case "delete.file.path.length":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File), nil
	case "delete.registry.key_name":
		return ev.DeleteRegistryKey.Registry.KeyName, nil
	case "delete.registry.key_name.length":
		return len(ev.DeleteRegistryKey.Registry.KeyName), nil
	case "delete.registry.key_path":
		return ev.DeleteRegistryKey.Registry.KeyPath, nil
	case "delete.registry.key_path.length":
		return len(ev.DeleteRegistryKey.Registry.KeyPath), nil
	case "delete_key.registry.key_name":
		return ev.DeleteRegistryKey.Registry.KeyName, nil
	case "delete_key.registry.key_name.length":
		return len(ev.DeleteRegistryKey.Registry.KeyName), nil
	case "delete_key.registry.key_path":
		return ev.DeleteRegistryKey.Registry.KeyPath, nil
	case "delete_key.registry.key_path.length":
		return len(ev.DeleteRegistryKey.Registry.KeyPath), nil
	case "event.hostname":
		return ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent), nil
	case "event.origin":
		return ev.BaseEvent.Origin, nil
	case "event.os":
		return ev.BaseEvent.Os, nil
	case "event.service":
		return ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent), nil
	case "event.timestamp":
		return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)), nil
	case "exec.cmdline":
		return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process), nil
	case "exec.container.id":
		return ev.Exec.Process.ContainerID, nil
	case "exec.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process)), nil
	case "exec.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process), nil
	case "exec.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process), nil
	case "exec.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent), nil
	case "exec.pid":
		return int(ev.Exec.Process.PIDContext.Pid), nil
	case "exec.ppid":
		return int(ev.Exec.Process.PPid), nil
	case "exec.user":
		return ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process), nil
	case "exec.user_sid":
		return ev.Exec.Process.OwnerSidString, nil
	case "exit.cause":
		return int(ev.Exit.Cause), nil
	case "exit.cmdline":
		return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process), nil
	case "exit.code":
		return int(ev.Exit.Code), nil
	case "exit.container.id":
		return ev.Exit.Process.ContainerID, nil
	case "exit.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process)), nil
	case "exit.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process), nil
	case "exit.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process), nil
	case "exit.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent), nil
	case "exit.pid":
		return int(ev.Exit.Process.PIDContext.Pid), nil
	case "exit.ppid":
		return int(ev.Exit.Process.PPid), nil
	case "exit.user":
		return ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process), nil
	case "exit.user_sid":
		return ev.Exit.Process.OwnerSidString, nil
	case "open.registry.key_name":
		return ev.OpenRegistryKey.Registry.KeyName, nil
	case "open.registry.key_name.length":
		return len(ev.OpenRegistryKey.Registry.KeyName), nil
	case "open.registry.key_path":
		return ev.OpenRegistryKey.Registry.KeyPath, nil
	case "open.registry.key_path.length":
		return len(ev.OpenRegistryKey.Registry.KeyPath), nil
	case "open_key.registry.key_name":
		return ev.OpenRegistryKey.Registry.KeyName, nil
	case "open_key.registry.key_name.length":
		return len(ev.OpenRegistryKey.Registry.KeyName), nil
	case "open_key.registry.key_path":
		return ev.OpenRegistryKey.Registry.KeyPath, nil
	case "open_key.registry.key_path.length":
		return len(ev.OpenRegistryKey.Registry.KeyPath), nil
	case "process.ancestors.cmdline":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.container.id":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := element.ProcessContext.Process.ContainerID
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.created_at":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envp":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.envs":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
			values = append(values, result...)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.name":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent), nil
	case "process.ancestors.file.path":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent), nil
	case "process.ancestors.length":
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		return iterator.Len(ctx), nil
	case "process.ancestors.pid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := int(element.ProcessContext.Process.PIDContext.Pid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.ppid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := int(element.ProcessContext.Process.PPid)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.user":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.user_sid":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := ptr
			result := element.ProcessContext.Process.OwnerSidString
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.cmdline":
		return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process), nil
	case "process.container.id":
		return ev.BaseEvent.ProcessContext.Process.ContainerID, nil
	case "process.created_at":
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process)), nil
	case "process.envp":
		return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process), nil
	case "process.envs":
		return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process), nil
	case "process.file.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent), nil
	case "process.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent), nil
	case "process.file.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent), nil
	case "process.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent), nil
	case "process.parent.cmdline":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return "", &eval.ErrNotSupported{Field: field}
		}
		return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent), nil
	case "process.parent.container.id":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return "", &eval.ErrNotSupported{Field: field}
		}
		return ev.BaseEvent.ProcessContext.Parent.ContainerID, nil
	case "process.parent.created_at":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return 0, &eval.ErrNotSupported{Field: field}
		}
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent)), nil
	case "process.parent.envp":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return []string{}, &eval.ErrNotSupported{Field: field}
		}
		return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent), nil
	case "process.parent.envs":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return []string{}, &eval.ErrNotSupported{Field: field}
		}
		return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent), nil
	case "process.parent.file.name":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return "", &eval.ErrNotSupported{Field: field}
		}
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.path":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return "", &eval.ErrNotSupported{Field: field}
		}
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent), nil
	case "process.parent.file.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent), nil
	case "process.parent.pid":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return 0, &eval.ErrNotSupported{Field: field}
		}
		return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid), nil
	case "process.parent.ppid":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return 0, &eval.ErrNotSupported{Field: field}
		}
		return int(ev.BaseEvent.ProcessContext.Parent.PPid), nil
	case "process.parent.user":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return "", &eval.ErrNotSupported{Field: field}
		}
		return ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent), nil
	case "process.parent.user_sid":
		if !ev.BaseEvent.ProcessContext.HasParent() {
			return "", &eval.ErrNotSupported{Field: field}
		}
		return ev.BaseEvent.ProcessContext.Parent.OwnerSidString, nil
	case "process.pid":
		return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid), nil
	case "process.ppid":
		return int(ev.BaseEvent.ProcessContext.Process.PPid), nil
	case "process.user":
		return ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process), nil
	case "process.user_sid":
		return ev.BaseEvent.ProcessContext.Process.OwnerSidString, nil
	case "rename.file.destination.device_path":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New), nil
	case "rename.file.destination.device_path.length":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New), nil
	case "rename.file.destination.name":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New), nil
	case "rename.file.destination.name.length":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New), nil
	case "rename.file.destination.path":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New), nil
	case "rename.file.destination.path.length":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New), nil
	case "rename.file.device_path":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old), nil
	case "rename.file.device_path.length":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old), nil
	case "rename.file.name":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old), nil
	case "rename.file.name.length":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old), nil
	case "rename.file.path":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old), nil
	case "rename.file.path.length":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old), nil
	case "set.registry.key_name":
		return ev.SetRegistryKeyValue.Registry.KeyName, nil
	case "set.registry.key_name.length":
		return len(ev.SetRegistryKeyValue.Registry.KeyName), nil
	case "set.registry.key_path":
		return ev.SetRegistryKeyValue.Registry.KeyPath, nil
	case "set.registry.key_path.length":
		return len(ev.SetRegistryKeyValue.Registry.KeyPath), nil
	case "set.registry.value_name":
		return ev.SetRegistryKeyValue.ValueName, nil
	case "set.registry.value_name.length":
		return len(ev.SetRegistryKeyValue.ValueName), nil
	case "set.value_name":
		return ev.SetRegistryKeyValue.ValueName, nil
	case "set_key_value.registry.key_name":
		return ev.SetRegistryKeyValue.Registry.KeyName, nil
	case "set_key_value.registry.key_name.length":
		return len(ev.SetRegistryKeyValue.Registry.KeyName), nil
	case "set_key_value.registry.key_path":
		return ev.SetRegistryKeyValue.Registry.KeyPath, nil
	case "set_key_value.registry.key_path.length":
		return len(ev.SetRegistryKeyValue.Registry.KeyPath), nil
	case "set_key_value.registry.value_name":
		return ev.SetRegistryKeyValue.ValueName, nil
	case "set_key_value.registry.value_name.length":
		return len(ev.SetRegistryKeyValue.ValueName), nil
	case "set_key_value.value_name":
		return ev.SetRegistryKeyValue.ValueName, nil
	case "write.file.device_path":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File), nil
	case "write.file.device_path.length":
		return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File), nil
	case "write.file.name":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File), nil
	case "write.file.name.length":
		return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File), nil
	case "write.file.path":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File), nil
	case "write.file.path.length":
		return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File), nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {
	case "change_permission.new_sd":
		return "change_permission", nil
	case "change_permission.old_sd":
		return "change_permission", nil
	case "change_permission.path":
		return "change_permission", nil
	case "change_permission.type":
		return "change_permission", nil
	case "change_permission.user_domain":
		return "change_permission", nil
	case "change_permission.username":
		return "change_permission", nil
	case "container.created_at":
		return "", nil
	case "container.id":
		return "", nil
	case "container.runtime":
		return "", nil
	case "container.tags":
		return "", nil
	case "create.file.device_path":
		return "create", nil
	case "create.file.device_path.length":
		return "create", nil
	case "create.file.name":
		return "create", nil
	case "create.file.name.length":
		return "create", nil
	case "create.file.path":
		return "create", nil
	case "create.file.path.length":
		return "create", nil
	case "create.registry.key_name":
		return "create_key", nil
	case "create.registry.key_name.length":
		return "create_key", nil
	case "create.registry.key_path":
		return "create_key", nil
	case "create.registry.key_path.length":
		return "create_key", nil
	case "create_key.registry.key_name":
		return "create_key", nil
	case "create_key.registry.key_name.length":
		return "create_key", nil
	case "create_key.registry.key_path":
		return "create_key", nil
	case "create_key.registry.key_path.length":
		return "create_key", nil
	case "delete.file.device_path":
		return "delete", nil
	case "delete.file.device_path.length":
		return "delete", nil
	case "delete.file.name":
		return "delete", nil
	case "delete.file.name.length":
		return "delete", nil
	case "delete.file.path":
		return "delete", nil
	case "delete.file.path.length":
		return "delete", nil
	case "delete.registry.key_name":
		return "delete_key", nil
	case "delete.registry.key_name.length":
		return "delete_key", nil
	case "delete.registry.key_path":
		return "delete_key", nil
	case "delete.registry.key_path.length":
		return "delete_key", nil
	case "delete_key.registry.key_name":
		return "delete_key", nil
	case "delete_key.registry.key_name.length":
		return "delete_key", nil
	case "delete_key.registry.key_path":
		return "delete_key", nil
	case "delete_key.registry.key_path.length":
		return "delete_key", nil
	case "event.hostname":
		return "", nil
	case "event.origin":
		return "", nil
	case "event.os":
		return "", nil
	case "event.service":
		return "", nil
	case "event.timestamp":
		return "", nil
	case "exec.cmdline":
		return "exec", nil
	case "exec.container.id":
		return "exec", nil
	case "exec.created_at":
		return "exec", nil
	case "exec.envp":
		return "exec", nil
	case "exec.envs":
		return "exec", nil
	case "exec.file.name":
		return "exec", nil
	case "exec.file.name.length":
		return "exec", nil
	case "exec.file.path":
		return "exec", nil
	case "exec.file.path.length":
		return "exec", nil
	case "exec.pid":
		return "exec", nil
	case "exec.ppid":
		return "exec", nil
	case "exec.user":
		return "exec", nil
	case "exec.user_sid":
		return "exec", nil
	case "exit.cause":
		return "exit", nil
	case "exit.cmdline":
		return "exit", nil
	case "exit.code":
		return "exit", nil
	case "exit.container.id":
		return "exit", nil
	case "exit.created_at":
		return "exit", nil
	case "exit.envp":
		return "exit", nil
	case "exit.envs":
		return "exit", nil
	case "exit.file.name":
		return "exit", nil
	case "exit.file.name.length":
		return "exit", nil
	case "exit.file.path":
		return "exit", nil
	case "exit.file.path.length":
		return "exit", nil
	case "exit.pid":
		return "exit", nil
	case "exit.ppid":
		return "exit", nil
	case "exit.user":
		return "exit", nil
	case "exit.user_sid":
		return "exit", nil
	case "open.registry.key_name":
		return "open_key", nil
	case "open.registry.key_name.length":
		return "open_key", nil
	case "open.registry.key_path":
		return "open_key", nil
	case "open.registry.key_path.length":
		return "open_key", nil
	case "open_key.registry.key_name":
		return "open_key", nil
	case "open_key.registry.key_name.length":
		return "open_key", nil
	case "open_key.registry.key_path":
		return "open_key", nil
	case "open_key.registry.key_path.length":
		return "open_key", nil
	case "process.ancestors.cmdline":
		return "", nil
	case "process.ancestors.container.id":
		return "", nil
	case "process.ancestors.created_at":
		return "", nil
	case "process.ancestors.envp":
		return "", nil
	case "process.ancestors.envs":
		return "", nil
	case "process.ancestors.file.name":
		return "", nil
	case "process.ancestors.file.name.length":
		return "", nil
	case "process.ancestors.file.path":
		return "", nil
	case "process.ancestors.file.path.length":
		return "", nil
	case "process.ancestors.length":
		return "", nil
	case "process.ancestors.pid":
		return "", nil
	case "process.ancestors.ppid":
		return "", nil
	case "process.ancestors.user":
		return "", nil
	case "process.ancestors.user_sid":
		return "", nil
	case "process.cmdline":
		return "", nil
	case "process.container.id":
		return "", nil
	case "process.created_at":
		return "", nil
	case "process.envp":
		return "", nil
	case "process.envs":
		return "", nil
	case "process.file.name":
		return "", nil
	case "process.file.name.length":
		return "", nil
	case "process.file.path":
		return "", nil
	case "process.file.path.length":
		return "", nil
	case "process.parent.cmdline":
		return "", nil
	case "process.parent.container.id":
		return "", nil
	case "process.parent.created_at":
		return "", nil
	case "process.parent.envp":
		return "", nil
	case "process.parent.envs":
		return "", nil
	case "process.parent.file.name":
		return "", nil
	case "process.parent.file.name.length":
		return "", nil
	case "process.parent.file.path":
		return "", nil
	case "process.parent.file.path.length":
		return "", nil
	case "process.parent.pid":
		return "", nil
	case "process.parent.ppid":
		return "", nil
	case "process.parent.user":
		return "", nil
	case "process.parent.user_sid":
		return "", nil
	case "process.pid":
		return "", nil
	case "process.ppid":
		return "", nil
	case "process.user":
		return "", nil
	case "process.user_sid":
		return "", nil
	case "rename.file.destination.device_path":
		return "rename", nil
	case "rename.file.destination.device_path.length":
		return "rename", nil
	case "rename.file.destination.name":
		return "rename", nil
	case "rename.file.destination.name.length":
		return "rename", nil
	case "rename.file.destination.path":
		return "rename", nil
	case "rename.file.destination.path.length":
		return "rename", nil
	case "rename.file.device_path":
		return "rename", nil
	case "rename.file.device_path.length":
		return "rename", nil
	case "rename.file.name":
		return "rename", nil
	case "rename.file.name.length":
		return "rename", nil
	case "rename.file.path":
		return "rename", nil
	case "rename.file.path.length":
		return "rename", nil
	case "set.registry.key_name":
		return "set_key_value", nil
	case "set.registry.key_name.length":
		return "set_key_value", nil
	case "set.registry.key_path":
		return "set_key_value", nil
	case "set.registry.key_path.length":
		return "set_key_value", nil
	case "set.registry.value_name":
		return "set_key_value", nil
	case "set.registry.value_name.length":
		return "set_key_value", nil
	case "set.value_name":
		return "set_key_value", nil
	case "set_key_value.registry.key_name":
		return "set_key_value", nil
	case "set_key_value.registry.key_name.length":
		return "set_key_value", nil
	case "set_key_value.registry.key_path":
		return "set_key_value", nil
	case "set_key_value.registry.key_path.length":
		return "set_key_value", nil
	case "set_key_value.registry.value_name":
		return "set_key_value", nil
	case "set_key_value.registry.value_name.length":
		return "set_key_value", nil
	case "set_key_value.value_name":
		return "set_key_value", nil
	case "write.file.device_path":
		return "write", nil
	case "write.file.device_path.length":
		return "write", nil
	case "write.file.name":
		return "write", nil
	case "write.file.name.length":
		return "write", nil
	case "write.file.path":
		return "write", nil
	case "write.file.path.length":
		return "write", nil
	}
	return "", &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
	case "change_permission.new_sd":
		return reflect.String, nil
	case "change_permission.old_sd":
		return reflect.String, nil
	case "change_permission.path":
		return reflect.String, nil
	case "change_permission.type":
		return reflect.String, nil
	case "change_permission.user_domain":
		return reflect.String, nil
	case "change_permission.username":
		return reflect.String, nil
	case "container.created_at":
		return reflect.Int, nil
	case "container.id":
		return reflect.String, nil
	case "container.runtime":
		return reflect.String, nil
	case "container.tags":
		return reflect.String, nil
	case "create.file.device_path":
		return reflect.String, nil
	case "create.file.device_path.length":
		return reflect.Int, nil
	case "create.file.name":
		return reflect.String, nil
	case "create.file.name.length":
		return reflect.Int, nil
	case "create.file.path":
		return reflect.String, nil
	case "create.file.path.length":
		return reflect.Int, nil
	case "create.registry.key_name":
		return reflect.String, nil
	case "create.registry.key_name.length":
		return reflect.Int, nil
	case "create.registry.key_path":
		return reflect.String, nil
	case "create.registry.key_path.length":
		return reflect.Int, nil
	case "create_key.registry.key_name":
		return reflect.String, nil
	case "create_key.registry.key_name.length":
		return reflect.Int, nil
	case "create_key.registry.key_path":
		return reflect.String, nil
	case "create_key.registry.key_path.length":
		return reflect.Int, nil
	case "delete.file.device_path":
		return reflect.String, nil
	case "delete.file.device_path.length":
		return reflect.Int, nil
	case "delete.file.name":
		return reflect.String, nil
	case "delete.file.name.length":
		return reflect.Int, nil
	case "delete.file.path":
		return reflect.String, nil
	case "delete.file.path.length":
		return reflect.Int, nil
	case "delete.registry.key_name":
		return reflect.String, nil
	case "delete.registry.key_name.length":
		return reflect.Int, nil
	case "delete.registry.key_path":
		return reflect.String, nil
	case "delete.registry.key_path.length":
		return reflect.Int, nil
	case "delete_key.registry.key_name":
		return reflect.String, nil
	case "delete_key.registry.key_name.length":
		return reflect.Int, nil
	case "delete_key.registry.key_path":
		return reflect.String, nil
	case "delete_key.registry.key_path.length":
		return reflect.Int, nil
	case "event.hostname":
		return reflect.String, nil
	case "event.origin":
		return reflect.String, nil
	case "event.os":
		return reflect.String, nil
	case "event.service":
		return reflect.String, nil
	case "event.timestamp":
		return reflect.Int, nil
	case "exec.cmdline":
		return reflect.String, nil
	case "exec.container.id":
		return reflect.String, nil
	case "exec.created_at":
		return reflect.Int, nil
	case "exec.envp":
		return reflect.String, nil
	case "exec.envs":
		return reflect.String, nil
	case "exec.file.name":
		return reflect.String, nil
	case "exec.file.name.length":
		return reflect.Int, nil
	case "exec.file.path":
		return reflect.String, nil
	case "exec.file.path.length":
		return reflect.Int, nil
	case "exec.pid":
		return reflect.Int, nil
	case "exec.ppid":
		return reflect.Int, nil
	case "exec.user":
		return reflect.String, nil
	case "exec.user_sid":
		return reflect.String, nil
	case "exit.cause":
		return reflect.Int, nil
	case "exit.cmdline":
		return reflect.String, nil
	case "exit.code":
		return reflect.Int, nil
	case "exit.container.id":
		return reflect.String, nil
	case "exit.created_at":
		return reflect.Int, nil
	case "exit.envp":
		return reflect.String, nil
	case "exit.envs":
		return reflect.String, nil
	case "exit.file.name":
		return reflect.String, nil
	case "exit.file.name.length":
		return reflect.Int, nil
	case "exit.file.path":
		return reflect.String, nil
	case "exit.file.path.length":
		return reflect.Int, nil
	case "exit.pid":
		return reflect.Int, nil
	case "exit.ppid":
		return reflect.Int, nil
	case "exit.user":
		return reflect.String, nil
	case "exit.user_sid":
		return reflect.String, nil
	case "open.registry.key_name":
		return reflect.String, nil
	case "open.registry.key_name.length":
		return reflect.Int, nil
	case "open.registry.key_path":
		return reflect.String, nil
	case "open.registry.key_path.length":
		return reflect.Int, nil
	case "open_key.registry.key_name":
		return reflect.String, nil
	case "open_key.registry.key_name.length":
		return reflect.Int, nil
	case "open_key.registry.key_path":
		return reflect.String, nil
	case "open_key.registry.key_path.length":
		return reflect.Int, nil
	case "process.ancestors.cmdline":
		return reflect.String, nil
	case "process.ancestors.container.id":
		return reflect.String, nil
	case "process.ancestors.created_at":
		return reflect.Int, nil
	case "process.ancestors.envp":
		return reflect.String, nil
	case "process.ancestors.envs":
		return reflect.String, nil
	case "process.ancestors.file.name":
		return reflect.String, nil
	case "process.ancestors.file.name.length":
		return reflect.Int, nil
	case "process.ancestors.file.path":
		return reflect.String, nil
	case "process.ancestors.file.path.length":
		return reflect.Int, nil
	case "process.ancestors.length":
		return reflect.Int, nil
	case "process.ancestors.pid":
		return reflect.Int, nil
	case "process.ancestors.ppid":
		return reflect.Int, nil
	case "process.ancestors.user":
		return reflect.String, nil
	case "process.ancestors.user_sid":
		return reflect.String, nil
	case "process.cmdline":
		return reflect.String, nil
	case "process.container.id":
		return reflect.String, nil
	case "process.created_at":
		return reflect.Int, nil
	case "process.envp":
		return reflect.String, nil
	case "process.envs":
		return reflect.String, nil
	case "process.file.name":
		return reflect.String, nil
	case "process.file.name.length":
		return reflect.Int, nil
	case "process.file.path":
		return reflect.String, nil
	case "process.file.path.length":
		return reflect.Int, nil
	case "process.parent.cmdline":
		return reflect.String, nil
	case "process.parent.container.id":
		return reflect.String, nil
	case "process.parent.created_at":
		return reflect.Int, nil
	case "process.parent.envp":
		return reflect.String, nil
	case "process.parent.envs":
		return reflect.String, nil
	case "process.parent.file.name":
		return reflect.String, nil
	case "process.parent.file.name.length":
		return reflect.Int, nil
	case "process.parent.file.path":
		return reflect.String, nil
	case "process.parent.file.path.length":
		return reflect.Int, nil
	case "process.parent.pid":
		return reflect.Int, nil
	case "process.parent.ppid":
		return reflect.Int, nil
	case "process.parent.user":
		return reflect.String, nil
	case "process.parent.user_sid":
		return reflect.String, nil
	case "process.pid":
		return reflect.Int, nil
	case "process.ppid":
		return reflect.Int, nil
	case "process.user":
		return reflect.String, nil
	case "process.user_sid":
		return reflect.String, nil
	case "rename.file.destination.device_path":
		return reflect.String, nil
	case "rename.file.destination.device_path.length":
		return reflect.Int, nil
	case "rename.file.destination.name":
		return reflect.String, nil
	case "rename.file.destination.name.length":
		return reflect.Int, nil
	case "rename.file.destination.path":
		return reflect.String, nil
	case "rename.file.destination.path.length":
		return reflect.Int, nil
	case "rename.file.device_path":
		return reflect.String, nil
	case "rename.file.device_path.length":
		return reflect.Int, nil
	case "rename.file.name":
		return reflect.String, nil
	case "rename.file.name.length":
		return reflect.Int, nil
	case "rename.file.path":
		return reflect.String, nil
	case "rename.file.path.length":
		return reflect.Int, nil
	case "set.registry.key_name":
		return reflect.String, nil
	case "set.registry.key_name.length":
		return reflect.Int, nil
	case "set.registry.key_path":
		return reflect.String, nil
	case "set.registry.key_path.length":
		return reflect.Int, nil
	case "set.registry.value_name":
		return reflect.String, nil
	case "set.registry.value_name.length":
		return reflect.Int, nil
	case "set.value_name":
		return reflect.String, nil
	case "set_key_value.registry.key_name":
		return reflect.String, nil
	case "set_key_value.registry.key_name.length":
		return reflect.Int, nil
	case "set_key_value.registry.key_path":
		return reflect.String, nil
	case "set_key_value.registry.key_path.length":
		return reflect.Int, nil
	case "set_key_value.registry.value_name":
		return reflect.String, nil
	case "set_key_value.registry.value_name.length":
		return reflect.Int, nil
	case "set_key_value.value_name":
		return reflect.String, nil
	case "write.file.device_path":
		return reflect.String, nil
	case "write.file.device_path.length":
		return reflect.Int, nil
	case "write.file.name":
		return reflect.String, nil
	case "write.file.name.length":
		return reflect.Int, nil
	case "write.file.path":
		return reflect.String, nil
	case "write.file.path.length":
		return reflect.Int, nil
	}
	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {
	case "change_permission.new_sd":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ChangePermission.NewSd"}
		}
		ev.ChangePermission.NewSd = rv
		return nil
	case "change_permission.old_sd":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ChangePermission.OldSd"}
		}
		ev.ChangePermission.OldSd = rv
		return nil
	case "change_permission.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ChangePermission.ObjectName"}
		}
		ev.ChangePermission.ObjectName = rv
		return nil
	case "change_permission.type":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ChangePermission.ObjectType"}
		}
		ev.ChangePermission.ObjectType = rv
		return nil
	case "change_permission.user_domain":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ChangePermission.UserDomain"}
		}
		ev.ChangePermission.UserDomain = rv
		return nil
	case "change_permission.username":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "ChangePermission.UserName"}
		}
		ev.ChangePermission.UserName = rv
		return nil
	case "container.created_at":
		if ev.BaseEvent.ContainerContext == nil {
			ev.BaseEvent.ContainerContext = &ContainerContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ContainerContext.CreatedAt"}
		}
		ev.BaseEvent.ContainerContext.CreatedAt = uint64(rv)
		return nil
	case "container.id":
		if ev.BaseEvent.ContainerContext == nil {
			ev.BaseEvent.ContainerContext = &ContainerContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ContainerContext.ContainerID"}
		}
		ev.BaseEvent.ContainerContext.ContainerID = containerutils.ContainerID(rv)
		return nil
	case "container.runtime":
		if ev.BaseEvent.ContainerContext == nil {
			ev.BaseEvent.ContainerContext = &ContainerContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ContainerContext.Runtime"}
		}
		ev.BaseEvent.ContainerContext.Runtime = rv
		return nil
	case "container.tags":
		if ev.BaseEvent.ContainerContext == nil {
			ev.BaseEvent.ContainerContext = &ContainerContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ContainerContext.Tags = append(ev.BaseEvent.ContainerContext.Tags, rv)
		case []string:
			ev.BaseEvent.ContainerContext.Tags = append(ev.BaseEvent.ContainerContext.Tags, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ContainerContext.Tags"}
		}
		return nil
	case "create.file.device_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateNewFile.File.PathnameStr"}
		}
		ev.CreateNewFile.File.PathnameStr = rv
		return nil
	case "create.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "create.file.device_path.length"}
	case "create.file.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateNewFile.File.BasenameStr"}
		}
		ev.CreateNewFile.File.BasenameStr = rv
		return nil
	case "create.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "create.file.name.length"}
	case "create.file.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateNewFile.File.UserPathnameStr"}
		}
		ev.CreateNewFile.File.UserPathnameStr = rv
		return nil
	case "create.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "create.file.path.length"}
	case "create.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.Registry.KeyName"}
		}
		ev.CreateRegistryKey.Registry.KeyName = rv
		return nil
	case "create.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "create.registry.key_name.length"}
	case "create.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.Registry.KeyPath"}
		}
		ev.CreateRegistryKey.Registry.KeyPath = rv
		return nil
	case "create.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "create.registry.key_path.length"}
	case "create_key.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.Registry.KeyName"}
		}
		ev.CreateRegistryKey.Registry.KeyName = rv
		return nil
	case "create_key.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.registry.key_name.length"}
	case "create_key.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.Registry.KeyPath"}
		}
		ev.CreateRegistryKey.Registry.KeyPath = rv
		return nil
	case "create_key.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.registry.key_path.length"}
	case "delete.file.device_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteFile.File.PathnameStr"}
		}
		ev.DeleteFile.File.PathnameStr = rv
		return nil
	case "delete.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "delete.file.device_path.length"}
	case "delete.file.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteFile.File.BasenameStr"}
		}
		ev.DeleteFile.File.BasenameStr = rv
		return nil
	case "delete.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "delete.file.name.length"}
	case "delete.file.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteFile.File.UserPathnameStr"}
		}
		ev.DeleteFile.File.UserPathnameStr = rv
		return nil
	case "delete.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "delete.file.path.length"}
	case "delete.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.Registry.KeyName"}
		}
		ev.DeleteRegistryKey.Registry.KeyName = rv
		return nil
	case "delete.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "delete.registry.key_name.length"}
	case "delete.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.Registry.KeyPath"}
		}
		ev.DeleteRegistryKey.Registry.KeyPath = rv
		return nil
	case "delete.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "delete.registry.key_path.length"}
	case "delete_key.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.Registry.KeyName"}
		}
		ev.DeleteRegistryKey.Registry.KeyName = rv
		return nil
	case "delete_key.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.registry.key_name.length"}
	case "delete_key.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.Registry.KeyPath"}
		}
		ev.DeleteRegistryKey.Registry.KeyPath = rv
		return nil
	case "delete_key.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.registry.key_path.length"}
	case "event.hostname":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.Hostname"}
		}
		ev.BaseEvent.Hostname = rv
		return nil
	case "event.origin":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.Origin"}
		}
		ev.BaseEvent.Origin = rv
		return nil
	case "event.os":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.Os"}
		}
		ev.BaseEvent.Os = rv
		return nil
	case "event.service":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.Service"}
		}
		ev.BaseEvent.Service = rv
		return nil
	case "event.timestamp":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.TimestampRaw"}
		}
		ev.BaseEvent.TimestampRaw = uint64(rv)
		return nil
	case "exec.cmdline":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.CmdLine"}
		}
		ev.Exec.Process.CmdLine = rv
		return nil
	case "exec.container.id":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.ContainerID"}
		}
		ev.Exec.Process.ContainerID = rv
		return nil
	case "exec.created_at":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.CreatedAt"}
		}
		ev.Exec.Process.CreatedAt = uint64(rv)
		return nil
	case "exec.envp":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Envp = append(ev.Exec.Process.Envp, rv)
		case []string:
			ev.Exec.Process.Envp = append(ev.Exec.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envp"}
		}
		return nil
	case "exec.envs":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exec.Process.Envs = append(ev.Exec.Process.Envs, rv)
		case []string:
			ev.Exec.Process.Envs = append(ev.Exec.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.Envs"}
		}
		return nil
	case "exec.file.name":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.BasenameStr"}
		}
		ev.Exec.Process.FileEvent.BasenameStr = rv
		return nil
	case "exec.file.name.length":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.file.name.length"}
	case "exec.file.path":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.FileEvent.PathnameStr"}
		}
		ev.Exec.Process.FileEvent.PathnameStr = rv
		return nil
	case "exec.file.path.length":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exec.file.path.length"}
	case "exec.pid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PIDContext.Pid"}
		}
		ev.Exec.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "exec.ppid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.PPid"}
		}
		ev.Exec.Process.PPid = uint32(rv)
		return nil
	case "exec.user":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.User"}
		}
		ev.Exec.Process.User = rv
		return nil
	case "exec.user_sid":
		if ev.Exec.Process == nil {
			ev.Exec.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exec.Process.OwnerSidString"}
		}
		ev.Exec.Process.OwnerSidString = rv
		return nil
	case "exit.cause":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Cause"}
		}
		ev.Exit.Cause = uint32(rv)
		return nil
	case "exit.cmdline":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.CmdLine"}
		}
		ev.Exit.Process.CmdLine = rv
		return nil
	case "exit.code":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Code"}
		}
		ev.Exit.Code = uint32(rv)
		return nil
	case "exit.container.id":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.ContainerID"}
		}
		ev.Exit.Process.ContainerID = rv
		return nil
	case "exit.created_at":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.CreatedAt"}
		}
		ev.Exit.Process.CreatedAt = uint64(rv)
		return nil
	case "exit.envp":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Envp = append(ev.Exit.Process.Envp, rv)
		case []string:
			ev.Exit.Process.Envp = append(ev.Exit.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Envp"}
		}
		return nil
	case "exit.envs":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.Exit.Process.Envs = append(ev.Exit.Process.Envs, rv)
		case []string:
			ev.Exit.Process.Envs = append(ev.Exit.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.Envs"}
		}
		return nil
	case "exit.file.name":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.BasenameStr"}
		}
		ev.Exit.Process.FileEvent.BasenameStr = rv
		return nil
	case "exit.file.name.length":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.file.name.length"}
	case "exit.file.path":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.FileEvent.PathnameStr"}
		}
		ev.Exit.Process.FileEvent.PathnameStr = rv
		return nil
	case "exit.file.path.length":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "exit.file.path.length"}
	case "exit.pid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PIDContext.Pid"}
		}
		ev.Exit.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "exit.ppid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.PPid"}
		}
		ev.Exit.Process.PPid = uint32(rv)
		return nil
	case "exit.user":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.User"}
		}
		ev.Exit.Process.User = rv
		return nil
	case "exit.user_sid":
		if ev.Exit.Process == nil {
			ev.Exit.Process = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "Exit.Process.OwnerSidString"}
		}
		ev.Exit.Process.OwnerSidString = rv
		return nil
	case "open.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.Registry.KeyName"}
		}
		ev.OpenRegistryKey.Registry.KeyName = rv
		return nil
	case "open.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "open.registry.key_name.length"}
	case "open.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.Registry.KeyPath"}
		}
		ev.OpenRegistryKey.Registry.KeyPath = rv
		return nil
	case "open.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "open.registry.key_path.length"}
	case "open_key.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.Registry.KeyName"}
		}
		ev.OpenRegistryKey.Registry.KeyName = rv
		return nil
	case "open_key.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.registry.key_name.length"}
	case "open_key.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.Registry.KeyPath"}
		}
		ev.OpenRegistryKey.Registry.KeyPath = rv
		return nil
	case "open_key.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.registry.key_path.length"}
	case "process.ancestors.cmdline":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.CmdLine"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.CmdLine = rv
		return nil
	case "process.ancestors.container.id":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.ContainerID"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.ContainerID = rv
		return nil
	case "process.ancestors.created_at":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.CreatedAt"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.CreatedAt = uint64(rv)
		return nil
	case "process.ancestors.envp":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envp = append(ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envp, rv)
		case []string:
			ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envp = append(ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envp"}
		}
		return nil
	case "process.ancestors.envs":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envs = append(ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envs, rv)
		case []string:
			ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envs = append(ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envs"}
		}
		return nil
	case "process.ancestors.file.name":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr = rv
		return nil
	case "process.ancestors.file.name.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.name.length"}
	case "process.ancestors.file.path":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr = rv
		return nil
	case "process.ancestors.file.path.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.path.length"}
	case "process.ancestors.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.length"}
	case "process.ancestors.pid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "process.ancestors.ppid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.PPid"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.PPid = uint32(rv)
		return nil
	case "process.ancestors.user":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.User"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.User = rv
		return nil
	case "process.ancestors.user_sid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Ancestor == nil {
			ev.BaseEvent.ProcessContext.Ancestor = &ProcessCacheEntry{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.OwnerSidString"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.OwnerSidString = rv
		return nil
	case "process.cmdline":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.CmdLine"}
		}
		ev.BaseEvent.ProcessContext.Process.CmdLine = rv
		return nil
	case "process.container.id":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.ContainerID"}
		}
		ev.BaseEvent.ProcessContext.Process.ContainerID = rv
		return nil
	case "process.created_at":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.CreatedAt"}
		}
		ev.BaseEvent.ProcessContext.Process.CreatedAt = uint64(rv)
		return nil
	case "process.envp":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ProcessContext.Process.Envp = append(ev.BaseEvent.ProcessContext.Process.Envp, rv)
		case []string:
			ev.BaseEvent.ProcessContext.Process.Envp = append(ev.BaseEvent.ProcessContext.Process.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.Envp"}
		}
		return nil
	case "process.envs":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ProcessContext.Process.Envs = append(ev.BaseEvent.ProcessContext.Process.Envs, rv)
		case []string:
			ev.BaseEvent.ProcessContext.Process.Envs = append(ev.BaseEvent.ProcessContext.Process.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.Envs"}
		}
		return nil
	case "process.file.name":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.FileEvent.BasenameStr"}
		}
		ev.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr = rv
		return nil
	case "process.file.name.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.file.name.length"}
	case "process.file.path":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.FileEvent.PathnameStr"}
		}
		ev.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr = rv
		return nil
	case "process.file.path.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.file.path.length"}
	case "process.parent.cmdline":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.CmdLine"}
		}
		ev.BaseEvent.ProcessContext.Parent.CmdLine = rv
		return nil
	case "process.parent.container.id":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.ContainerID"}
		}
		ev.BaseEvent.ProcessContext.Parent.ContainerID = rv
		return nil
	case "process.parent.created_at":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.CreatedAt"}
		}
		ev.BaseEvent.ProcessContext.Parent.CreatedAt = uint64(rv)
		return nil
	case "process.parent.envp":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ProcessContext.Parent.Envp = append(ev.BaseEvent.ProcessContext.Parent.Envp, rv)
		case []string:
			ev.BaseEvent.ProcessContext.Parent.Envp = append(ev.BaseEvent.ProcessContext.Parent.Envp, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.Envp"}
		}
		return nil
	case "process.parent.envs":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		switch rv := value.(type) {
		case string:
			ev.BaseEvent.ProcessContext.Parent.Envs = append(ev.BaseEvent.ProcessContext.Parent.Envs, rv)
		case []string:
			ev.BaseEvent.ProcessContext.Parent.Envs = append(ev.BaseEvent.ProcessContext.Parent.Envs, rv...)
		default:
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.Envs"}
		}
		return nil
	case "process.parent.file.name":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.FileEvent.BasenameStr"}
		}
		ev.BaseEvent.ProcessContext.Parent.FileEvent.BasenameStr = rv
		return nil
	case "process.parent.file.name.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.parent.file.name.length"}
	case "process.parent.file.path":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.FileEvent.PathnameStr"}
		}
		ev.BaseEvent.ProcessContext.Parent.FileEvent.PathnameStr = rv
		return nil
	case "process.parent.file.path.length":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		return &eval.ErrFieldReadOnly{Field: "process.parent.file.path.length"}
	case "process.parent.pid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.PIDContext.Pid"}
		}
		ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid = uint32(rv)
		return nil
	case "process.parent.ppid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.PPid"}
		}
		ev.BaseEvent.ProcessContext.Parent.PPid = uint32(rv)
		return nil
	case "process.parent.user":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.User"}
		}
		ev.BaseEvent.ProcessContext.Parent.User = rv
		return nil
	case "process.parent.user_sid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		if ev.BaseEvent.ProcessContext.Parent == nil {
			ev.BaseEvent.ProcessContext.Parent = &Process{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Parent.OwnerSidString"}
		}
		ev.BaseEvent.ProcessContext.Parent.OwnerSidString = rv
		return nil
	case "process.pid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.PIDContext.Pid"}
		}
		ev.BaseEvent.ProcessContext.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "process.ppid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.PPid"}
		}
		ev.BaseEvent.ProcessContext.Process.PPid = uint32(rv)
		return nil
	case "process.user":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.User"}
		}
		ev.BaseEvent.ProcessContext.Process.User = rv
		return nil
	case "process.user_sid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.OwnerSidString"}
		}
		ev.BaseEvent.ProcessContext.Process.OwnerSidString = rv
		return nil
	case "rename.file.destination.device_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RenameFile.New.PathnameStr"}
		}
		ev.RenameFile.New.PathnameStr = rv
		return nil
	case "rename.file.destination.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.device_path.length"}
	case "rename.file.destination.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RenameFile.New.BasenameStr"}
		}
		ev.RenameFile.New.BasenameStr = rv
		return nil
	case "rename.file.destination.name.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.name.length"}
	case "rename.file.destination.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RenameFile.New.UserPathnameStr"}
		}
		ev.RenameFile.New.UserPathnameStr = rv
		return nil
	case "rename.file.destination.path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.path.length"}
	case "rename.file.device_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RenameFile.Old.PathnameStr"}
		}
		ev.RenameFile.Old.PathnameStr = rv
		return nil
	case "rename.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.device_path.length"}
	case "rename.file.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RenameFile.Old.BasenameStr"}
		}
		ev.RenameFile.Old.BasenameStr = rv
		return nil
	case "rename.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.name.length"}
	case "rename.file.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "RenameFile.Old.UserPathnameStr"}
		}
		ev.RenameFile.Old.UserPathnameStr = rv
		return nil
	case "rename.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.path.length"}
	case "set.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.Registry.KeyName"}
		}
		ev.SetRegistryKeyValue.Registry.KeyName = rv
		return nil
	case "set.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "set.registry.key_name.length"}
	case "set.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.Registry.KeyPath"}
		}
		ev.SetRegistryKeyValue.Registry.KeyPath = rv
		return nil
	case "set.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "set.registry.key_path.length"}
	case "set.registry.value_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.ValueName"}
		}
		ev.SetRegistryKeyValue.ValueName = rv
		return nil
	case "set.registry.value_name.length":
		return &eval.ErrFieldReadOnly{Field: "set.registry.value_name.length"}
	case "set.value_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.ValueName"}
		}
		ev.SetRegistryKeyValue.ValueName = rv
		return nil
	case "set_key_value.registry.key_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.Registry.KeyName"}
		}
		ev.SetRegistryKeyValue.Registry.KeyName = rv
		return nil
	case "set_key_value.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.registry.key_name.length"}
	case "set_key_value.registry.key_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.Registry.KeyPath"}
		}
		ev.SetRegistryKeyValue.Registry.KeyPath = rv
		return nil
	case "set_key_value.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.registry.key_path.length"}
	case "set_key_value.registry.value_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.ValueName"}
		}
		ev.SetRegistryKeyValue.ValueName = rv
		return nil
	case "set_key_value.registry.value_name.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.registry.value_name.length"}
	case "set_key_value.value_name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.ValueName"}
		}
		ev.SetRegistryKeyValue.ValueName = rv
		return nil
	case "write.file.device_path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "WriteFile.File.PathnameStr"}
		}
		ev.WriteFile.File.PathnameStr = rv
		return nil
	case "write.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "write.file.device_path.length"}
	case "write.file.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "WriteFile.File.BasenameStr"}
		}
		ev.WriteFile.File.BasenameStr = rv
		return nil
	case "write.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "write.file.name.length"}
	case "write.file.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "WriteFile.File.UserPathnameStr"}
		}
		ev.WriteFile.File.UserPathnameStr = rv
		return nil
	case "write.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "write.file.path.length"}
	}
	return &eval.ErrFieldNotFound{Field: field}
}
