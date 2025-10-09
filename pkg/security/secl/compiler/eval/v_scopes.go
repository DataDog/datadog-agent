// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

type VariableScope interface {
	Key() (string, bool)
	ParentScope() (VariableScope, bool)
}

type ReleasableVariableScope interface {
	AppendReleaseCallback(callback func())
}

type ScoperFnc func(ctx *Context) (VariableScope, error)

type VariableScoper struct {
	scoperType InternalScoperType
	getScopeCb ScoperFnc
}

func NewVariableScoper(scoperType InternalScoperType, cb ScoperFnc) *VariableScoper {
	return &VariableScoper{
		scoperType: scoperType,
		getScopeCb: cb,
	}
}

func (vs *VariableScoper) Type() InternalScoperType {
	return vs.scoperType
}

func (vs *VariableScoper) GetScope(ctx *Context) (VariableScope, error) {
	return vs.getScopeCb(ctx)
}

type InternalScoperType int

const (
	UndefinedScoperType InternalScoperType = iota
	GlobalScoperType
	ProcessScoperType
	ContainerScoperType
	CGroupScoperType
)

func (isn InternalScoperType) String() string {
	switch isn {
	case GlobalScoperType:
		return "global"
	case ProcessScoperType:
		return "process"
	case ContainerScoperType:
		return "container"
	case CGroupScoperType:
		return "cgroup"
	default:
		return ""
	}
}

func (isn InternalScoperType) VariablePrefix() string {
	switch isn {
	case ProcessScoperType:
		return "process"
	case ContainerScoperType:
		return "container"
	case CGroupScoperType:
		return "cgroup"
	default:
		return ""
	}
}

// func ProcessScoper(ctx *Context) VariableScope {
// 	scopeEvaluator := ctx.GetScopeFieldEvaluator()
// 	if scopeEvaluator != nil {
// 		pid, ok := scopeEvaluator.Eval(ctx).(int)
// 		if !ok {
// 			return nil
// 		}
// 		if pce := ctx.Event.(*model.Event).FieldHandlers.ResolveProcessCacheEntryFromPID(uint32(pid)); pce != nil {
// 			return pce
// 		}
// 	} else if pce := ctx.Event.(*model.Event).ProcessCacheEntry; pce != nil {
// 		return pce
// 	}

// 	return nil
// }

// type Scoper interface {
// 	Name() string
// 	Hash(OpaqueContextForNow) (string, bool)
// 	Parent() Scoper
// }

// type globalScope struct{}

// func (g *globalScope) Name() string {
// 	return "global"
// }

// func (g *globalScope) Hash(_ OpaqueContextForNow) (string, bool) {
// 	return "", true
// }

// func (g *globalScope) Parent() Scoper {
// 	return nil
// }

// var scopes map[string]Scoper = map[string]Scoper{
// 	"": &globalScope{},
// }
