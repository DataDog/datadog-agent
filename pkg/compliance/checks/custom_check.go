// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"errors"
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

type checkFactoryFunc func(name string) custom.CheckFunc

var customCheckFactory = custom.GetCustomCheck

func newCustomCheck(ruleID string, res compliance.Resource) (checkable, error) {
	if res.Custom == nil {
		return nil, errors.New("expecting custom resource in custom check")
	}

	if res.Custom.Name == "" {
		return nil, errors.New("missing check name in custom check")
	}

	var (
		expr *eval.IterableExpression
		err  error
	)

	if res.Condition != "" {
		expr, err = eval.Cache.ParseIterable(res.Condition)
		if err != nil {
			return nil, err
		}
	}

	checkFunc := customCheckFactory(res.Custom.Name)
	if checkFunc == nil {
		return nil, fmt.Errorf("custom check with name: %s does not exist", res.Custom.Name)
	}

	return &customCheck{
		ruleID:    ruleID,
		custom:    res.Custom,
		expr:      expr,
		checkFunc: checkFunc,
	}, nil
}

func (c *customCheck) check(e env.Env) []*compliance.Report {
	report, err := c.checkFunc(e, c.ruleID, c.custom.Variables, c.expr)
	if err != nil {
		report = compliance.BuildReportForError(err)
	}
	return []*compliance.Report{report}
}
