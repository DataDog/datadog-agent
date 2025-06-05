// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func getCommonStateScopes() map[Scope]VariableProviderFactory {
	return map[Scope]VariableProviderFactory{
		"process": func() VariableProvider {
			return eval.NewScopedVariables(func(ctx *eval.Context, scopeFieldEvaluator eval.Evaluator) eval.VariableScope {
				if scopeFieldEvaluator != nil {
					pid, ok := scopeFieldEvaluator.Eval(ctx).(int)
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
		"container": func() VariableProvider {
			return eval.NewScopedVariables(func(ctx *eval.Context, _ eval.Evaluator) eval.VariableScope {
				if cc := ctx.Event.(*model.Event).ContainerContext; cc != nil {
					return cc
				}
				return nil
			})
		},
	}
}
