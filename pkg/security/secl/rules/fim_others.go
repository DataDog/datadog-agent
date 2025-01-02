// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !unix

// Package rules holds rules related files
package rules

type expandedRule struct {
	id   string
	expr string
}

func expandFim(baseID, baseExpr string) []expandedRule {
	return []expandedRule{
		{
			id:   baseID,
			expr: baseExpr,
		},
	}
}
