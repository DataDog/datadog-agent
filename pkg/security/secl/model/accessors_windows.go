// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"reflect"
)

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {
	case "process.ancestors":
		return &ProcessAncestorsIterator{}, nil
	}
	return nil, &eval.ErrIteratorNotSupported{Field: field}
}
func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{
		eval.EventType("exec"),
		eval.EventType("exit"),
		eval.EventType("file.create"),
		eval.EventType("registry.create_key"),
		eval.EventType("registry.delete_key"),
		eval.EventType("registry.open_key"),
		eval.EventType("registry.set_key_value"),
	}
}
func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
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
	case "container.tags":
		return &eval.StringArrayEvaluator{
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext)
			},
			Field:  field,
			Weight: 9999 * eval.HandlerWeight,
		}, nil
	case "create.name":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFileBasename(ev, &ev.CreateNewFile.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.name.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFileBasename(ev, &ev.CreateNewFile.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.path":
		return &eval.StringEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.CreateNewFile.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.FieldHandlers.ResolveFilePath(ev, &ev.CreateNewFile.FileEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "create_key.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.RegistryEvent.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.RegistryEvent.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.RegistryEvent.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.path.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.RegistryEvent.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.value":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.CreateRegistryKey.RegistryEvent.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "create_key.value.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.CreateRegistryKey.RegistryEvent.ValueName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.RegistryEvent.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.RegistryEvent.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.RegistryEvent.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.path.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.RegistryEvent.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.value":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.DeleteRegistryKey.RegistryEvent.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "delete_key.value.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.DeleteRegistryKey.RegistryEvent.ValueName)
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
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exec.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exec.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
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
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.Exit.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "exit.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
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
	case "open_key.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.RegistryEvent.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.RegistryEvent.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.RegistryEvent.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.path.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.RegistryEvent.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.value":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.OpenRegistryKey.RegistryEvent.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "open_key.value.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.OpenRegistryKey.RegistryEvent.ValueName)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []string {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.StringCache[field]; ok {
					return result
				}
				var results []string
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) []int {
				ev := ctx.Event.(*Event)
				if result, ok := ctx.IntCache[field]; ok {
					return result
				}
				var results []int
				iterator := &ProcessAncestorsIterator{}
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
					result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
					results = append(results, result)
					value = iterator.Next()
				}
				ctx.IntCache[field] = results
				return results
			}, Field: field,
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
				value := iterator.Front(ctx)
				for value != nil {
					element := (*ProcessCacheEntry)(value)
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
			OpOverrides: eval.CaseInsensitiveCmp,
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.FieldHandlers.ResolveFilePath(ev, &ev.BaseEvent.ProcessContext.Process.FileEvent)
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "process.file.path.length":
		return &eval.IntEvaluator{
			OpOverrides: eval.CaseInsensitiveCmp,
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
			OpOverrides: eval.CaseInsensitiveCmp,
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
			OpOverrides: eval.CaseInsensitiveCmp,
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
	case "set_key_value.name":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.RegistryEvent.KeyName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.name.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.RegistryEvent.KeyName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.path":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.RegistryEvent.KeyPath
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.path.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.RegistryEvent.KeyPath)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.value":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.SetRegistryKeyValue.RegistryEvent.ValueName
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "set_key_value.value.length":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return len(ev.SetRegistryKeyValue.RegistryEvent.ValueName)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFields() []eval.Field {
	return []eval.Field{
		"container.created_at",
		"container.id",
		"container.tags",
		"create.name",
		"create.name.length",
		"create.path",
		"create.path.length",
		"create_key.name",
		"create_key.name.length",
		"create_key.path",
		"create_key.path.length",
		"create_key.value",
		"create_key.value.length",
		"delete_key.name",
		"delete_key.name.length",
		"delete_key.path",
		"delete_key.path.length",
		"delete_key.value",
		"delete_key.value.length",
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
		"open_key.name",
		"open_key.name.length",
		"open_key.path",
		"open_key.path.length",
		"open_key.value",
		"open_key.value.length",
		"process.ancestors.cmdline",
		"process.ancestors.container.id",
		"process.ancestors.created_at",
		"process.ancestors.envp",
		"process.ancestors.envs",
		"process.ancestors.file.name",
		"process.ancestors.file.name.length",
		"process.ancestors.file.path",
		"process.ancestors.file.path.length",
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
		"set_key_value.name",
		"set_key_value.name.length",
		"set_key_value.path",
		"set_key_value.path.length",
		"set_key_value.value",
		"set_key_value.value.length",
	}
}
func (ev *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
	case "container.created_at":
		return int(ev.FieldHandlers.ResolveContainerCreatedAt(ev, ev.BaseEvent.ContainerContext)), nil
	case "container.id":
		return ev.FieldHandlers.ResolveContainerID(ev, ev.BaseEvent.ContainerContext), nil
	case "container.tags":
		return ev.FieldHandlers.ResolveContainerTags(ev, ev.BaseEvent.ContainerContext), nil
	case "create.name":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.CreateNewFile.FileEvent), nil
	case "create.name.length":
		return ev.FieldHandlers.ResolveFileBasename(ev, &ev.CreateNewFile.FileEvent), nil
	case "create.path":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.CreateNewFile.FileEvent), nil
	case "create.path.length":
		return ev.FieldHandlers.ResolveFilePath(ev, &ev.CreateNewFile.FileEvent), nil
	case "create_key.name":
		return ev.CreateRegistryKey.RegistryEvent.KeyName, nil
	case "create_key.name.length":
		return len(ev.CreateRegistryKey.RegistryEvent.KeyName), nil
	case "create_key.path":
		return ev.CreateRegistryKey.RegistryEvent.KeyPath, nil
	case "create_key.path.length":
		return len(ev.CreateRegistryKey.RegistryEvent.KeyPath), nil
	case "create_key.value":
		return ev.CreateRegistryKey.RegistryEvent.ValueName, nil
	case "create_key.value.length":
		return len(ev.CreateRegistryKey.RegistryEvent.ValueName), nil
	case "delete_key.name":
		return ev.DeleteRegistryKey.RegistryEvent.KeyName, nil
	case "delete_key.name.length":
		return len(ev.DeleteRegistryKey.RegistryEvent.KeyName), nil
	case "delete_key.path":
		return ev.DeleteRegistryKey.RegistryEvent.KeyPath, nil
	case "delete_key.path.length":
		return len(ev.DeleteRegistryKey.RegistryEvent.KeyPath), nil
	case "delete_key.value":
		return ev.DeleteRegistryKey.RegistryEvent.ValueName, nil
	case "delete_key.value.length":
		return len(ev.DeleteRegistryKey.RegistryEvent.ValueName), nil
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
	case "open_key.name":
		return ev.OpenRegistryKey.RegistryEvent.KeyName, nil
	case "open_key.name.length":
		return len(ev.OpenRegistryKey.RegistryEvent.KeyName), nil
	case "open_key.path":
		return ev.OpenRegistryKey.RegistryEvent.KeyPath, nil
	case "open_key.path.length":
		return len(ev.OpenRegistryKey.RegistryEvent.KeyPath), nil
	case "open_key.value":
		return ev.OpenRegistryKey.RegistryEvent.ValueName, nil
	case "open_key.value.length":
		return len(ev.OpenRegistryKey.RegistryEvent.ValueName), nil
	case "process.ancestors.cmdline":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.name.length":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(ev.FieldHandlers.ResolveFileBasename(ev, &element.ProcessContext.Process.FileEvent))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path":
		var values []string
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent)
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.file.path.length":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
			result := len(ev.FieldHandlers.ResolveFilePath(ev, &element.ProcessContext.Process.FileEvent))
			values = append(values, result)
			ptr = iterator.Next()
		}
		return values, nil
	case "process.ancestors.pid":
		var values []int
		ctx := eval.NewContext(ev)
		iterator := &ProcessAncestorsIterator{}
		ptr := iterator.Front(ctx)
		for ptr != nil {
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
			element := (*ProcessCacheEntry)(ptr)
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
	case "set_key_value.name":
		return ev.SetRegistryKeyValue.RegistryEvent.KeyName, nil
	case "set_key_value.name.length":
		return len(ev.SetRegistryKeyValue.RegistryEvent.KeyName), nil
	case "set_key_value.path":
		return ev.SetRegistryKeyValue.RegistryEvent.KeyPath, nil
	case "set_key_value.path.length":
		return len(ev.SetRegistryKeyValue.RegistryEvent.KeyPath), nil
	case "set_key_value.value":
		return ev.SetRegistryKeyValue.RegistryEvent.ValueName, nil
	case "set_key_value.value.length":
		return len(ev.SetRegistryKeyValue.RegistryEvent.ValueName), nil
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {
	case "container.created_at":
		return "*", nil
	case "container.id":
		return "*", nil
	case "container.tags":
		return "*", nil
	case "create.name":
		return "file.create", nil
	case "create.name.length":
		return "file.create", nil
	case "create.path":
		return "file.create", nil
	case "create.path.length":
		return "file.create", nil
	case "create_key.name":
		return "registry.create_key", nil
	case "create_key.name.length":
		return "registry.create_key", nil
	case "create_key.path":
		return "registry.create_key", nil
	case "create_key.path.length":
		return "registry.create_key", nil
	case "create_key.value":
		return "registry.create_key", nil
	case "create_key.value.length":
		return "registry.create_key", nil
	case "delete_key.name":
		return "registry.delete_key", nil
	case "delete_key.name.length":
		return "registry.delete_key", nil
	case "delete_key.path":
		return "registry.delete_key", nil
	case "delete_key.path.length":
		return "registry.delete_key", nil
	case "delete_key.value":
		return "registry.delete_key", nil
	case "delete_key.value.length":
		return "registry.delete_key", nil
	case "event.service":
		return "*", nil
	case "event.timestamp":
		return "*", nil
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
	case "open_key.name":
		return "registry.open_key", nil
	case "open_key.name.length":
		return "registry.open_key", nil
	case "open_key.path":
		return "registry.open_key", nil
	case "open_key.path.length":
		return "registry.open_key", nil
	case "open_key.value":
		return "registry.open_key", nil
	case "open_key.value.length":
		return "registry.open_key", nil
	case "process.ancestors.cmdline":
		return "*", nil
	case "process.ancestors.container.id":
		return "*", nil
	case "process.ancestors.created_at":
		return "*", nil
	case "process.ancestors.envp":
		return "*", nil
	case "process.ancestors.envs":
		return "*", nil
	case "process.ancestors.file.name":
		return "*", nil
	case "process.ancestors.file.name.length":
		return "*", nil
	case "process.ancestors.file.path":
		return "*", nil
	case "process.ancestors.file.path.length":
		return "*", nil
	case "process.ancestors.pid":
		return "*", nil
	case "process.ancestors.ppid":
		return "*", nil
	case "process.ancestors.user":
		return "*", nil
	case "process.ancestors.user_sid":
		return "*", nil
	case "process.cmdline":
		return "*", nil
	case "process.container.id":
		return "*", nil
	case "process.created_at":
		return "*", nil
	case "process.envp":
		return "*", nil
	case "process.envs":
		return "*", nil
	case "process.file.name":
		return "*", nil
	case "process.file.name.length":
		return "*", nil
	case "process.file.path":
		return "*", nil
	case "process.file.path.length":
		return "*", nil
	case "process.parent.cmdline":
		return "*", nil
	case "process.parent.container.id":
		return "*", nil
	case "process.parent.created_at":
		return "*", nil
	case "process.parent.envp":
		return "*", nil
	case "process.parent.envs":
		return "*", nil
	case "process.parent.file.name":
		return "*", nil
	case "process.parent.file.name.length":
		return "*", nil
	case "process.parent.file.path":
		return "*", nil
	case "process.parent.file.path.length":
		return "*", nil
	case "process.parent.pid":
		return "*", nil
	case "process.parent.ppid":
		return "*", nil
	case "process.parent.user":
		return "*", nil
	case "process.parent.user_sid":
		return "*", nil
	case "process.pid":
		return "*", nil
	case "process.ppid":
		return "*", nil
	case "process.user":
		return "*", nil
	case "process.user_sid":
		return "*", nil
	case "set_key_value.name":
		return "registry.set_key_value", nil
	case "set_key_value.name.length":
		return "registry.set_key_value", nil
	case "set_key_value.path":
		return "registry.set_key_value", nil
	case "set_key_value.path.length":
		return "registry.set_key_value", nil
	case "set_key_value.value":
		return "registry.set_key_value", nil
	case "set_key_value.value.length":
		return "registry.set_key_value", nil
	}
	return "", &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
	case "container.created_at":
		return reflect.Int, nil
	case "container.id":
		return reflect.String, nil
	case "container.tags":
		return reflect.String, nil
	case "create.name":
		return reflect.String, nil
	case "create.name.length":
		return reflect.Int, nil
	case "create.path":
		return reflect.String, nil
	case "create.path.length":
		return reflect.Int, nil
	case "create_key.name":
		return reflect.String, nil
	case "create_key.name.length":
		return reflect.Int, nil
	case "create_key.path":
		return reflect.String, nil
	case "create_key.path.length":
		return reflect.Int, nil
	case "create_key.value":
		return reflect.String, nil
	case "create_key.value.length":
		return reflect.Int, nil
	case "delete_key.name":
		return reflect.String, nil
	case "delete_key.name.length":
		return reflect.Int, nil
	case "delete_key.path":
		return reflect.String, nil
	case "delete_key.path.length":
		return reflect.Int, nil
	case "delete_key.value":
		return reflect.String, nil
	case "delete_key.value.length":
		return reflect.Int, nil
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
	case "open_key.name":
		return reflect.String, nil
	case "open_key.name.length":
		return reflect.Int, nil
	case "open_key.path":
		return reflect.String, nil
	case "open_key.path.length":
		return reflect.Int, nil
	case "open_key.value":
		return reflect.String, nil
	case "open_key.value.length":
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
	case "set_key_value.name":
		return reflect.String, nil
	case "set_key_value.name.length":
		return reflect.Int, nil
	case "set_key_value.path":
		return reflect.String, nil
	case "set_key_value.path.length":
		return reflect.Int, nil
	case "set_key_value.value":
		return reflect.String, nil
	case "set_key_value.value.length":
		return reflect.Int, nil
	}
	return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {
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
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ContainerContext.ID"}
		}
		ev.BaseEvent.ContainerContext.ID = rv
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
	case "create.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateNewFile.FileEvent.BasenameStr"}
		}
		ev.CreateNewFile.FileEvent.BasenameStr = rv
		return nil
	case "create.name.length":
		return &eval.ErrFieldReadOnly{Field: "create.name.length"}
	case "create.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateNewFile.FileEvent.PathnameStr"}
		}
		ev.CreateNewFile.FileEvent.PathnameStr = rv
		return nil
	case "create.path.length":
		return &eval.ErrFieldReadOnly{Field: "create.path.length"}
	case "create_key.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.RegistryEvent.KeyName"}
		}
		ev.CreateRegistryKey.RegistryEvent.KeyName = rv
		return nil
	case "create_key.name.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.name.length"}
	case "create_key.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.RegistryEvent.KeyPath"}
		}
		ev.CreateRegistryKey.RegistryEvent.KeyPath = rv
		return nil
	case "create_key.path.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.path.length"}
	case "create_key.value":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "CreateRegistryKey.RegistryEvent.ValueName"}
		}
		ev.CreateRegistryKey.RegistryEvent.ValueName = rv
		return nil
	case "create_key.value.length":
		return &eval.ErrFieldReadOnly{Field: "create_key.value.length"}
	case "delete_key.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.RegistryEvent.KeyName"}
		}
		ev.DeleteRegistryKey.RegistryEvent.KeyName = rv
		return nil
	case "delete_key.name.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.name.length"}
	case "delete_key.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.RegistryEvent.KeyPath"}
		}
		ev.DeleteRegistryKey.RegistryEvent.KeyPath = rv
		return nil
	case "delete_key.path.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.path.length"}
	case "delete_key.value":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "DeleteRegistryKey.RegistryEvent.ValueName"}
		}
		ev.DeleteRegistryKey.RegistryEvent.ValueName = rv
		return nil
	case "delete_key.value.length":
		return &eval.ErrFieldReadOnly{Field: "delete_key.value.length"}
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
	case "open_key.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.RegistryEvent.KeyName"}
		}
		ev.OpenRegistryKey.RegistryEvent.KeyName = rv
		return nil
	case "open_key.name.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.name.length"}
	case "open_key.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.RegistryEvent.KeyPath"}
		}
		ev.OpenRegistryKey.RegistryEvent.KeyPath = rv
		return nil
	case "open_key.path.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.path.length"}
	case "open_key.value":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "OpenRegistryKey.RegistryEvent.ValueName"}
		}
		ev.OpenRegistryKey.RegistryEvent.ValueName = rv
		return nil
	case "open_key.value.length":
		return &eval.ErrFieldReadOnly{Field: "open_key.value.length"}
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
	case "set_key_value.name":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.RegistryEvent.KeyName"}
		}
		ev.SetRegistryKeyValue.RegistryEvent.KeyName = rv
		return nil
	case "set_key_value.name.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.name.length"}
	case "set_key_value.path":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.RegistryEvent.KeyPath"}
		}
		ev.SetRegistryKeyValue.RegistryEvent.KeyPath = rv
		return nil
	case "set_key_value.path.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.path.length"}
	case "set_key_value.value":
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "SetRegistryKeyValue.RegistryEvent.ValueName"}
		}
		ev.SetRegistryKeyValue.RegistryEvent.ValueName = rv
		return nil
	case "set_key_value.value.length":
		return &eval.ErrFieldReadOnly{Field: "set_key_value.value.length"}
	}
	return &eval.ErrFieldNotFound{Field: field}
}
