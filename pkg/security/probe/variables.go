// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

var (
	// SECLVariables set of variables
	SECLVariables = map[string]eval.VariableValue{
		"process.pid": eval.NewIntVariable(func(ctx *eval.Context) int {
			pc := ctx.Event.(*model.Event).ProcessContext
			if pc == nil {
				return 0
			}
			return int(pc.Process.Pid)
		}, nil),
	}
)
