// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests

// Package eval holds eval related files
package eval

import "errors"

// AppendResolvedField instructs the context that this field has been resolved
func (c *Context) AppendResolvedField(field string) {
	if field == "" {
		return
	}

	c.resolvedFields = append(c.resolvedFields, field)
}

// DecorateRuleExpr decorate the rule
func (m *MatchingSubExpr) DecorateRuleExpr(expr string, before, after string) (string, error) {
	a, b := m.ValueA.getPosWithinRuleExpr(expr, m.Offset), m.ValueB.getPosWithinRuleExpr(expr, m.Offset)

	if a.Offset+a.Length > len(expr) || b.Offset+b.Length > len(expr) {
		return expr, errors.New("expression overflow")
	}

	if b.Offset < a.Offset {
		tmp := b
		b = a
		a = tmp
	}

	if a.Length == 0 {
		return expr[:b.Offset] + before + expr[b.Offset:b.Offset+b.Length] + after + expr[b.Offset+b.Length:], nil
	}

	if b.Length == 0 {
		return expr[0:a.Offset] + before + expr[a.Offset:a.Offset+a.Length] + after + expr[a.Offset+a.Length:], nil
	}

	return expr[0:a.Offset] + before + expr[a.Offset:a.Offset+a.Length] + after +
		expr[a.Offset+a.Length:b.Offset] + before + expr[b.Offset:b.Offset+b.Length] + after +
		expr[b.Offset+b.Length:], nil
}

// DecorateRuleExpr decorate the rule
func (m *MatchingSubExprs) DecorateRuleExpr(expr string, before, after string) (string, error) {
	var err error
	for _, mse := range *m {
		expr, err = mse.DecorateRuleExpr(expr, before, after)
		if err != nil {
			return expr, err
		}
	}
	return expr, nil
}
