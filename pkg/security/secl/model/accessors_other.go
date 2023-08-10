// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build !linux && !windows
// +build !linux,!windows

package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"net"
	"reflect"
)

// Aliases used to avoid compilation error in case of unused imported package
type NetIP = net.IP

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {
	case "process.ancestors":
		return &ProcessAncestorsIterator{}, nil
	}
	return nil, &eval.ErrIteratorNotSupported{Field: field}
}
func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{}
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
	case "event.timestamp":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent))
			},
			Field:  field,
			Weight: eval.HandlerWeight,
		}, nil
	case "network.destination.ip":
		return &eval.CIDREvaluator{
			EvalFnc: func(ctx *eval.Context) net.IPNet {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.NetworkContext.Destination.IPNet
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.destination.port":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.NetworkContext.Destination.Port)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.l3_protocol":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.NetworkContext.L3Protocol)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.l4_protocol":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.NetworkContext.L4Protocol)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.size":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.NetworkContext.Size)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.source.ip":
		return &eval.CIDREvaluator{
			EvalFnc: func(ctx *eval.Context) net.IPNet {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.NetworkContext.Source.IPNet
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "network.source.port":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.NetworkContext.Source.Port)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ContainerID":
		return &eval.StringEvaluator{
			EvalFnc: func(ctx *eval.Context) string {
				ev := ctx.Event.(*Event)
				return ev.BaseEvent.ProcessContext.Process.ContainerID
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.Pid":
		return &eval.IntEvaluator{
			EvalFnc: func(ctx *eval.Context) int {
				ev := ctx.Event.(*Event)
				return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid)
			},
			Field:  field,
			Weight: eval.FunctionWeight,
		}, nil
	case "process.ancestors.ContainerID":
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
	case "process.ancestors.Pid":
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
	case "process.parent.ContainerID":
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
	case "process.parent.Pid":
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
	}
	return nil, &eval.ErrFieldNotFound{Field: field}
}
func (ev *Event) GetFields() []eval.Field {
	return []eval.Field{
		"container.created_at",
		"container.id",
		"container.tags",
		"event.timestamp",
		"network.destination.ip",
		"network.destination.port",
		"network.l3_protocol",
		"network.l4_protocol",
		"network.size",
		"network.source.ip",
		"network.source.port",
		"process.ContainerID",
		"process.Pid",
		"process.ancestors.ContainerID",
		"process.ancestors.Pid",
		"process.parent.ContainerID",
		"process.parent.Pid",
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
	case "event.timestamp":
		return int(ev.FieldHandlers.ResolveEventTimestamp(ev, &ev.BaseEvent)), nil
	case "network.destination.ip":
		return ev.BaseEvent.NetworkContext.Destination.IPNet, nil
	case "network.destination.port":
		return int(ev.BaseEvent.NetworkContext.Destination.Port), nil
	case "network.l3_protocol":
		return int(ev.BaseEvent.NetworkContext.L3Protocol), nil
	case "network.l4_protocol":
		return int(ev.BaseEvent.NetworkContext.L4Protocol), nil
	case "network.size":
		return int(ev.BaseEvent.NetworkContext.Size), nil
	case "network.source.ip":
		return ev.BaseEvent.NetworkContext.Source.IPNet, nil
	case "network.source.port":
		return int(ev.BaseEvent.NetworkContext.Source.Port), nil
	case "process.ContainerID":
		return ev.BaseEvent.ProcessContext.Process.ContainerID, nil
	case "process.Pid":
		return int(ev.BaseEvent.ProcessContext.Process.PIDContext.Pid), nil
	case "process.ancestors.ContainerID":
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
	case "process.ancestors.Pid":
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
	case "process.parent.ContainerID":
		return ev.BaseEvent.ProcessContext.Parent.ContainerID, nil
	case "process.parent.Pid":
		return int(ev.BaseEvent.ProcessContext.Parent.PIDContext.Pid), nil
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
	case "event.timestamp":
		return "*", nil
	case "network.destination.ip":
		return "*", nil
	case "network.destination.port":
		return "*", nil
	case "network.l3_protocol":
		return "*", nil
	case "network.l4_protocol":
		return "*", nil
	case "network.size":
		return "*", nil
	case "network.source.ip":
		return "*", nil
	case "network.source.port":
		return "*", nil
	case "process.ContainerID":
		return "*", nil
	case "process.Pid":
		return "*", nil
	case "process.ancestors.ContainerID":
		return "*", nil
	case "process.ancestors.Pid":
		return "*", nil
	case "process.parent.ContainerID":
		return "*", nil
	case "process.parent.Pid":
		return "*", nil
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
	case "event.timestamp":
		return reflect.Int, nil
	case "network.destination.ip":
		return reflect.Struct, nil
	case "network.destination.port":
		return reflect.Int, nil
	case "network.l3_protocol":
		return reflect.Int, nil
	case "network.l4_protocol":
		return reflect.Int, nil
	case "network.size":
		return reflect.Int, nil
	case "network.source.ip":
		return reflect.Struct, nil
	case "network.source.port":
		return reflect.Int, nil
	case "process.ContainerID":
		return reflect.String, nil
	case "process.Pid":
		return reflect.Int, nil
	case "process.ancestors.ContainerID":
		return reflect.String, nil
	case "process.ancestors.Pid":
		return reflect.Int, nil
	case "process.parent.ContainerID":
		return reflect.String, nil
	case "process.parent.Pid":
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
	case "event.timestamp":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.TimestampRaw"}
		}
		ev.BaseEvent.TimestampRaw = uint64(rv)
		return nil
	case "network.destination.ip":
		rv, ok := value.(net.IPNet)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.Destination.IPNet"}
		}
		ev.BaseEvent.NetworkContext.Destination.IPNet = rv
		return nil
	case "network.destination.port":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.Destination.Port"}
		}
		ev.BaseEvent.NetworkContext.Destination.Port = uint16(rv)
		return nil
	case "network.l3_protocol":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.L3Protocol"}
		}
		ev.BaseEvent.NetworkContext.L3Protocol = uint16(rv)
		return nil
	case "network.l4_protocol":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.L4Protocol"}
		}
		ev.BaseEvent.NetworkContext.L4Protocol = uint16(rv)
		return nil
	case "network.size":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.Size"}
		}
		ev.BaseEvent.NetworkContext.Size = uint32(rv)
		return nil
	case "network.source.ip":
		rv, ok := value.(net.IPNet)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.Source.IPNet"}
		}
		ev.BaseEvent.NetworkContext.Source.IPNet = rv
		return nil
	case "network.source.port":
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.NetworkContext.Source.Port"}
		}
		ev.BaseEvent.NetworkContext.Source.Port = uint16(rv)
		return nil
	case "process.ContainerID":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(string)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.ContainerID"}
		}
		ev.BaseEvent.ProcessContext.Process.ContainerID = rv
		return nil
	case "process.Pid":
		if ev.BaseEvent.ProcessContext == nil {
			ev.BaseEvent.ProcessContext = &ProcessContext{}
		}
		rv, ok := value.(int)
		if !ok {
			return &eval.ErrValueTypeMismatch{Field: "BaseEvent.ProcessContext.Process.PIDContext.Pid"}
		}
		ev.BaseEvent.ProcessContext.Process.PIDContext.Pid = uint32(rv)
		return nil
	case "process.ancestors.ContainerID":
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
	case "process.ancestors.Pid":
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
	case "process.parent.ContainerID":
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
	case "process.parent.Pid":
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
	}
	return &eval.ErrFieldNotFound{Field: field}
}
