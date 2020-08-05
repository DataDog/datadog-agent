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

type customCheck struct {
	ruleID    string
	custom    *compliance.Custom
	expr      *eval.IterableExpression
	checkFunc custom.CheckFunc
}

func newCustomCheck(ruleID string, res compliance.Resource) (checkable, error) {
	if res.Custom == nil {
		return nil, fmt.Errorf("expecting custom resource in custom check")
	}

	if res.Custom.Name == "" {
		return nil, fmt.Errorf("expecting custom resource name in custom check")
	}

	expr, err := eval.Cache.ParseIterable(res.Condition)
	if err != nil {
		return nil, err
	}

	f := custom.GetCustomCheck(res.Custom.Name)
	if f != nil {
		return nil, fmt.Errorf("custom check with name: %s does not exist", res.Custom.Name)
	}

	return &customCheck{
		ruleID:    ruleID,
		custom:    res.Custom,
		expr:      expr,
		checkFunc: f,
	}, nil
}

func (c *customCheck) check(e env.Env) (*compliance.Report, error) {
	return c.checkFunc(e, c.ruleID, c.custom.Variables, c.expr)
}
