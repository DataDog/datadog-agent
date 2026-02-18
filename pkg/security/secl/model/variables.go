// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/google/uuid"
)

var (
	// SECLVariables set of variables
	SECLVariables = map[string]eval.SECLVariable{
		"process.pid": eval.NewScopedIntVariable(func(ctx *eval.Context, _ bool) (int, bool) {
			pc := ctx.Event.(*Event).ProcessContext
			if pc == nil {
				return 0, false
			}
			return int(pc.Process.Pid), true
		}, nil),
		"builtins.uuid4": eval.NewScopedStringVariable(func(_ *eval.Context, _ bool) (string, bool) {
			return uuid.New().String(), true
		}, nil),
	}
)
