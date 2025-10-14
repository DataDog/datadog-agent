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

// DefaultVariableScopers returns the default variable scopers
func DefaultVariableScopers() map[Scope]*eval.VariableScoper {
	variableScopers := getCommonVariableScopers()
	variableScopers[ScopeCGroup] = eval.NewVariableScoper(eval.CGroupScoperType, func(ctx *eval.Context) (eval.VariableScope, error) {
		if ctx.Event.(*model.Event).CGroupContext == nil || ctx.Event.(*model.Event).CGroupContext.CGroupFile.IsNull() {
			return nil, fmt.Errorf("failed to get cgroup scope")
		}
		return ctx.Event.(*model.Event).CGroupContext, nil
	})
	return variableScopers
}
