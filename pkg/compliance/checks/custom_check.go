// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/custom"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
)

func checkCustom(e env.Env, ruleID string, res compliance.Resource, expr *eval.IterableExpression) (*compliance.Report, error) {
	if res.Custom == nil || res.Custom.Name == "" {
		return nil, fmt.Errorf("expecting custom resource in custom check")
	}

	f := custom.GetCustomCheck(res.Custom.Name)
	if f != nil {
		return nil, fmt.Errorf("custom check with name: %s does not exist", res.Custom.Name)
	}

	return f(e, ruleID, res.Custom.Variables, expr)
}
