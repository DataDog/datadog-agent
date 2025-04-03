// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"math"
	"net"
	"reflect"
)

// to always require the math package
var _ = math.MaxUint16
var _ = net.IP{}

func (_ *Model) GetEventTypes() []eval.EventType {
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
func (_ *Model) GetFieldRestrictions(field eval.Field) []eval.EventType {
	switch field {
	}
	return nil
}
func (_ *Model) GetEvaluator(field eval.Field, regID eval.RegisterID, offset int) (eval.Evaluator, error) {
	switch field {
	case "change_permission.new_sd":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "change_permission.old_sd":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "change_permission.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.ObjectName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "change_permission.type":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.ObjectType
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "change_permission.user_domain":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.UserDomain
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "change_permission.username":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.ChangePermission.UserName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "container.runtime":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerRuntime(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "create_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "delete_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "event.hostname":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "event.origin":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.Origin
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "event.os":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.Os
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "event.rule.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.RuleTags
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "event.service":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "event.timestamp":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exec.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exec.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.Exec.Process.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.cause":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Cause)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.code":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Code)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.Exit.Process.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "open_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.cmdline":
		return &eval.StringArrayEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveProcessCmdLine(ev, &element.ProcessContext.Process)
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
					return ev.FieldHandlers.ResolveProcessCmdLine(ev, &current.ProcessContext.Process)
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 200 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := element.ProcessContext.Process.ContainerID
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) string {
					return current.ProcessContext.Process.ContainerID
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &element.ProcessContext.Process))
					return []int{result}
				}
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) int {
					return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &current.ProcessContext.Process))
				})
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveProcessEnvp(ev, &element.ProcessContext.Process)
					return result
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIteratorArray(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) []string {
					return ev.FieldHandlers.ResolveProcessEnvp(ev, &current.ProcessContext.Process)
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveProcessEnvs(ev, &element.ProcessContext.Process)
					return result
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIteratorArray(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) []string {
					return ev.FieldHandlers.ResolveProcessEnvs(ev, &current.ProcessContext.Process)
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: 100 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
					return ev.FieldHandlers.ResolveFileBasename(ev, &current.ProcessContext.Process.FileEvent)
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
					return []int{result}
				}
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) int {
					return len(ev.FieldHandlers.ResolveFileBasename(ev, &current.ProcessContext.Process.FileEvent))
				})
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
					return ev.FieldHandlers.ResolveFilePath(ev, &current.ProcessContext.Process.FileEvent)
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) []int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
					return []int{result}
				}
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) int {
					return len(ev.FieldHandlers.ResolveFilePath(ev, &current.ProcessContext.Process.FileEvent))
				})
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				iterator := &ProcessAncestorsIterator{}
				return iterator.Len(ctx)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := int(element.ProcessContext.Process.PIDContext.Pid)
					return []int{result}
				}
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) int {
					return int(current.ProcessContext.Process.PIDContext.Pid)
				})
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := int(element.ProcessContext.Process.PPid)
					return []int{result}
				}
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) int {
					return int(current.ProcessContext.Process.PPid)
				})
				ctx.IntCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveUser(ev, &element.ProcessContext.Process)
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
					return ev.FieldHandlers.ResolveUser(ev, &current.ProcessContext.Process)
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.user_sid":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := element.ProcessContext.Process.OwnerSidString
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) string {
					return current.ProcessContext.Process.OwnerSidString
				})
				ctx.StringCache[field] = results
				return results
			}, Field: field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.ProcessContext.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.BaseEvent.ProcessContext.Parent.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.parent.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: 100 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.parent.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.BaseEvent.ProcessContext.Parent.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.parent.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.BaseEvent.ProcessContext.Parent.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.ppid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.ProcessContext.Process.PPid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.ProcessContext.Process.OwnerSidString
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "set.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set.registry.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set.registry.value_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.ValueName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.Registry.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.Registry.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.registry.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.registry.value_name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.ValueName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "set_key_value.value_name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "write.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.WindowsPathCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
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
		"event.rule.tags",
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
	m := &Model{}
	evaluator, err := m.GetEvaluator(field, "", 0)
	if err != nil {
		return nil, err
	}
	ctx := eval.NewContext(ev)
	value := evaluator.Eval(ctx)
	return value, nil
}
func (ev *Event) GetFieldMetadata(field eval.Field) (eval.EventType, reflect.Kind, string, error) {
	switch field {
	case "change_permission.new_sd":
		return "change_permission", reflect.String, "string", nil
	case "change_permission.old_sd":
		return "change_permission", reflect.String, "string", nil
	case "change_permission.path":
		return "change_permission", reflect.String, "string", nil
	case "change_permission.type":
		return "change_permission", reflect.String, "string", nil
	case "change_permission.user_domain":
		return "change_permission", reflect.String, "string", nil
	case "change_permission.username":
		return "change_permission", reflect.String, "string", nil
	case "container.created_at":
		return "", reflect.Int, "int", nil
	case "container.id":
		return "", reflect.String, "string", nil
	case "container.runtime":
		return "", reflect.String, "string", nil
	case "container.tags":
		return "", reflect.String, "string", nil
	case "create.file.device_path":
		return "create", reflect.String, "string", nil
	case "create.file.device_path.length":
		return "create", reflect.Int, "int", nil
	case "create.file.name":
		return "create", reflect.String, "string", nil
	case "create.file.name.length":
		return "create", reflect.Int, "int", nil
	case "create.file.path":
		return "create", reflect.String, "string", nil
	case "create.file.path.length":
		return "create", reflect.Int, "int", nil
	case "create.registry.key_name":
		return "create_key", reflect.String, "string", nil
	case "create.registry.key_name.length":
		return "create_key", reflect.Int, "int", nil
	case "create.registry.key_path":
		return "create_key", reflect.String, "string", nil
	case "create.registry.key_path.length":
		return "create_key", reflect.Int, "int", nil
	case "create_key.registry.key_name":
		return "create_key", reflect.String, "string", nil
	case "create_key.registry.key_name.length":
		return "create_key", reflect.Int, "int", nil
	case "create_key.registry.key_path":
		return "create_key", reflect.String, "string", nil
	case "create_key.registry.key_path.length":
		return "create_key", reflect.Int, "int", nil
	case "delete.file.device_path":
		return "delete", reflect.String, "string", nil
	case "delete.file.device_path.length":
		return "delete", reflect.Int, "int", nil
	case "delete.file.name":
		return "delete", reflect.String, "string", nil
	case "delete.file.name.length":
		return "delete", reflect.Int, "int", nil
	case "delete.file.path":
		return "delete", reflect.String, "string", nil
	case "delete.file.path.length":
		return "delete", reflect.Int, "int", nil
	case "delete.registry.key_name":
		return "delete_key", reflect.String, "string", nil
	case "delete.registry.key_name.length":
		return "delete_key", reflect.Int, "int", nil
	case "delete.registry.key_path":
		return "delete_key", reflect.String, "string", nil
	case "delete.registry.key_path.length":
		return "delete_key", reflect.Int, "int", nil
	case "delete_key.registry.key_name":
		return "delete_key", reflect.String, "string", nil
	case "delete_key.registry.key_name.length":
		return "delete_key", reflect.Int, "int", nil
	case "delete_key.registry.key_path":
		return "delete_key", reflect.String, "string", nil
	case "delete_key.registry.key_path.length":
		return "delete_key", reflect.Int, "int", nil
	case "event.hostname":
		return "", reflect.String, "string", nil
	case "event.origin":
		return "", reflect.String, "string", nil
	case "event.os":
		return "", reflect.String, "string", nil
	case "event.rule.tags":
		return "", reflect.String, "string", nil
	case "event.service":
		return "", reflect.String, "string", nil
	case "event.timestamp":
		return "", reflect.Int, "int", nil
	case "exec.cmdline":
		return "exec", reflect.String, "string", nil
	case "exec.container.id":
		return "exec", reflect.String, "string", nil
	case "exec.created_at":
		return "exec", reflect.Int, "int", nil
	case "exec.envp":
		return "exec", reflect.String, "string", nil
	case "exec.envs":
		return "exec", reflect.String, "string", nil
	case "exec.file.name":
		return "exec", reflect.String, "string", nil
	case "exec.file.name.length":
		return "exec", reflect.Int, "int", nil
	case "exec.file.path":
		return "exec", reflect.String, "string", nil
	case "exec.file.path.length":
		return "exec", reflect.Int, "int", nil
	case "exec.pid":
		return "exec", reflect.Int, "int", nil
	case "exec.ppid":
		return "exec", reflect.Int, "int", nil
	case "exec.user":
		return "exec", reflect.String, "string", nil
	case "exec.user_sid":
		return "exec", reflect.String, "string", nil
	case "exit.cause":
		return "exit", reflect.Int, "int", nil
	case "exit.cmdline":
		return "exit", reflect.String, "string", nil
	case "exit.code":
		return "exit", reflect.Int, "int", nil
	case "exit.container.id":
		return "exit", reflect.String, "string", nil
	case "exit.created_at":
		return "exit", reflect.Int, "int", nil
	case "exit.envp":
		return "exit", reflect.String, "string", nil
	case "exit.envs":
		return "exit", reflect.String, "string", nil
	case "exit.file.name":
		return "exit", reflect.String, "string", nil
	case "exit.file.name.length":
		return "exit", reflect.Int, "int", nil
	case "exit.file.path":
		return "exit", reflect.String, "string", nil
	case "exit.file.path.length":
		return "exit", reflect.Int, "int", nil
	case "exit.pid":
		return "exit", reflect.Int, "int", nil
	case "exit.ppid":
		return "exit", reflect.Int, "int", nil
	case "exit.user":
		return "exit", reflect.String, "string", nil
	case "exit.user_sid":
		return "exit", reflect.String, "string", nil
	case "open.registry.key_name":
		return "open_key", reflect.String, "string", nil
	case "open.registry.key_name.length":
		return "open_key", reflect.Int, "int", nil
	case "open.registry.key_path":
		return "open_key", reflect.String, "string", nil
	case "open.registry.key_path.length":
		return "open_key", reflect.Int, "int", nil
	case "open_key.registry.key_name":
		return "open_key", reflect.String, "string", nil
	case "open_key.registry.key_name.length":
		return "open_key", reflect.Int, "int", nil
	case "open_key.registry.key_path":
		return "open_key", reflect.String, "string", nil
	case "open_key.registry.key_path.length":
		return "open_key", reflect.Int, "int", nil
	case "process.ancestors.cmdline":
		return "", reflect.String, "string", nil
	case "process.ancestors.container.id":
		return "", reflect.String, "string", nil
	case "process.ancestors.created_at":
		return "", reflect.Int, "int", nil
	case "process.ancestors.envp":
		return "", reflect.String, "string", nil
	case "process.ancestors.envs":
		return "", reflect.String, "string", nil
	case "process.ancestors.file.name":
		return "", reflect.String, "string", nil
	case "process.ancestors.file.name.length":
		return "", reflect.Int, "int", nil
	case "process.ancestors.file.path":
		return "", reflect.String, "string", nil
	case "process.ancestors.file.path.length":
		return "", reflect.Int, "int", nil
	case "process.ancestors.length":
		return "", reflect.Int, "int", nil
	case "process.ancestors.pid":
		return "", reflect.Int, "int", nil
	case "process.ancestors.ppid":
		return "", reflect.Int, "int", nil
	case "process.ancestors.user":
		return "", reflect.String, "string", nil
	case "process.ancestors.user_sid":
		return "", reflect.String, "string", nil
	case "process.cmdline":
		return "", reflect.String, "string", nil
	case "process.container.id":
		return "", reflect.String, "string", nil
	case "process.created_at":
		return "", reflect.Int, "int", nil
	case "process.envp":
		return "", reflect.String, "string", nil
	case "process.envs":
		return "", reflect.String, "string", nil
	case "process.file.name":
		return "", reflect.String, "string", nil
	case "process.file.name.length":
		return "", reflect.Int, "int", nil
	case "process.file.path":
		return "", reflect.String, "string", nil
	case "process.file.path.length":
		return "", reflect.Int, "int", nil
	case "process.parent.cmdline":
		return "", reflect.String, "string", nil
	case "process.parent.container.id":
		return "", reflect.String, "string", nil
	case "process.parent.created_at":
		return "", reflect.Int, "int", nil
	case "process.parent.envp":
		return "", reflect.String, "string", nil
	case "process.parent.envs":
		return "", reflect.String, "string", nil
	case "process.parent.file.name":
		return "", reflect.String, "string", nil
	case "process.parent.file.name.length":
		return "", reflect.Int, "int", nil
	case "process.parent.file.path":
		return "", reflect.String, "string", nil
	case "process.parent.file.path.length":
		return "", reflect.Int, "int", nil
	case "process.parent.pid":
		return "", reflect.Int, "int", nil
	case "process.parent.ppid":
		return "", reflect.Int, "int", nil
	case "process.parent.user":
		return "", reflect.String, "string", nil
	case "process.parent.user_sid":
		return "", reflect.String, "string", nil
	case "process.pid":
		return "", reflect.Int, "int", nil
	case "process.ppid":
		return "", reflect.Int, "int", nil
	case "process.user":
		return "", reflect.String, "string", nil
	case "process.user_sid":
		return "", reflect.String, "string", nil
	case "rename.file.destination.device_path":
		return "rename", reflect.String, "string", nil
	case "rename.file.destination.device_path.length":
		return "rename", reflect.Int, "int", nil
	case "rename.file.destination.name":
		return "rename", reflect.String, "string", nil
	case "rename.file.destination.name.length":
		return "rename", reflect.Int, "int", nil
	case "rename.file.destination.path":
		return "rename", reflect.String, "string", nil
	case "rename.file.destination.path.length":
		return "rename", reflect.Int, "int", nil
	case "rename.file.device_path":
		return "rename", reflect.String, "string", nil
	case "rename.file.device_path.length":
		return "rename", reflect.Int, "int", nil
	case "rename.file.name":
		return "rename", reflect.String, "string", nil
	case "rename.file.name.length":
		return "rename", reflect.Int, "int", nil
	case "rename.file.path":
		return "rename", reflect.String, "string", nil
	case "rename.file.path.length":
		return "rename", reflect.Int, "int", nil
	case "set.registry.key_name":
		return "set_key_value", reflect.String, "string", nil
	case "set.registry.key_name.length":
		return "set_key_value", reflect.Int, "int", nil
	case "set.registry.key_path":
		return "set_key_value", reflect.String, "string", nil
	case "set.registry.key_path.length":
		return "set_key_value", reflect.Int, "int", nil
	case "set.registry.value_name":
		return "set_key_value", reflect.String, "string", nil
	case "set.registry.value_name.length":
		return "set_key_value", reflect.Int, "int", nil
	case "set.value_name":
		return "set_key_value", reflect.String, "string", nil
	case "set_key_value.registry.key_name":
		return "set_key_value", reflect.String, "string", nil
	case "set_key_value.registry.key_name.length":
		return "set_key_value", reflect.Int, "int", nil
	case "set_key_value.registry.key_path":
		return "set_key_value", reflect.String, "string", nil
	case "set_key_value.registry.key_path.length":
		return "set_key_value", reflect.Int, "int", nil
	case "set_key_value.registry.value_name":
		return "set_key_value", reflect.String, "string", nil
	case "set_key_value.registry.value_name.length":
		return "set_key_value", reflect.Int, "int", nil
	case "set_key_value.value_name":
		return "set_key_value", reflect.String, "string", nil
	case "write.file.device_path":
		return "write", reflect.String, "string", nil
	case "write.file.device_path.length":
		return "write", reflect.Int, "int", nil
	case "write.file.name":
		return "write", reflect.String, "string", nil
	case "write.file.name.length":
		return "write", reflect.Int, "int", nil
	case "write.file.path":
		return "write", reflect.String, "string", nil
	case "write.file.path.length":
		return "write", reflect.Int, "int", nil
	}
	return "", reflect.Invalid, "", &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {
	}
	return &eval.ErrFieldNotFound{Field: field}
}
