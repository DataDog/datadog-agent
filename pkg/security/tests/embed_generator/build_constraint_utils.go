// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"go/build/constraint"
	"strings"
)

func convertBuildTags(content string) (string, error) {
	var res strings.Builder
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		expr, err := constraint.Parse(line)
		if err != nil {
			res.WriteString(line + "\n")
		} else {
			newBuildExpr := convertConstraintExprFinal(expr)
			newBuildLines, err := constraint.PlusBuildLines(newBuildExpr)
			if err != nil {
				return "", err
			}
			for _, bl := range newBuildLines {
				res.WriteString(bl + "\n")
			}
		}
	}
	return res.String(), nil
}

func convertConstraintExprFinal(expr constraint.Expr) constraint.Expr {
	lhs := convertConstraintExpr(expr)
	rhs := &constraint.AndExpr{
		X: &constraint.NotExpr{X: &constraint.TagExpr{Tag: "functionaltests"}},
		Y: &constraint.NotExpr{X: &constraint.TagExpr{Tag: "stresstests"}},
	}

	var res constraint.Expr
	if lhs == nil {
		res = rhs
	} else {
		res = &constraint.AndExpr{
			X: lhs,
			Y: rhs,
		}
	}
	return simplifyBuildTree(res)
}

func convertConstraintExpr(expr constraint.Expr) constraint.Expr {
	switch e := expr.(type) {
	case *constraint.AndExpr:
		X := convertConstraintExpr(e.X)
		Y := convertConstraintExpr(e.Y)
		if X == nil {
			return Y
		} else if Y == nil {
			return X
		}
		return &constraint.AndExpr{X: X, Y: Y}
	case *constraint.OrExpr:
		X := convertConstraintExpr(e.X)
		Y := convertConstraintExpr(e.Y)
		if X == nil {
			return Y
		} else if Y == nil {
			return X
		}
		return &constraint.OrExpr{X: X, Y: Y}
	case *constraint.NotExpr:
		X := convertConstraintExpr(e.X)
		if X == nil {
			return nil
		}
		return &constraint.NotExpr{X: X}
	case *constraint.TagExpr:
		if e.Tag == "functionaltests" || e.Tag == "stresstests" {
			return nil
		} else {
			return e
		}
	default:
		panic("Unsupported build constraint node")
	}
}

func simplifyBuildTree(expr constraint.Expr) constraint.Expr {
	switch e := expr.(type) {
	case *constraint.AndExpr:
		X := simplifyBuildTree(e.X)
		Y := simplifyBuildTree(e.Y)
		if X.String() == Y.String() {
			return X
		}
		return &constraint.AndExpr{X: X, Y: Y}
	case *constraint.OrExpr:
		X := simplifyBuildTree(e.X)
		Y := simplifyBuildTree(e.Y)
		if X.String() == Y.String() {
			return X
		}
		return &constraint.OrExpr{X: X, Y: Y}
	default:
		return expr
	}
}
