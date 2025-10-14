// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ScopeGlobal is the global scope
	ScopeGlobal = ""
	// ScopeProcess is the scope for process variables
	ScopeProcess = "process"
	// ScopeContainer is the scope for container variables
	ScopeContainer = "container"
	// ScopeCGroup is the scope for cgroup variables
	ScopeCGroup = "cgroup"
)

type globalScope struct{}

// GlobalScopeKey is the constant scope key used by the global scope
const GlobalScopeKey = ""

// Key always returns the same unique key of the global scope
func (gs *globalScope) Key() (string, bool) {
	return GlobalScopeKey, true
}

// ParentScope returns the parent entity scope
func (gs *globalScope) ParentScope() (eval.VariableScope, bool) {
	return nil, false
}

func getCommonVariableScopers() map[Scope]*eval.VariableScoper {
	return map[Scope]*eval.VariableScoper{
		ScopeGlobal: eval.NewVariableScoper(eval.GlobalScoperType, func(_ *eval.Context) (eval.VariableScope, error) {
			return &globalScope{}, nil
		}),
		ScopeProcess: eval.NewVariableScoper(eval.ProcessScoperType, func(ctx *eval.Context) (eval.VariableScope, error) {
			scopeEvaluator := ctx.GetScopeFieldEvaluator()
			if scopeEvaluator != nil {
				pid, ok := scopeEvaluator.Eval(ctx).(int)
				if !ok {
					return nil, fmt.Errorf("failed to evaluate scope field value")
				}
				if pce := ctx.Event.(*model.Event).FieldHandlers.ResolveProcessCacheEntryFromPID(uint32(pid)); pce != nil {
					return pce, nil
				}
			} else {
				if pce := ctx.Event.(*model.Event).ProcessCacheEntry; pce != nil {
					return pce, nil
				}
			}
			return nil, fmt.Errorf("failed to get process scope")
		}),
		ScopeContainer: eval.NewVariableScoper(eval.ContainerScoperType, func(ctx *eval.Context) (eval.VariableScope, error) {
			if cc := ctx.Event.(*model.Event).ContainerContext; cc != nil {
				return cc, nil
			}
			return nil, fmt.Errorf("failed to get container scope")
		}),
	}
}
