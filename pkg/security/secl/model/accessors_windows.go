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
	"net"
	"reflect"
	"strings"
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
	// handle legacy field mapping
	if newField, found := GetDefaultLegacyFields(field); found {
		field = newField
	}
	switch field {
	}
	return nil
}
func (_ *Model) GetEvaluator(field eval.Field, regID eval.RegisterID, offset int) (eval.Evaluator, error) {
	// Handle array index access (e.g., field[0])
	// This is processed here before the switch to support all array fields
	baseField, arrayIndex, isArrayAccess, err := eval.ExtractArrayIndexAccess(field)
	if err != nil {
		return nil, err
	}
	if isArrayAccess {
		// Get the base field evaluator (returns the full array)
		arrayEvaluator, err := (&Model{}).GetEvaluator(baseField, regID, offset)
		if err != nil {
			return nil, err
		}
		// Wrap it to return only the specific index
		return eval.WrapEvaluatorWithArrayIndex(arrayEvaluator, arrayIndex, baseField)
	}
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
	case "create.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.CreateNewFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "create.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.DeleteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "delete.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
	case "event.source":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveSource(ev, &ev.BaseEvent)
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exec.Process.ContainerContext.CreatedAt)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exec.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return string(ev.Exec.Process.ContainerContext.ContainerID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exec.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exec.Process.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
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
	case "exec.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exec.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
	case "exit.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.Exit.Process.ContainerContext.CreatedAt)
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
				return string(ev.Exit.Process.ContainerContext.ContainerID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "exit.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exit.Process.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
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
	case "exit.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "exit.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			},
			Field:  field,
			Weight: 200 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.container.created_at":
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
					result := int(element.ProcessContext.Process.ContainerContext.CreatedAt)
					return []int{result}
				}
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) int {
					return int(current.ProcessContext.Process.ContainerContext.CreatedAt)
				})
				ctx.IntCache[field] = results
				return results
			},
			Field:  field,
			Weight: eval.IteratorWeight,
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
					result := string(element.ProcessContext.Process.ContainerContext.ContainerID)
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) string {
					return string(current.ProcessContext.Process.ContainerContext.ContainerID)
				})
				ctx.StringCache[field] = results
				return results
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.container.tags":
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
					result := ev.FieldHandlers.ResolveContainerTags(ev, &element.ProcessContext.Process.ContainerContext)
					return result
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIteratorArray(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) []string {
					return ev.FieldHandlers.ResolveContainerTags(ev, &current.ProcessContext.Process.ContainerContext)
				})
				ctx.StringCache[field] = results
				return results
			},
			Field:  field,
			Weight: 9999 * eval.IteratorWeight,
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
			},
			Field:  field,
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
			},
			Field:  field,
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
			},
			Field:  field,
			Weight: 100 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.extension":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				iterator := &ProcessAncestorsIterator{Root: ev.BaseEvent.ProcessContext.Ancestor}
				if regID != "" {
					element := iterator.At(ctx, regID, ctx.Registers[regID])
					if element == nil {
						return nil
					}
					result := ev.FieldHandlers.ResolveFileExtension(ev, &element.ProcessContext.Process.FileEvent)
					return []string{result}
				}
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
					return ev.FieldHandlers.ResolveFileExtension(ev, &current.ProcessContext.Process.FileEvent)
				})
				ctx.StringCache[field] = results
				return results
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			},
			Field:  field,
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
			},
			Field:  field,
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
			},
			Field:  field,
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
			},
			Field:  field,
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
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
			},
			Field:  field,
			Weight: 200 * eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.ProcessContext.Process.ContainerContext.CreatedAt)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.container.id":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return string(ev.BaseEvent.ProcessContext.Process.ContainerContext.ContainerID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Process.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
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
	case "process.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
	case "process.parent.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return 0
				}
				return int(ev.BaseEvent.ProcessContext.Parent.ContainerContext.CreatedAt)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
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
				return string(ev.BaseEvent.ProcessContext.Parent.ContainerContext.ContainerID)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
			Offset: offset,
		}, nil
	case "process.parent.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return []string{}
				}
				return ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Parent.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
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
	case "process.parent.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				if !ev.BaseEvent.ProcessContext.HasParent() {
					return ""
				}
				return ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "process.parent.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.New)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.destination.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.Old)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "rename.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) int {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) string {
				ctx.AppendResolvedField(field)
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.WriteFile.File)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
			Offset: offset,
		}, nil
	case "write.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
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
	fields := []eval.Field{
		"change_permission.new_sd",
		"change_permission.old_sd",
		"change_permission.path",
		"change_permission.type",
		"change_permission.user_domain",
		"change_permission.username",
		"create.file.device_path",
		"create.file.device_path.length",
		"create.file.extension",
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
		"delete.file.extension",
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
		"event.source",
		"event.timestamp",
		"exec.cmdline",
		"exec.container.created_at",
		"exec.container.id",
		"exec.container.tags",
		"exec.created_at",
		"exec.envp",
		"exec.envs",
		"exec.file.extension",
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
		"exit.container.created_at",
		"exit.container.id",
		"exit.container.tags",
		"exit.created_at",
		"exit.envp",
		"exit.envs",
		"exit.file.extension",
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
		"process.ancestors.container.created_at",
		"process.ancestors.container.id",
		"process.ancestors.container.tags",
		"process.ancestors.created_at",
		"process.ancestors.envp",
		"process.ancestors.envs",
		"process.ancestors.file.extension",
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
		"process.container.created_at",
		"process.container.id",
		"process.container.tags",
		"process.created_at",
		"process.envp",
		"process.envs",
		"process.file.extension",
		"process.file.name",
		"process.file.name.length",
		"process.file.path",
		"process.file.path.length",
		"process.parent.cmdline",
		"process.parent.container.created_at",
		"process.parent.container.id",
		"process.parent.container.tags",
		"process.parent.created_at",
		"process.parent.envp",
		"process.parent.envs",
		"process.parent.file.extension",
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
		"rename.file.destination.extension",
		"rename.file.destination.name",
		"rename.file.destination.name.length",
		"rename.file.destination.path",
		"rename.file.destination.path.length",
		"rename.file.device_path",
		"rename.file.device_path.length",
		"rename.file.extension",
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
		"write.file.extension",
		"write.file.name",
		"write.file.name.length",
		"write.file.path",
		"write.file.path.length",
	}
	// Add legacy field names if mapping is available
	legacyKeys := GetDefaultLegacyFieldsKeys()
	if legacyKeys != nil {
		fields = append(fields, legacyKeys...)
	}
	return fields
}

// GetFieldMetadata returns EventType, reflect.Kind, BasicType, IsArray, error
func (ev *Event) GetFieldMetadata(field eval.Field) (eval.EventType, reflect.Kind, string, bool, error) {
	originalField := field
	// handle legacy field mapping
	if newField, found := GetDefaultLegacyFields(field); found {
		field = newField
	}
	switch field {
	case "change_permission.new_sd":
		return "change_permission", reflect.String, "string", false, nil
	case "change_permission.old_sd":
		return "change_permission", reflect.String, "string", false, nil
	case "change_permission.path":
		return "change_permission", reflect.String, "string", false, nil
	case "change_permission.type":
		return "change_permission", reflect.String, "string", false, nil
	case "change_permission.user_domain":
		return "change_permission", reflect.String, "string", false, nil
	case "change_permission.username":
		return "change_permission", reflect.String, "string", false, nil
	case "create.file.device_path":
		return "create", reflect.String, "string", false, nil
	case "create.file.device_path.length":
		return "create", reflect.Int, "int", false, nil
	case "create.file.extension":
		return "create", reflect.String, "string", false, nil
	case "create.file.name":
		return "create", reflect.String, "string", false, nil
	case "create.file.name.length":
		return "create", reflect.Int, "int", false, nil
	case "create.file.path":
		return "create", reflect.String, "string", false, nil
	case "create.file.path.length":
		return "create", reflect.Int, "int", false, nil
	case "create.registry.key_name":
		return "create_key", reflect.String, "string", false, nil
	case "create.registry.key_name.length":
		return "create_key", reflect.Int, "int", false, nil
	case "create.registry.key_path":
		return "create_key", reflect.String, "string", false, nil
	case "create.registry.key_path.length":
		return "create_key", reflect.Int, "int", false, nil
	case "create_key.registry.key_name":
		return "create_key", reflect.String, "string", false, nil
	case "create_key.registry.key_name.length":
		return "create_key", reflect.Int, "int", false, nil
	case "create_key.registry.key_path":
		return "create_key", reflect.String, "string", false, nil
	case "create_key.registry.key_path.length":
		return "create_key", reflect.Int, "int", false, nil
	case "delete.file.device_path":
		return "delete", reflect.String, "string", false, nil
	case "delete.file.device_path.length":
		return "delete", reflect.Int, "int", false, nil
	case "delete.file.extension":
		return "delete", reflect.String, "string", false, nil
	case "delete.file.name":
		return "delete", reflect.String, "string", false, nil
	case "delete.file.name.length":
		return "delete", reflect.Int, "int", false, nil
	case "delete.file.path":
		return "delete", reflect.String, "string", false, nil
	case "delete.file.path.length":
		return "delete", reflect.Int, "int", false, nil
	case "delete.registry.key_name":
		return "delete_key", reflect.String, "string", false, nil
	case "delete.registry.key_name.length":
		return "delete_key", reflect.Int, "int", false, nil
	case "delete.registry.key_path":
		return "delete_key", reflect.String, "string", false, nil
	case "delete.registry.key_path.length":
		return "delete_key", reflect.Int, "int", false, nil
	case "delete_key.registry.key_name":
		return "delete_key", reflect.String, "string", false, nil
	case "delete_key.registry.key_name.length":
		return "delete_key", reflect.Int, "int", false, nil
	case "delete_key.registry.key_path":
		return "delete_key", reflect.String, "string", false, nil
	case "delete_key.registry.key_path.length":
		return "delete_key", reflect.Int, "int", false, nil
	case "event.hostname":
		return "", reflect.String, "string", false, nil
	case "event.origin":
		return "", reflect.String, "string", false, nil
	case "event.os":
		return "", reflect.String, "string", false, nil
	case "event.rule.tags":
		return "", reflect.String, "string", true, nil
	case "event.service":
		return "", reflect.String, "string", false, nil
	case "event.source":
		return "", reflect.String, "string", false, nil
	case "event.timestamp":
		return "", reflect.Int, "int", false, nil
	case "exec.cmdline":
		return "exec", reflect.String, "string", false, nil
	case "exec.container.created_at":
		return "exec", reflect.Int, "int", false, nil
	case "exec.container.id":
		return "exec", reflect.String, "string", false, nil
	case "exec.container.tags":
		return "exec", reflect.String, "string", true, nil
	case "exec.created_at":
		return "exec", reflect.Int, "int", false, nil
	case "exec.envp":
		return "exec", reflect.String, "string", true, nil
	case "exec.envs":
		return "exec", reflect.String, "string", true, nil
	case "exec.file.extension":
		return "exec", reflect.String, "string", false, nil
	case "exec.file.name":
		return "exec", reflect.String, "string", false, nil
	case "exec.file.name.length":
		return "exec", reflect.Int, "int", false, nil
	case "exec.file.path":
		return "exec", reflect.String, "string", false, nil
	case "exec.file.path.length":
		return "exec", reflect.Int, "int", false, nil
	case "exec.pid":
		return "exec", reflect.Int, "int", false, nil
	case "exec.ppid":
		return "exec", reflect.Int, "int", false, nil
	case "exec.user":
		return "exec", reflect.String, "string", false, nil
	case "exec.user_sid":
		return "exec", reflect.String, "string", false, nil
	case "exit.cause":
		return "exit", reflect.Int, "int", false, nil
	case "exit.cmdline":
		return "exit", reflect.String, "string", false, nil
	case "exit.code":
		return "exit", reflect.Int, "int", false, nil
	case "exit.container.created_at":
		return "exit", reflect.Int, "int", false, nil
	case "exit.container.id":
		return "exit", reflect.String, "string", false, nil
	case "exit.container.tags":
		return "exit", reflect.String, "string", true, nil
	case "exit.created_at":
		return "exit", reflect.Int, "int", false, nil
	case "exit.envp":
		return "exit", reflect.String, "string", true, nil
	case "exit.envs":
		return "exit", reflect.String, "string", true, nil
	case "exit.file.extension":
		return "exit", reflect.String, "string", false, nil
	case "exit.file.name":
		return "exit", reflect.String, "string", false, nil
	case "exit.file.name.length":
		return "exit", reflect.Int, "int", false, nil
	case "exit.file.path":
		return "exit", reflect.String, "string", false, nil
	case "exit.file.path.length":
		return "exit", reflect.Int, "int", false, nil
	case "exit.pid":
		return "exit", reflect.Int, "int", false, nil
	case "exit.ppid":
		return "exit", reflect.Int, "int", false, nil
	case "exit.user":
		return "exit", reflect.String, "string", false, nil
	case "exit.user_sid":
		return "exit", reflect.String, "string", false, nil
	case "open.registry.key_name":
		return "open_key", reflect.String, "string", false, nil
	case "open.registry.key_name.length":
		return "open_key", reflect.Int, "int", false, nil
	case "open.registry.key_path":
		return "open_key", reflect.String, "string", false, nil
	case "open.registry.key_path.length":
		return "open_key", reflect.Int, "int", false, nil
	case "open_key.registry.key_name":
		return "open_key", reflect.String, "string", false, nil
	case "open_key.registry.key_name.length":
		return "open_key", reflect.Int, "int", false, nil
	case "open_key.registry.key_path":
		return "open_key", reflect.String, "string", false, nil
	case "open_key.registry.key_path.length":
		return "open_key", reflect.Int, "int", false, nil
	case "process.ancestors.cmdline":
		return "", reflect.String, "string", false, nil
	case "process.ancestors.container.created_at":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.container.id":
		return "", reflect.String, "string", false, nil
	case "process.ancestors.container.tags":
		return "", reflect.String, "string", true, nil
	case "process.ancestors.created_at":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.envp":
		return "", reflect.String, "string", true, nil
	case "process.ancestors.envs":
		return "", reflect.String, "string", true, nil
	case "process.ancestors.file.extension":
		return "", reflect.String, "string", false, nil
	case "process.ancestors.file.name":
		return "", reflect.String, "string", false, nil
	case "process.ancestors.file.name.length":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.file.path":
		return "", reflect.String, "string", false, nil
	case "process.ancestors.file.path.length":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.length":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.pid":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.ppid":
		return "", reflect.Int, "int", false, nil
	case "process.ancestors.user":
		return "", reflect.String, "string", false, nil
	case "process.ancestors.user_sid":
		return "", reflect.String, "string", false, nil
	case "process.cmdline":
		return "", reflect.String, "string", false, nil
	case "process.container.created_at":
		return "", reflect.Int, "int", false, nil
	case "process.container.id":
		return "", reflect.String, "string", false, nil
	case "process.container.tags":
		return "", reflect.String, "string", true, nil
	case "process.created_at":
		return "", reflect.Int, "int", false, nil
	case "process.envp":
		return "", reflect.String, "string", true, nil
	case "process.envs":
		return "", reflect.String, "string", true, nil
	case "process.file.extension":
		return "", reflect.String, "string", false, nil
	case "process.file.name":
		return "", reflect.String, "string", false, nil
	case "process.file.name.length":
		return "", reflect.Int, "int", false, nil
	case "process.file.path":
		return "", reflect.String, "string", false, nil
	case "process.file.path.length":
		return "", reflect.Int, "int", false, nil
	case "process.parent.cmdline":
		return "", reflect.String, "string", false, nil
	case "process.parent.container.created_at":
		return "", reflect.Int, "int", false, nil
	case "process.parent.container.id":
		return "", reflect.String, "string", false, nil
	case "process.parent.container.tags":
		return "", reflect.String, "string", true, nil
	case "process.parent.created_at":
		return "", reflect.Int, "int", false, nil
	case "process.parent.envp":
		return "", reflect.String, "string", true, nil
	case "process.parent.envs":
		return "", reflect.String, "string", true, nil
	case "process.parent.file.extension":
		return "", reflect.String, "string", false, nil
	case "process.parent.file.name":
		return "", reflect.String, "string", false, nil
	case "process.parent.file.name.length":
		return "", reflect.Int, "int", false, nil
	case "process.parent.file.path":
		return "", reflect.String, "string", false, nil
	case "process.parent.file.path.length":
		return "", reflect.Int, "int", false, nil
	case "process.parent.pid":
		return "", reflect.Int, "int", false, nil
	case "process.parent.ppid":
		return "", reflect.Int, "int", false, nil
	case "process.parent.user":
		return "", reflect.String, "string", false, nil
	case "process.parent.user_sid":
		return "", reflect.String, "string", false, nil
	case "process.pid":
		return "", reflect.Int, "int", false, nil
	case "process.ppid":
		return "", reflect.Int, "int", false, nil
	case "process.user":
		return "", reflect.String, "string", false, nil
	case "process.user_sid":
		return "", reflect.String, "string", false, nil
	case "rename.file.destination.device_path":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.destination.device_path.length":
		return "rename", reflect.Int, "int", false, nil
	case "rename.file.destination.extension":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.destination.name":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.destination.name.length":
		return "rename", reflect.Int, "int", false, nil
	case "rename.file.destination.path":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.destination.path.length":
		return "rename", reflect.Int, "int", false, nil
	case "rename.file.device_path":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.device_path.length":
		return "rename", reflect.Int, "int", false, nil
	case "rename.file.extension":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.name":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.name.length":
		return "rename", reflect.Int, "int", false, nil
	case "rename.file.path":
		return "rename", reflect.String, "string", false, nil
	case "rename.file.path.length":
		return "rename", reflect.Int, "int", false, nil
	case "set.registry.key_name":
		return "set_key_value", reflect.String, "string", false, nil
	case "set.registry.key_name.length":
		return "set_key_value", reflect.Int, "int", false, nil
	case "set.registry.key_path":
		return "set_key_value", reflect.String, "string", false, nil
	case "set.registry.key_path.length":
		return "set_key_value", reflect.Int, "int", false, nil
	case "set.registry.value_name":
		return "set_key_value", reflect.String, "string", false, nil
	case "set.registry.value_name.length":
		return "set_key_value", reflect.Int, "int", false, nil
	case "set.value_name":
		return "set_key_value", reflect.String, "string", false, nil
	case "set_key_value.registry.key_name":
		return "set_key_value", reflect.String, "string", false, nil
	case "set_key_value.registry.key_name.length":
		return "set_key_value", reflect.Int, "int", false, nil
	case "set_key_value.registry.key_path":
		return "set_key_value", reflect.String, "string", false, nil
	case "set_key_value.registry.key_path.length":
		return "set_key_value", reflect.Int, "int", false, nil
	case "set_key_value.registry.value_name":
		return "set_key_value", reflect.String, "string", false, nil
	case "set_key_value.registry.value_name.length":
		return "set_key_value", reflect.Int, "int", false, nil
	case "set_key_value.value_name":
		return "set_key_value", reflect.String, "string", false, nil
	case "write.file.device_path":
		return "write", reflect.String, "string", false, nil
	case "write.file.device_path.length":
		return "write", reflect.Int, "int", false, nil
	case "write.file.extension":
		return "write", reflect.String, "string", false, nil
	case "write.file.name":
		return "write", reflect.String, "string", false, nil
	case "write.file.name.length":
		return "write", reflect.Int, "int", false, nil
	case "write.file.path":
		return "write", reflect.String, "string", false, nil
	case "write.file.path.length":
		return "write", reflect.Int, "int", false, nil
	}
	return "", reflect.Invalid, "", false, &eval.ErrFieldNotFound{Field: originalField}
}
func (ev *Event) SetFieldValue(field eval.Field, value interface{}) error {
	// handle legacy field mapping
	mappedField := field
	if newField, found := GetDefaultLegacyFields(field); found {
		mappedField = newField
	}
	if strings.HasPrefix(mappedField, "process.") || strings.HasPrefix(mappedField, "exec.") || strings.HasPrefix(mappedField, "exit.") || strings.HasPrefix(mappedField, "ptrace.") {
		ev.initPointerFields()
	}
	switch mappedField {
	case "change_permission.new_sd":
		return ev.setStringFieldValue("change_permission.new_sd", &ev.ChangePermission.NewSd, value)
	case "change_permission.old_sd":
		return ev.setStringFieldValue("change_permission.old_sd", &ev.ChangePermission.OldSd, value)
	case "change_permission.path":
		return ev.setStringFieldValue("change_permission.path", &ev.ChangePermission.ObjectName, value)
	case "change_permission.type":
		return ev.setStringFieldValue("change_permission.type", &ev.ChangePermission.ObjectType, value)
	case "change_permission.user_domain":
		return ev.setStringFieldValue("change_permission.user_domain", &ev.ChangePermission.UserDomain, value)
	case "change_permission.username":
		return ev.setStringFieldValue("change_permission.username", &ev.ChangePermission.UserName, value)
	case "create.file.device_path":
		return ev.setStringFieldValue("create.file.device_path", &ev.CreateNewFile.File.PathnameStr, value)
	case "create.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "create.file.device_path.length"}
	case "create.file.extension":
		return ev.setStringFieldValue("create.file.extension", &ev.CreateNewFile.File.Extension, value)
	case "create.file.name":
		return ev.setStringFieldValue("create.file.name", &ev.CreateNewFile.File.BasenameStr, value)
	case "create.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "create.file.name.length"}
	case "create.file.path":
		return ev.setStringFieldValue("create.file.path", &ev.CreateNewFile.File.UserPathnameStr, value)
	case "create.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "create.file.path.length"}
	case "create.registry.key_name":
		return ev.setStringFieldValue("create.registry.key_name", &ev.CreateRegistryKey.Registry.KeyName, value)
	case "create.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "create.registry.key_name.length"}
	case "create.registry.key_path":
		return ev.setStringFieldValue("create.registry.key_path", &ev.CreateRegistryKey.Registry.KeyPath, value)
	case "create.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "create.registry.key_path.length"}
	case "create_key.registry.key_name":
		return ev.setStringFieldValue("create_key.registry.key_name", &ev.CreateRegistryKey.Registry.KeyName, value)
	case "create_key.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.registry.key_name.length"}
	case "create_key.registry.key_path":
		return ev.setStringFieldValue("create_key.registry.key_path", &ev.CreateRegistryKey.Registry.KeyPath, value)
	case "create_key.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.registry.key_path.length"}
	case "delete.file.device_path":
		return ev.setStringFieldValue("delete.file.device_path", &ev.DeleteFile.File.PathnameStr, value)
	case "delete.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "delete.file.device_path.length"}
	case "delete.file.extension":
		return ev.setStringFieldValue("delete.file.extension", &ev.DeleteFile.File.Extension, value)
	case "delete.file.name":
		return ev.setStringFieldValue("delete.file.name", &ev.DeleteFile.File.BasenameStr, value)
	case "delete.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "delete.file.name.length"}
	case "delete.file.path":
		return ev.setStringFieldValue("delete.file.path", &ev.DeleteFile.File.UserPathnameStr, value)
	case "delete.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "delete.file.path.length"}
	case "delete.registry.key_name":
		return ev.setStringFieldValue("delete.registry.key_name", &ev.DeleteRegistryKey.Registry.KeyName, value)
	case "delete.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "delete.registry.key_name.length"}
	case "delete.registry.key_path":
		return ev.setStringFieldValue("delete.registry.key_path", &ev.DeleteRegistryKey.Registry.KeyPath, value)
	case "delete.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "delete.registry.key_path.length"}
	case "delete_key.registry.key_name":
		return ev.setStringFieldValue("delete_key.registry.key_name", &ev.DeleteRegistryKey.Registry.KeyName, value)
	case "delete_key.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.registry.key_name.length"}
	case "delete_key.registry.key_path":
		return ev.setStringFieldValue("delete_key.registry.key_path", &ev.DeleteRegistryKey.Registry.KeyPath, value)
	case "delete_key.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.registry.key_path.length"}
	case "event.hostname":
		return ev.setStringFieldValue("event.hostname", &ev.BaseEvent.Hostname, value)
	case "event.origin":
		return ev.setStringFieldValue("event.origin", &ev.BaseEvent.Origin, value)
	case "event.os":
		return ev.setStringFieldValue("event.os", &ev.BaseEvent.Os, value)
	case "event.rule.tags":
		return ev.setStringArrayFieldValue("event.rule.tags", &ev.BaseEvent.RuleTags, value)
	case "event.service":
		return ev.setStringFieldValue("event.service", &ev.BaseEvent.Service, value)
	case "event.source":
		return ev.setStringFieldValue("event.source", &ev.BaseEvent.Source, value)
	case "event.timestamp":
		return ev.setUint64FieldValue("event.timestamp", &ev.BaseEvent.TimestampRaw, value)
	case "exec.cmdline":
		return ev.setStringFieldValue("exec.cmdline", &ev.Exec.Process.CmdLine, value)
	case "exec.container.created_at":
		return ev.setUint64FieldValue("exec.container.created_at", &ev.Exec.Process.ContainerContext.CreatedAt, value)
	case "exec.container.id":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "exec.container.id"}
		}
		ev.Exec.Process.ContainerContext.ContainerID = containerutils.ContainerID(rv)
		return nil
	case "exec.container.tags":
		return ev.setStringArrayFieldValue("exec.container.tags", &ev.Exec.Process.ContainerContext.Tags, value)
	case "exec.created_at":
		return ev.setUint64FieldValue("exec.created_at", &ev.Exec.Process.CreatedAt, value)
	case "exec.envp":
		return ev.setStringArrayFieldValue("exec.envp", &ev.Exec.Process.Envp, value)
	case "exec.envs":
		return ev.setStringArrayFieldValue("exec.envs", &ev.Exec.Process.Envs, value)
	case "exec.file.extension":
		return ev.setStringFieldValue("exec.file.extension", &ev.Exec.Process.FileEvent.Extension, value)
	case "exec.file.name":
		return ev.setStringFieldValue("exec.file.name", &ev.Exec.Process.FileEvent.BasenameStr, value)
	case "exec.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "exec.file.name.length"}
	case "exec.file.path":
		return ev.setStringFieldValue("exec.file.path", &ev.Exec.Process.FileEvent.PathnameStr, value)
	case "exec.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "exec.file.path.length"}
	case "exec.pid":
		return ev.setUint32FieldValue("exec.pid", &ev.Exec.Process.PIDContext.Pid, value)
	case "exec.ppid":
		return ev.setUint32FieldValue("exec.ppid", &ev.Exec.Process.PPid, value)
	case "exec.user":
		return ev.setStringFieldValue("exec.user", &ev.Exec.Process.User, value)
	case "exec.user_sid":
		return ev.setStringFieldValue("exec.user_sid", &ev.Exec.Process.OwnerSidString, value)
	case "exit.cause":
		return ev.setUint32FieldValue("exit.cause", &ev.Exit.Cause, value)
	case "exit.cmdline":
		return ev.setStringFieldValue("exit.cmdline", &ev.Exit.Process.CmdLine, value)
	case "exit.code":
		return ev.setUint32FieldValue("exit.code", &ev.Exit.Code, value)
	case "exit.container.created_at":
		return ev.setUint64FieldValue("exit.container.created_at", &ev.Exit.Process.ContainerContext.CreatedAt, value)
	case "exit.container.id":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "exit.container.id"}
		}
		ev.Exit.Process.ContainerContext.ContainerID = containerutils.ContainerID(rv)
		return nil
	case "exit.container.tags":
		return ev.setStringArrayFieldValue("exit.container.tags", &ev.Exit.Process.ContainerContext.Tags, value)
	case "exit.created_at":
		return ev.setUint64FieldValue("exit.created_at", &ev.Exit.Process.CreatedAt, value)
	case "exit.envp":
		return ev.setStringArrayFieldValue("exit.envp", &ev.Exit.Process.Envp, value)
	case "exit.envs":
		return ev.setStringArrayFieldValue("exit.envs", &ev.Exit.Process.Envs, value)
	case "exit.file.extension":
		return ev.setStringFieldValue("exit.file.extension", &ev.Exit.Process.FileEvent.Extension, value)
	case "exit.file.name":
		return ev.setStringFieldValue("exit.file.name", &ev.Exit.Process.FileEvent.BasenameStr, value)
	case "exit.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "exit.file.name.length"}
	case "exit.file.path":
		return ev.setStringFieldValue("exit.file.path", &ev.Exit.Process.FileEvent.PathnameStr, value)
	case "exit.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "exit.file.path.length"}
	case "exit.pid":
		return ev.setUint32FieldValue("exit.pid", &ev.Exit.Process.PIDContext.Pid, value)
	case "exit.ppid":
		return ev.setUint32FieldValue("exit.ppid", &ev.Exit.Process.PPid, value)
	case "exit.user":
		return ev.setStringFieldValue("exit.user", &ev.Exit.Process.User, value)
	case "exit.user_sid":
		return ev.setStringFieldValue("exit.user_sid", &ev.Exit.Process.OwnerSidString, value)
	case "open.registry.key_name":
		return ev.setStringFieldValue("open.registry.key_name", &ev.OpenRegistryKey.Registry.KeyName, value)
	case "open.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "open.registry.key_name.length"}
	case "open.registry.key_path":
		return ev.setStringFieldValue("open.registry.key_path", &ev.OpenRegistryKey.Registry.KeyPath, value)
	case "open.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "open.registry.key_path.length"}
	case "open_key.registry.key_name":
		return ev.setStringFieldValue("open_key.registry.key_name", &ev.OpenRegistryKey.Registry.KeyName, value)
	case "open_key.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.registry.key_name.length"}
	case "open_key.registry.key_path":
		return ev.setStringFieldValue("open_key.registry.key_path", &ev.OpenRegistryKey.Registry.KeyPath, value)
	case "open_key.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.registry.key_path.length"}
	case "process.ancestors.cmdline":
		return ev.setStringFieldValue("process.ancestors.cmdline", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.CmdLine, value)
	case "process.ancestors.container.created_at":
		return ev.setUint64FieldValue("process.ancestors.container.created_at", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.ContainerContext.CreatedAt, value)
	case "process.ancestors.container.id":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "process.ancestors.container.id"}
		}
		ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.ContainerContext.ContainerID = containerutils.ContainerID(rv)
		return nil
	case "process.ancestors.container.tags":
		return ev.setStringArrayFieldValue("process.ancestors.container.tags", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.ContainerContext.Tags, value)
	case "process.ancestors.created_at":
		return ev.setUint64FieldValue("process.ancestors.created_at", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.CreatedAt, value)
	case "process.ancestors.envp":
		return ev.setStringArrayFieldValue("process.ancestors.envp", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envp, value)
	case "process.ancestors.envs":
		return ev.setStringArrayFieldValue("process.ancestors.envs", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.Envs, value)
	case "process.ancestors.file.extension":
		return ev.setStringFieldValue("process.ancestors.file.extension", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.Extension, value)
	case "process.ancestors.file.name":
		return ev.setStringFieldValue("process.ancestors.file.name", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.BasenameStr, value)
	case "process.ancestors.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.name.length"}
	case "process.ancestors.file.path":
		return ev.setStringFieldValue("process.ancestors.file.path", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.FileEvent.PathnameStr, value)
	case "process.ancestors.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.file.path.length"}
	case "process.ancestors.length":
		return &eval.ErrFieldReadOnly{Field: "process.ancestors.length"}
	case "process.ancestors.pid":
		return ev.setUint32FieldValue("process.ancestors.pid", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.PIDContext.Pid, value)
	case "process.ancestors.ppid":
		return ev.setUint32FieldValue("process.ancestors.ppid", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.PPid, value)
	case "process.ancestors.user":
		return ev.setStringFieldValue("process.ancestors.user", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.User, value)
	case "process.ancestors.user_sid":
		return ev.setStringFieldValue("process.ancestors.user_sid", &ev.BaseEvent.ProcessContext.Ancestor.ProcessContext.Process.OwnerSidString, value)
	case "process.cmdline":
		return ev.setStringFieldValue("process.cmdline", &ev.BaseEvent.ProcessContext.Process.CmdLine, value)
	case "process.container.created_at":
		return ev.setUint64FieldValue("process.container.created_at", &ev.BaseEvent.ProcessContext.Process.ContainerContext.CreatedAt, value)
	case "process.container.id":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "process.container.id"}
		}
		ev.BaseEvent.ProcessContext.Process.ContainerContext.ContainerID = containerutils.ContainerID(rv)
		return nil
	case "process.container.tags":
		return ev.setStringArrayFieldValue("process.container.tags", &ev.BaseEvent.ProcessContext.Process.ContainerContext.Tags, value)
	case "process.created_at":
		return ev.setUint64FieldValue("process.created_at", &ev.BaseEvent.ProcessContext.Process.CreatedAt, value)
	case "process.envp":
		return ev.setStringArrayFieldValue("process.envp", &ev.BaseEvent.ProcessContext.Process.Envp, value)
	case "process.envs":
		return ev.setStringArrayFieldValue("process.envs", &ev.BaseEvent.ProcessContext.Process.Envs, value)
	case "process.file.extension":
		return ev.setStringFieldValue("process.file.extension", &ev.BaseEvent.ProcessContext.Process.FileEvent.Extension, value)
	case "process.file.name":
		return ev.setStringFieldValue("process.file.name", &ev.BaseEvent.ProcessContext.Process.FileEvent.BasenameStr, value)
	case "process.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "process.file.name.length"}
	case "process.file.path":
		return ev.setStringFieldValue("process.file.path", &ev.BaseEvent.ProcessContext.Process.FileEvent.PathnameStr, value)
	case "process.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "process.file.path.length"}
	case "process.parent.cmdline":
		return ev.setStringFieldValue("process.parent.cmdline", &ev.BaseEvent.ProcessContext.Parent.CmdLine, value)
	case "process.parent.container.created_at":
		return ev.setUint64FieldValue("process.parent.container.created_at", &ev.BaseEvent.ProcessContext.Parent.ContainerContext.CreatedAt, value)
	case "process.parent.container.id":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "process.parent.container.id"}
		}
		ev.BaseEvent.ProcessContext.Parent.ContainerContext.ContainerID = containerutils.ContainerID(rv)
		return nil
	case "process.parent.container.tags":
		return ev.setStringArrayFieldValue("process.parent.container.tags", &ev.BaseEvent.ProcessContext.Parent.ContainerContext.Tags, value)
	case "process.parent.created_at":
		return ev.setUint64FieldValue("process.parent.created_at", &ev.BaseEvent.ProcessContext.Parent.CreatedAt, value)
	case "process.parent.envp":
		return ev.setStringArrayFieldValue("process.parent.envp", &ev.BaseEvent.ProcessContext.Parent.Envp, value)
	case "process.parent.envs":
		return ev.setStringArrayFieldValue("process.parent.envs", &ev.BaseEvent.ProcessContext.Parent.Envs, value)
	case "process.parent.file.extension":
		return ev.setStringFieldValue("process.parent.file.extension", &ev.BaseEvent.ProcessContext.Parent.FileEvent.Extension, value)
	case "process.parent.file.name":
		return ev.setStringFieldValue("process.parent.file.name", &ev.BaseEvent.ProcessContext.Parent.FileEvent.BasenameStr, value)
	case "process.parent.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "process.parent.file.name.length"}
	case "process.parent.file.path":
		return ev.setStringFieldValue("process.parent.file.path", &ev.BaseEvent.ProcessContext.Parent.FileEvent.PathnameStr, value)
	case "process.parent.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "process.parent.file.path.length"}
	case "process.parent.pid":
		return ev.setUint32FieldValue("process.parent.pid", &ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid, value)
	case "process.parent.ppid":
		return ev.setUint32FieldValue("process.parent.ppid", &ev.BaseEvent.ProcessContext.Parent.PPid, value)
	case "process.parent.user":
		return ev.setStringFieldValue("process.parent.user", &ev.BaseEvent.ProcessContext.Parent.User, value)
	case "process.parent.user_sid":
		return ev.setStringFieldValue("process.parent.user_sid", &ev.BaseEvent.ProcessContext.Parent.OwnerSidString, value)
	case "process.pid":
		return ev.setUint32FieldValue("process.pid", &ev.BaseEvent.ProcessContext.Process.PIDContext.Pid, value)
	case "process.ppid":
		return ev.setUint32FieldValue("process.ppid", &ev.BaseEvent.ProcessContext.Process.PPid, value)
	case "process.user":
		return ev.setStringFieldValue("process.user", &ev.BaseEvent.ProcessContext.Process.User, value)
	case "process.user_sid":
		return ev.setStringFieldValue("process.user_sid", &ev.BaseEvent.ProcessContext.Process.OwnerSidString, value)
	case "rename.file.destination.device_path":
		return ev.setStringFieldValue("rename.file.destination.device_path", &ev.RenameFile.New.PathnameStr, value)
	case "rename.file.destination.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.device_path.length"}
	case "rename.file.destination.extension":
		return ev.setStringFieldValue("rename.file.destination.extension", &ev.RenameFile.New.Extension, value)
	case "rename.file.destination.name":
		return ev.setStringFieldValue("rename.file.destination.name", &ev.RenameFile.New.BasenameStr, value)
	case "rename.file.destination.name.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.name.length"}
	case "rename.file.destination.path":
		return ev.setStringFieldValue("rename.file.destination.path", &ev.RenameFile.New.UserPathnameStr, value)
	case "rename.file.destination.path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.destination.path.length"}
	case "rename.file.device_path":
		return ev.setStringFieldValue("rename.file.device_path", &ev.RenameFile.Old.PathnameStr, value)
	case "rename.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.device_path.length"}
	case "rename.file.extension":
		return ev.setStringFieldValue("rename.file.extension", &ev.RenameFile.Old.Extension, value)
	case "rename.file.name":
		return ev.setStringFieldValue("rename.file.name", &ev.RenameFile.Old.BasenameStr, value)
	case "rename.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.name.length"}
	case "rename.file.path":
		return ev.setStringFieldValue("rename.file.path", &ev.RenameFile.Old.UserPathnameStr, value)
	case "rename.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "rename.file.path.length"}
	case "set.registry.key_name":
		return ev.setStringFieldValue("set.registry.key_name", &ev.SetRegistryKeyValue.Registry.KeyName, value)
	case "set.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "set.registry.key_name.length"}
	case "set.registry.key_path":
		return ev.setStringFieldValue("set.registry.key_path", &ev.SetRegistryKeyValue.Registry.KeyPath, value)
	case "set.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "set.registry.key_path.length"}
	case "set.registry.value_name":
		return ev.setStringFieldValue("set.registry.value_name", &ev.SetRegistryKeyValue.ValueName, value)
	case "set.registry.value_name.length":
		return &eval.ErrFieldReadOnly{Field: "set.registry.value_name.length"}
	case "set.value_name":
		return ev.setStringFieldValue("set.value_name", &ev.SetRegistryKeyValue.ValueName, value)
	case "set_key_value.registry.key_name":
		return ev.setStringFieldValue("set_key_value.registry.key_name", &ev.SetRegistryKeyValue.Registry.KeyName, value)
	case "set_key_value.registry.key_name.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.registry.key_name.length"}
	case "set_key_value.registry.key_path":
		return ev.setStringFieldValue("set_key_value.registry.key_path", &ev.SetRegistryKeyValue.Registry.KeyPath, value)
	case "set_key_value.registry.key_path.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.registry.key_path.length"}
	case "set_key_value.registry.value_name":
		return ev.setStringFieldValue("set_key_value.registry.value_name", &ev.SetRegistryKeyValue.ValueName, value)
	case "set_key_value.registry.value_name.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.registry.value_name.length"}
	case "set_key_value.value_name":
		return ev.setStringFieldValue("set_key_value.value_name", &ev.SetRegistryKeyValue.ValueName, value)
	case "write.file.device_path":
		return ev.setStringFieldValue("write.file.device_path", &ev.WriteFile.File.PathnameStr, value)
	case "write.file.device_path.length":
		return &eval.ErrFieldReadOnly{Field: "write.file.device_path.length"}
	case "write.file.extension":
		return ev.setStringFieldValue("write.file.extension", &ev.WriteFile.File.Extension, value)
	case "write.file.name":
		return ev.setStringFieldValue("write.file.name", &ev.WriteFile.File.BasenameStr, value)
	case "write.file.name.length":
		return &eval.ErrFieldReadOnly{Field: "write.file.name.length"}
	case "write.file.path":
		return ev.setStringFieldValue("write.file.path", &ev.WriteFile.File.UserPathnameStr, value)
	case "write.file.path.length":
		return &eval.ErrFieldReadOnly{Field: "write.file.path.length"}
	}
	return &eval.ErrFieldNotFound{Field: field}
}
