// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package custom

import (
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
)

// CheckFunc defines the custom check function interface
type CheckFunc func(e env.Env, ruleID string, vars map[string]string, expr *eval.IterableExpression) (*compliance.Report, error)

// GetCustomCheck returns a CheckFunc based on custom check registered name
func GetCustomCheck(name string) CheckFunc {
	f, found := customCheckRegistry[name]
	if found {
		return f
	}

	return nil
}

var customCheckRegistry = make(map[string]CheckFunc)

func registerCustomCheck(name string, f CheckFunc) {
	customCheckRegistry[name] = f
}
