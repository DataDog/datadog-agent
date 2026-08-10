// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

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

// Field evaluators. One named function per field: the evaluator of a field is
// what a CPU profile samples inside, and an anonymous closure of GetEvaluator
// tells nothing about which field it resolves.
func evalChangePermissionNewSd(ctx *eval.Context) string {
	ctx.AppendResolvedField("change_permission.new_sd")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveNewSecurityDescriptor(ev, &ev.ChangePermission)
}
func evalChangePermissionOldSd(ctx *eval.Context) string {
	ctx.AppendResolvedField("change_permission.old_sd")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveOldSecurityDescriptor(ev, &ev.ChangePermission)
}
func evalChangePermissionPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("change_permission.path")
	ev := ctx.Event.(*Event)
	return ev.ChangePermission.ObjectName
}
func evalChangePermissionType(ctx *eval.Context) string {
	ctx.AppendResolvedField("change_permission.type")
	ev := ctx.Event.(*Event)
	return ev.ChangePermission.ObjectType
}
func evalChangePermissionUserDomain(ctx *eval.Context) string {
	ctx.AppendResolvedField("change_permission.user_domain")
	ev := ctx.Event.(*Event)
	return ev.ChangePermission.UserDomain
}
func evalChangePermissionUsername(ctx *eval.Context) string {
	ctx.AppendResolvedField("change_permission.username")
	ev := ctx.Event.(*Event)
	return ev.ChangePermission.UserName
}
func evalCreateFileDevicePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("create.file.device_path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File)
}
func evalCreateFileDevicePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create.file.device_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.CreateNewFile.File))
}
func evalCreateFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("create.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.CreateNewFile.File)
}
func evalCreateFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("create.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File)
}
func evalCreateFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.CreateNewFile.File))
}
func evalCreateFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("create.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File)
}
func evalCreateFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.CreateNewFile.File))
}
func evalCreateRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("create.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.CreateRegistryKey.Registry.KeyName
}
func evalCreateRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.CreateRegistryKey.Registry.KeyName)
}
func evalCreateRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("create.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.CreateRegistryKey.Registry.KeyPath
}
func evalCreateRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.CreateRegistryKey.Registry.KeyPath)
}
func evalCreateKeyRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("create_key.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.CreateRegistryKey.Registry.KeyName
}
func evalCreateKeyRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create_key.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.CreateRegistryKey.Registry.KeyName)
}
func evalCreateKeyRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("create_key.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.CreateRegistryKey.Registry.KeyPath
}
func evalCreateKeyRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("create_key.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.CreateRegistryKey.Registry.KeyPath)
}
func evalDeleteFileDevicePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete.file.device_path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File)
}
func evalDeleteFileDevicePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete.file.device_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.DeleteFile.File))
}
func evalDeleteFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.DeleteFile.File)
}
func evalDeleteFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File)
}
func evalDeleteFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.DeleteFile.File))
}
func evalDeleteFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File)
}
func evalDeleteFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.DeleteFile.File))
}
func evalDeleteRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.DeleteRegistryKey.Registry.KeyName
}
func evalDeleteRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.DeleteRegistryKey.Registry.KeyName)
}
func evalDeleteRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.DeleteRegistryKey.Registry.KeyPath
}
func evalDeleteRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.DeleteRegistryKey.Registry.KeyPath)
}
func evalDeleteKeyRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete_key.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.DeleteRegistryKey.Registry.KeyName
}
func evalDeleteKeyRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete_key.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.DeleteRegistryKey.Registry.KeyName)
}
func evalDeleteKeyRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("delete_key.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.DeleteRegistryKey.Registry.KeyPath
}
func evalDeleteKeyRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("delete_key.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.DeleteRegistryKey.Registry.KeyPath)
}
func evalEventHostname(ctx *eval.Context) string {
	ctx.AppendResolvedField("event.hostname")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveHostname(ev, &ev.BaseEvent)
}
func evalEventOrigin(ctx *eval.Context) string {
	ctx.AppendResolvedField("event.origin")
	ev := ctx.Event.(*Event)
	return ev.BaseEvent.Origin
}
func evalEventOs(ctx *eval.Context) string {
	ctx.AppendResolvedField("event.os")
	ev := ctx.Event.(*Event)
	return ev.BaseEvent.Os
}
func evalEventRuleTags(ctx *eval.Context) []string {
	ctx.AppendResolvedField("event.rule.tags")
	ev := ctx.Event.(*Event)
	return ev.BaseEvent.RuleTags
}
func evalEventService(ctx *eval.Context) string {
	ctx.AppendResolvedField("event.service")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveService(ev, &ev.BaseEvent)
}
func evalEventSource(ctx *eval.Context) string {
	ctx.AppendResolvedField("event.source")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveSource(ev, &ev.BaseEvent)
}
func evalEventTimestamp(ctx *eval.Context) int {
	ctx.AppendResolvedField("event.timestamp")
	ev := ctx.Event.(*Event)
	return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent))
}
func evalExecCmdline(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.cmdline")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exec.Process)
}
func evalExecContainerCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("exec.container.created_at")
	ev := ctx.Event.(*Event)
	return int(ev.Exec.Process.ContainerContext.CreatedAt)
}
func evalExecContainerId(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.container.id")
	ev := ctx.Event.(*Event)
	return string(ev.Exec.Process.ContainerContext.ContainerID)
}
func evalExecContainerTags(ctx *eval.Context) []string {
	ctx.AppendResolvedField("exec.container.tags")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exec.Process.ContainerContext)
}
func evalExecCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("exec.created_at")
	ev := ctx.Event.(*Event)
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exec.Process))
}
func evalExecEnvp(ctx *eval.Context) []string {
	ctx.AppendResolvedField("exec.envp")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exec.Process)
}
func evalExecEnvs(ctx *eval.Context) []string {
	ctx.AppendResolvedField("exec.envs")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exec.Process)
}
func evalExecFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exec.Process.FileEvent)
}
func evalExecFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent)
}
func evalExecFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("exec.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exec.Process.FileEvent))
}
func evalExecFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
}
func evalExecFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("exec.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent))
}
func evalExecPid(ctx *eval.Context) int {
	ctx.AppendResolvedField("exec.pid")
	ev := ctx.Event.(*Event)
	return int(ev.Exec.Process.PIDContext.Pid)
}
func evalExecPpid(ctx *eval.Context) int {
	ctx.AppendResolvedField("exec.ppid")
	ev := ctx.Event.(*Event)
	return int(ev.Exec.Process.PPid)
}
func evalExecUser(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.user")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveUser(ev, ev.Exec.Process)
}
func evalExecUserSid(ctx *eval.Context) string {
	ctx.AppendResolvedField("exec.user_sid")
	ev := ctx.Event.(*Event)
	return ev.Exec.Process.OwnerSidString
}
func evalExitCause(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.cause")
	ev := ctx.Event.(*Event)
	return int(ev.Exit.Cause)
}
func evalExitCmdline(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.cmdline")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.Exit.Process)
}
func evalExitCode(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.code")
	ev := ctx.Event.(*Event)
	return int(ev.Exit.Code)
}
func evalExitContainerCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.container.created_at")
	ev := ctx.Event.(*Event)
	return int(ev.Exit.Process.ContainerContext.CreatedAt)
}
func evalExitContainerId(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.container.id")
	ev := ctx.Event.(*Event)
	return string(ev.Exit.Process.ContainerContext.ContainerID)
}
func evalExitContainerTags(ctx *eval.Context) []string {
	ctx.AppendResolvedField("exit.container.tags")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveContainerTags(ev, &ev.Exit.Process.ContainerContext)
}
func evalExitCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.created_at")
	ev := ctx.Event.(*Event)
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.Exit.Process))
}
func evalExitEnvp(ctx *eval.Context) []string {
	ctx.AppendResolvedField("exit.envp")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.Exit.Process)
}
func evalExitEnvs(ctx *eval.Context) []string {
	ctx.AppendResolvedField("exit.envs")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.Exit.Process)
}
func evalExitFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileExtension(ev, &ev.Exit.Process.FileEvent)
}
func evalExitFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent)
}
func evalExitFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.Exit.Process.FileEvent))
}
func evalExitFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
}
func evalExitFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent))
}
func evalExitPid(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.pid")
	ev := ctx.Event.(*Event)
	return int(ev.Exit.Process.PIDContext.Pid)
}
func evalExitPpid(ctx *eval.Context) int {
	ctx.AppendResolvedField("exit.ppid")
	ev := ctx.Event.(*Event)
	return int(ev.Exit.Process.PPid)
}
func evalExitUser(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.user")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveUser(ev, ev.Exit.Process)
}
func evalExitUserSid(ctx *eval.Context) string {
	ctx.AppendResolvedField("exit.user_sid")
	ev := ctx.Event.(*Event)
	return ev.Exit.Process.OwnerSidString
}
func evalOpenRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("open.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.OpenRegistryKey.Registry.KeyName
}
func evalOpenRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("open.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.OpenRegistryKey.Registry.KeyName)
}
func evalOpenRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("open.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.OpenRegistryKey.Registry.KeyPath
}
func evalOpenRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("open.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.OpenRegistryKey.Registry.KeyPath)
}
func evalOpenKeyRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("open_key.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.OpenRegistryKey.Registry.KeyName
}
func evalOpenKeyRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("open_key.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.OpenRegistryKey.Registry.KeyName)
}
func evalOpenKeyRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("open_key.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.OpenRegistryKey.Registry.KeyPath
}
func evalOpenKeyRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("open_key.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.OpenRegistryKey.Registry.KeyPath)
}
func evalProcessAncestorsCmdline(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.cmdline")
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
	if result, ok := ctx.StringCache["process.ancestors.cmdline"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
		return ev.FieldHandlers.ResolveProcessCmdLine(ev, &current.ProcessContext.Process)
	})
	ctx.StringCache["process.ancestors.cmdline"] = results
	return results
}
func evalProcessAncestorsContainerCreatedAt(ctx *eval.Context, regID eval.RegisterID) []int {
	ctx.AppendResolvedField("process.ancestors.container.created_at")
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
	if result, ok := ctx.IntCache["process.ancestors.container.created_at"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) int {
		return int(current.ProcessContext.Process.ContainerContext.CreatedAt)
	})
	ctx.IntCache["process.ancestors.container.created_at"] = results
	return results
}
func evalProcessAncestorsContainerId(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.container.id")
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
	if result, ok := ctx.StringCache["process.ancestors.container.id"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) string {
		return string(current.ProcessContext.Process.ContainerContext.ContainerID)
	})
	ctx.StringCache["process.ancestors.container.id"] = results
	return results
}
func evalProcessAncestorsContainerTags(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.container.tags")
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
	if result, ok := ctx.StringCache["process.ancestors.container.tags"]; ok {
		return result
	}
	results := newIteratorArray(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) []string {
		return ev.FieldHandlers.ResolveContainerTags(ev, &current.ProcessContext.Process.ContainerContext)
	})
	ctx.StringCache["process.ancestors.container.tags"] = results
	return results
}
func evalProcessAncestorsCreatedAt(ctx *eval.Context, regID eval.RegisterID) []int {
	ctx.AppendResolvedField("process.ancestors.created_at")
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
	if result, ok := ctx.IntCache["process.ancestors.created_at"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) int {
		return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &current.ProcessContext.Process))
	})
	ctx.IntCache["process.ancestors.created_at"] = results
	return results
}
func evalProcessAncestorsEnvp(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.envp")
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
	if result, ok := ctx.StringCache["process.ancestors.envp"]; ok {
		return result
	}
	results := newIteratorArray(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) []string {
		return ev.FieldHandlers.ResolveProcessEnvp(ev, &current.ProcessContext.Process)
	})
	ctx.StringCache["process.ancestors.envp"] = results
	return results
}
func evalProcessAncestorsEnvs(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.envs")
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
	if result, ok := ctx.StringCache["process.ancestors.envs"]; ok {
		return result
	}
	results := newIteratorArray(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) []string {
		return ev.FieldHandlers.ResolveProcessEnvs(ev, &current.ProcessContext.Process)
	})
	ctx.StringCache["process.ancestors.envs"] = results
	return results
}
func evalProcessAncestorsFileExtension(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.file.extension")
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
	if result, ok := ctx.StringCache["process.ancestors.file.extension"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
		return ev.FieldHandlers.ResolveFileExtension(ev, &current.ProcessContext.Process.FileEvent)
	})
	ctx.StringCache["process.ancestors.file.extension"] = results
	return results
}
func evalProcessAncestorsFileName(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.file.name")
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
	if result, ok := ctx.StringCache["process.ancestors.file.name"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
		return ev.FieldHandlers.ResolveFileBasename(ev, &current.ProcessContext.Process.FileEvent)
	})
	ctx.StringCache["process.ancestors.file.name"] = results
	return results
}
func evalProcessAncestorsFileNameLength(ctx *eval.Context, regID eval.RegisterID) []int {
	ctx.AppendResolvedField("process.ancestors.file.name.length")
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
	if result, ok := ctx.IntCache["process.ancestors.file.name.length"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) int {
		return len(ev.FieldHandlers.ResolveFileBasename(ev, &current.ProcessContext.Process.FileEvent))
	})
	ctx.IntCache["process.ancestors.file.name.length"] = results
	return results
}
func evalProcessAncestorsFilePath(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.file.path")
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
	if result, ok := ctx.StringCache["process.ancestors.file.path"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
		return ev.FieldHandlers.ResolveFilePath(ev, &current.ProcessContext.Process.FileEvent)
	})
	ctx.StringCache["process.ancestors.file.path"] = results
	return results
}
func evalProcessAncestorsFilePathLength(ctx *eval.Context, regID eval.RegisterID) []int {
	ctx.AppendResolvedField("process.ancestors.file.path.length")
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
	if result, ok := ctx.IntCache["process.ancestors.file.path.length"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) int {
		return len(ev.FieldHandlers.ResolveFilePath(ev, &current.ProcessContext.Process.FileEvent))
	})
	ctx.IntCache["process.ancestors.file.path.length"] = results
	return results
}
func evalProcessAncestorsLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.ancestors.length")
	iterator := &ProcessAncestorsIterator{}
	return iterator.Len(ctx)
}
func evalProcessAncestorsPid(ctx *eval.Context, regID eval.RegisterID) []int {
	ctx.AppendResolvedField("process.ancestors.pid")
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
	if result, ok := ctx.IntCache["process.ancestors.pid"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) int {
		return int(current.ProcessContext.Process.PIDContext.Pid)
	})
	ctx.IntCache["process.ancestors.pid"] = results
	return results
}
func evalProcessAncestorsPpid(ctx *eval.Context, regID eval.RegisterID) []int {
	ctx.AppendResolvedField("process.ancestors.ppid")
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
	if result, ok := ctx.IntCache["process.ancestors.ppid"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) int {
		return int(current.ProcessContext.Process.PPid)
	})
	ctx.IntCache["process.ancestors.ppid"] = results
	return results
}
func evalProcessAncestorsUser(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.user")
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
	if result, ok := ctx.StringCache["process.ancestors.user"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, ev, func(ev *Event, current *ProcessCacheEntry) string {
		return ev.FieldHandlers.ResolveUser(ev, &current.ProcessContext.Process)
	})
	ctx.StringCache["process.ancestors.user"] = results
	return results
}
func evalProcessAncestorsUserSid(ctx *eval.Context, regID eval.RegisterID) []string {
	ctx.AppendResolvedField("process.ancestors.user_sid")
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
	if result, ok := ctx.StringCache["process.ancestors.user_sid"]; ok {
		return result
	}
	results := newIterator(iterator, "BaseEvent.ProcessContext.Ancestor", ctx, nil, func(ev *Event, current *ProcessCacheEntry) string {
		return current.ProcessContext.Process.OwnerSidString
	})
	ctx.StringCache["process.ancestors.user_sid"] = results
	return results
}
func evalProcessCmdline(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.cmdline")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, &ev.BaseEvent.ProcessContext.Process)
}
func evalProcessContainerCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.container.created_at")
	ev := ctx.Event.(*Event)
	return int(ev.BaseEvent.ProcessContext.Process.ContainerContext.CreatedAt)
}
func evalProcessContainerId(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.container.id")
	ev := ctx.Event.(*Event)
	return string(ev.BaseEvent.ProcessContext.Process.ContainerContext.ContainerID)
}
func evalProcessContainerTags(ctx *eval.Context) []string {
	ctx.AppendResolvedField("process.container.tags")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Process.ContainerContext)
}
func evalProcessCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.created_at")
	ev := ctx.Event.(*Event)
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, &ev.BaseEvent.ProcessContext.Process))
}
func evalProcessEnvp(ctx *eval.Context) []string {
	ctx.AppendResolvedField("process.envp")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessEnvp(ev, &ev.BaseEvent.ProcessContext.Process)
}
func evalProcessEnvs(ctx *eval.Context) []string {
	ctx.AppendResolvedField("process.envs")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveProcessEnvs(ev, &ev.BaseEvent.ProcessContext.Process)
}
func evalProcessFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}
func evalProcessFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}
func evalProcessFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
}
func evalProcessFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
}
func evalProcessFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent))
}
func evalProcessParentCmdline(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.cmdline")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveProcessCmdLine(ev, ev.BaseEvent.ProcessContext.Parent)
}
func evalProcessParentContainerCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.parent.container.created_at")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	return int(ev.BaseEvent.ProcessContext.Parent.ContainerContext.CreatedAt)
}
func evalProcessParentContainerId(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.container.id")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return string(ev.BaseEvent.ProcessContext.Parent.ContainerContext.ContainerID)
}
func evalProcessParentContainerTags(ctx *eval.Context) []string {
	ctx.AppendResolvedField("process.parent.container.tags")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveContainerTags(ev, &ev.BaseEvent.ProcessContext.Parent.ContainerContext)
}
func evalProcessParentCreatedAt(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.parent.created_at")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	return int(ev.FieldHandlers.ResolveProcessCreatedAt(ev, ev.BaseEvent.ProcessContext.Parent))
}
func evalProcessParentEnvp(ctx *eval.Context) []string {
	ctx.AppendResolvedField("process.parent.envp")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvp(ev, ev.BaseEvent.ProcessContext.Parent)
}
func evalProcessParentEnvs(ctx *eval.Context) []string {
	ctx.AppendResolvedField("process.parent.envs")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return []string{}
	}
	return ev.FieldHandlers.ResolveProcessEnvs(ev, ev.BaseEvent.ProcessContext.Parent)
}
func evalProcessParentFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.file.extension")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileExtension(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}
func evalProcessParentFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.file.name")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}
func evalProcessParentFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.parent.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
}
func evalProcessParentFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.file.path")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent)
}
func evalProcessParentFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.parent.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Parent.FileEvent))
}
func evalProcessParentPid(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.parent.pid")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid)
}
func evalProcessParentPpid(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.parent.ppid")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return 0
	}
	return int(ev.BaseEvent.ProcessContext.Parent.PPid)
}
func evalProcessParentUser(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.user")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.FieldHandlers.ResolveUser(ev, ev.BaseEvent.ProcessContext.Parent)
}
func evalProcessParentUserSid(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.parent.user_sid")
	ev := ctx.Event.(*Event)
	if !ev.BaseEvent.ProcessContext.HasParent() {
		return ""
	}
	return ev.BaseEvent.ProcessContext.Parent.OwnerSidString
}
func evalProcessPid(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.pid")
	ev := ctx.Event.(*Event)
	return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid)
}
func evalProcessPpid(ctx *eval.Context) int {
	ctx.AppendResolvedField("process.ppid")
	ev := ctx.Event.(*Event)
	return int(ev.BaseEvent.ProcessContext.Process.PPid)
}
func evalProcessUser(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.user")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveUser(ev, &ev.BaseEvent.ProcessContext.Process)
}
func evalProcessUserSid(ctx *eval.Context) string {
	ctx.AppendResolvedField("process.user_sid")
	ev := ctx.Event.(*Event)
	return ev.BaseEvent.ProcessContext.Process.OwnerSidString
}
func evalRenameFileDestinationDevicePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.destination.device_path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New)
}
func evalRenameFileDestinationDevicePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("rename.file.destination.device_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.New))
}
func evalRenameFileDestinationExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.destination.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.New)
}
func evalRenameFileDestinationName(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.destination.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New)
}
func evalRenameFileDestinationNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("rename.file.destination.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.New))
}
func evalRenameFileDestinationPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.destination.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New)
}
func evalRenameFileDestinationPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("rename.file.destination.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.New))
}
func evalRenameFileDevicePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.device_path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old)
}
func evalRenameFileDevicePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("rename.file.device_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.RenameFile.Old))
}
func evalRenameFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.RenameFile.Old)
}
func evalRenameFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old)
}
func evalRenameFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("rename.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.RenameFile.Old))
}
func evalRenameFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("rename.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old)
}
func evalRenameFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("rename.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.RenameFile.Old))
}
func evalSetRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("set.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.Registry.KeyName
}
func evalSetRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("set.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.SetRegistryKeyValue.Registry.KeyName)
}
func evalSetRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("set.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.Registry.KeyPath
}
func evalSetRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("set.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.SetRegistryKeyValue.Registry.KeyPath)
}
func evalSetRegistryValueName(ctx *eval.Context) string {
	ctx.AppendResolvedField("set.registry.value_name")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.ValueName
}
func evalSetRegistryValueNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("set.registry.value_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.SetRegistryKeyValue.ValueName)
}
func evalSetValueName(ctx *eval.Context) string {
	ctx.AppendResolvedField("set.value_name")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.ValueName
}
func evalSetKeyValueRegistryKeyName(ctx *eval.Context) string {
	ctx.AppendResolvedField("set_key_value.registry.key_name")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.Registry.KeyName
}
func evalSetKeyValueRegistryKeyNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("set_key_value.registry.key_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.SetRegistryKeyValue.Registry.KeyName)
}
func evalSetKeyValueRegistryKeyPath(ctx *eval.Context) string {
	ctx.AppendResolvedField("set_key_value.registry.key_path")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.Registry.KeyPath
}
func evalSetKeyValueRegistryKeyPathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("set_key_value.registry.key_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.SetRegistryKeyValue.Registry.KeyPath)
}
func evalSetKeyValueRegistryValueName(ctx *eval.Context) string {
	ctx.AppendResolvedField("set_key_value.registry.value_name")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.ValueName
}
func evalSetKeyValueRegistryValueNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("set_key_value.registry.value_name.length")
	ev := ctx.Event.(*Event)
	return len(ev.SetRegistryKeyValue.ValueName)
}
func evalSetKeyValueValueName(ctx *eval.Context) string {
	ctx.AppendResolvedField("set_key_value.value_name")
	ev := ctx.Event.(*Event)
	return ev.SetRegistryKeyValue.ValueName
}
func evalWriteFileDevicePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("write.file.device_path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File)
}
func evalWriteFileDevicePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("write.file.device_path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFilePath(ev, &ev.WriteFile.File))
}
func evalWriteFileExtension(ctx *eval.Context) string {
	ctx.AppendResolvedField("write.file.extension")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileExtension(ev, &ev.WriteFile.File)
}
func evalWriteFileName(ctx *eval.Context) string {
	ctx.AppendResolvedField("write.file.name")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File)
}
func evalWriteFileNameLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("write.file.name.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFimFileBasename(ev, &ev.WriteFile.File))
}
func evalWriteFilePath(ctx *eval.Context) string {
	ctx.AppendResolvedField("write.file.path")
	ev := ctx.Event.(*Event)
	return ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File)
}
func evalWriteFilePathLength(ctx *eval.Context) int {
	ctx.AppendResolvedField("write.file.path.length")
	ev := ctx.Event.(*Event)
	return len(ev.FieldHandlers.ResolveFileUserPath(ev, &ev.WriteFile.File))
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
			EvalFnc: evalChangePermissionNewSd,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "change_permission.old_sd":
		return &eval.StringEvaluator{
			EvalFnc: evalChangePermissionOldSd,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "change_permission.path":
		return &eval.StringEvaluator{
			EvalFnc: evalChangePermissionPath,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "change_permission.type":
		return &eval.StringEvaluator{
			EvalFnc: evalChangePermissionType,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "change_permission.user_domain":
		return &eval.StringEvaluator{
			EvalFnc: evalChangePermissionUserDomain,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "change_permission.username":
		return &eval.StringEvaluator{
			EvalFnc: evalChangePermissionUsername,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "create.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalCreateFileDevicePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalCreateFileDevicePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalCreateFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalCreateFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalCreateFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalCreateFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalCreateFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "create.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalCreateRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "create.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalCreateRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "create.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalCreateRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "create.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalCreateRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "create_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalCreateKeyRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "create_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalCreateKeyRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "create_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalCreateKeyRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "create_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalCreateKeyRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "delete.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalDeleteFileDevicePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalDeleteFileDevicePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalDeleteFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalDeleteFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalDeleteFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalDeleteFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalDeleteFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "delete.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalDeleteRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "delete.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalDeleteRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "delete.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalDeleteRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "delete.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalDeleteRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "delete_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalDeleteKeyRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "delete_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalDeleteKeyRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "delete_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalDeleteKeyRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "delete_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalDeleteKeyRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "event.hostname":
		return &eval.StringEvaluator{
			EvalFnc: evalEventHostname,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "event.origin":
		return &eval.StringEvaluator{
			EvalFnc: evalEventOrigin,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "event.os":
		return &eval.StringEvaluator{
			EvalFnc: evalEventOs,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "event.rule.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalEventRuleTags,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "event.service":
		return &eval.StringEvaluator{
			EvalFnc: evalEventService,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "event.source":
		return &eval.StringEvaluator{
			EvalFnc: evalEventSource,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "event.timestamp":
		return &eval.IntEvaluator{
			EvalFnc: evalEventTimestamp,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exec.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalExecCmdline,
			Field:       field,
			Weight:      200 * eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exec.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalExecContainerCreatedAt,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exec.container.id":
		return &eval.StringEvaluator{
			EvalFnc: evalExecContainerId,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exec.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalExecContainerTags,
			Field:   field,
			Weight:  9999 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exec.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalExecCreatedAt,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exec.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalExecEnvp,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exec.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalExecEnvs,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exec.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalExecFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exec.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalExecFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exec.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalExecFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exec.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalExecFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exec.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalExecFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exec.pid":
		return &eval.IntEvaluator{
			EvalFnc: evalExecPid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exec.ppid":
		return &eval.IntEvaluator{
			EvalFnc: evalExecPpid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exec.user":
		return &eval.StringEvaluator{
			EvalFnc: evalExecUser,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exec.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: evalExecUserSid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.cause":
		return &eval.IntEvaluator{
			EvalFnc: evalExitCause,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalExitCmdline,
			Field:       field,
			Weight:      200 * eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exit.code":
		return &eval.IntEvaluator{
			EvalFnc: evalExitCode,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalExitContainerCreatedAt,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.container.id":
		return &eval.StringEvaluator{
			EvalFnc: evalExitContainerId,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalExitContainerTags,
			Field:   field,
			Weight:  9999 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exit.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalExitCreatedAt,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exit.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalExitEnvp,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exit.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalExitEnvs,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exit.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalExitFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exit.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalExitFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exit.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalExitFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exit.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalExitFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exit.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalExitFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "exit.pid":
		return &eval.IntEvaluator{
			EvalFnc: evalExitPid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.ppid":
		return &eval.IntEvaluator{
			EvalFnc: evalExitPpid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "exit.user":
		return &eval.StringEvaluator{
			EvalFnc: evalExitUser,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "exit.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: evalExitUserSid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "open.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalOpenRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "open.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalOpenRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "open.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalOpenRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "open.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalOpenRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "open_key.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalOpenKeyRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "open_key.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalOpenKeyRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "open_key.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalOpenKeyRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "open_key.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalOpenKeyRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "process.ancestors.cmdline":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsCmdline(ctx, regID)
			},
			Field:  field,
			Weight: 200 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.container.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				return evalProcessAncestorsContainerCreatedAt(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.container.id":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsContainerId(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsContainerTags(ctx, regID)
			},
			Field:  field,
			Weight: 9999 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.created_at":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				return evalProcessAncestorsCreatedAt(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsEnvp(ctx, regID)
			},
			Field:  field,
			Weight: 100 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsEnvs(ctx, regID)
			},
			Field:  field,
			Weight: 100 * eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.extension":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsFileExtension(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.name":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsFileName(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.name.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc: func(ctx *eval.Context) []int {
				return evalProcessAncestorsFileNameLength(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.path":
		return &eval.StringArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsFilePath(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.file.path.length":
		return &eval.IntArrayEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc: func(ctx *eval.Context) []int {
				return evalProcessAncestorsFilePathLength(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.length":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessAncestorsLength,
			Field:   field,
			Weight:  eval.IteratorWeight,
			Offset:  offset,
		}, nil
	case "process.ancestors.pid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				return evalProcessAncestorsPid(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.ppid":
		return &eval.IntArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []int {
				return evalProcessAncestorsPpid(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.user":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsUser(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.ancestors.user_sid":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				return evalProcessAncestorsUserSid(ctx, regID)
			},
			Field:  field,
			Weight: eval.IteratorWeight,
			Offset: offset,
		}, nil
	case "process.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalProcessCmdline,
			Field:       field,
			Weight:      200 * eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessContainerCreatedAt,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.container.id":
		return &eval.StringEvaluator{
			EvalFnc: evalProcessContainerId,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalProcessContainerTags,
			Field:   field,
			Weight:  9999 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessCreatedAt,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalProcessEnvp,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalProcessEnvs,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalProcessFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalProcessFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalProcessFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalProcessFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalProcessFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.cmdline":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalProcessParentCmdline,
			Field:       field,
			Weight:      200 * eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.container.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessParentContainerCreatedAt,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.parent.container.id":
		return &eval.StringEvaluator{
			EvalFnc: evalProcessParentContainerId,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.parent.container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalProcessParentContainerTags,
			Field:   field,
			Weight:  9999 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.parent.created_at":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessParentCreatedAt,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.parent.envp":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalProcessParentEnvp,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.parent.envs":
		return &eval.StringArrayEvaluator{
			EvalFnc: evalProcessParentEnvs,
			Field:   field,
			Weight:  100 * eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.parent.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalProcessParentFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalProcessParentFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalProcessParentFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalProcessParentFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalProcessParentFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "process.parent.pid":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessParentPid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.parent.ppid":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessParentPpid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.parent.user":
		return &eval.StringEvaluator{
			EvalFnc: evalProcessParentUser,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.parent.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: evalProcessParentUserSid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.pid":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessPid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.ppid":
		return &eval.IntEvaluator{
			EvalFnc: evalProcessPpid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "process.user":
		return &eval.StringEvaluator{
			EvalFnc: evalProcessUser,
			Field:   field,
			Weight:  eval.HandlerWeight,
			Offset:  offset,
		}, nil
	case "process.user_sid":
		return &eval.StringEvaluator{
			EvalFnc: evalProcessUserSid,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "rename.file.destination.device_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFileDestinationDevicePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.destination.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFileDestinationDevicePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.destination.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalRenameFileDestinationExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.destination.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalRenameFileDestinationName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.destination.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalRenameFileDestinationNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.destination.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFileDestinationPath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.destination.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFileDestinationPathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFileDevicePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFileDevicePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalRenameFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalRenameFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalRenameFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "rename.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalRenameFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "set.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalSetRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalSetRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalSetRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "set.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalSetRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "set.registry.value_name":
		return &eval.StringEvaluator{
			EvalFnc: evalSetRegistryValueName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set.registry.value_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalSetRegistryValueNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set.value_name":
		return &eval.StringEvaluator{
			EvalFnc: evalSetValueName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set_key_value.registry.key_name":
		return &eval.StringEvaluator{
			EvalFnc: evalSetKeyValueRegistryKeyName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set_key_value.registry.key_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalSetKeyValueRegistryKeyNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set_key_value.registry.key_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalSetKeyValueRegistryKeyPath,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "set_key_value.registry.key_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalSetKeyValueRegistryKeyPathLength,
			Field:       field,
			Weight:      eval.FunctionWeight,
			Offset:      offset,
		}, nil
	case "set_key_value.registry.value_name":
		return &eval.StringEvaluator{
			EvalFnc: evalSetKeyValueRegistryValueName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set_key_value.registry.value_name.length":
		return &eval.IntEvaluator{
			EvalFnc: evalSetKeyValueRegistryValueNameLength,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "set_key_value.value_name":
		return &eval.StringEvaluator{
			EvalFnc: evalSetKeyValueValueName,
			Field:   field,
			Weight:  eval.FunctionWeight,
			Offset:  offset,
		}, nil
	case "write.file.device_path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalWriteFileDevicePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "write.file.device_path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalWriteFileDevicePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "write.file.extension":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp, eval.ExtensionCmp},
			EvalFnc:     evalWriteFileExtension,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "write.file.name":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalWriteFileName,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "write.file.name.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.CaseInsensitiveCmp},
			EvalFnc:     evalWriteFileNameLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "write.file.path":
		return &eval.StringEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalWriteFilePath,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
		}, nil
	case "write.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: []*eval.OpOverrides{eval.WindowsPathCmp},
			EvalFnc:     evalWriteFilePathLength,
			Field:       field,
			Weight:      eval.HandlerWeight,
			Offset:      offset,
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
