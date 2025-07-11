// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

const (
	// ScopeProcess is the scope for process variables
	ScopeProcess = "process"
	// ScopeContainer is the scope for container variables
	ScopeContainer = "container"
)

// IsScopeVariable returns true if the variable name is a scope variable
func IsScopeVariable(varName string) bool {
	for _, scope := range VariableScopes {
		if strings.HasPrefix(varName, scope+".") {
			return true
		}
	}
	return false
}

func getCommonStateScopes() map[Scope]VariableProviderFactory {
	return map[Scope]VariableProviderFactory{
		ScopeProcess: func() VariableProvider {
			return eval.NewScopedVariables(ScopeProcess, func(ctx *eval.Context) eval.VariableScope {
				scopeEvaluator := ctx.GetScopeFieldEvaluator()
				if scopeEvaluator != nil {
					pid, ok := scopeEvaluator.Eval(ctx).(int)
					if !ok {
						return nil
					}
					if pce := ctx.Event.(*model.Event).FieldHandlers.ResolveProcessCacheEntryFromPID(uint32(pid)); pce != nil {
						return pce
					}
				} else {
					if pce := ctx.Event.(*model.Event).ProcessCacheEntry; pce != nil {
						return pce
					}
				}
				return nil
			})
		},
		ScopeContainer: func() VariableProvider {
			return eval.NewScopedVariables(ScopeContainer, func(ctx *eval.Context) eval.VariableScope {
				if cc := ctx.Event.(*model.Event).ContainerContext; cc != nil {
					return cc
				}
				return nil
			})
		},
	}
}
