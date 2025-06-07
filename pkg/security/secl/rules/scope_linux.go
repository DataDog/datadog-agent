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

func getStateScopes() map[Scope]VariableProviderFactory {
	stateScopes := getCommonStateScopes()
	stateScopes["cgroup"] = func() VariableProvider {
		return eval.NewScopedVariables(func(ctx *eval.Context) eval.VariableScope {
			if ctx.Event.(*model.Event).CGroupContext == nil || ctx.Event.(*model.Event).CGroupContext.CGroupFile.IsNull() {
				return nil
			}
			return ctx.Event.(*model.Event).CGroupContext
		})
	}
	return stateScopes
}
