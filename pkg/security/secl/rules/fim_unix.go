// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package rules holds rules related files
package rules

import (
	"fmt"
	"strings"
)

type expandedRule struct {
	id   string
	expr string
}

func expandFim(baseID, groupID, baseExpr string) []expandedRule {
	if !strings.Contains(baseExpr, "fim.write.file.") {
		return []expandedRule{
			{
				id:   baseID,
				expr: baseExpr,
			},
		}
	}

	var expandedRules []expandedRule
	for _, eventType := range []string{"open", "chmod", "chown", "link", "rename", "unlink", "utimes"} {
		expr := strings.Replace(baseExpr, "fim.write.file.", fmt.Sprintf("%s.file.", eventType), -1)
		if eventType == "open" {
			expr = fmt.Sprintf("(%s) && open.flags & (O_CREAT|O_TRUNC|O_APPEND|O_RDWR|O_WRONLY) > 0", expr)
		}

		id := fmt.Sprintf("__fim_expanded_%s_%s_%s", eventType, groupID, baseID)
		expandedRules = append(expandedRules, expandedRule{
			id:   id,
			expr: expr,
		})

		if eventType == "rename" {
			expr := strings.Replace(baseExpr, "fim.write.file.", "rename.file.destination.", -1)
			id := fmt.Sprintf("__fim_expanded_%s_%s_%s", "rename_destination", groupID, baseID)
			expandedRules = append(expandedRules, expandedRule{
				id:   id,
				expr: expr,
			})
		}
	}

	return expandedRules
}
