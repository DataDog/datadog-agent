// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

var (
	// SECLVariables set of variables
	SECLVariables = map[string]eval.VariableValue{
		"process.pid": {
			IntFnc: func(ctx *eval.Context) int {
				return int((*Event)(ctx.Object).ProcessContext.Process.Pid)
			},
		},
	}
)
