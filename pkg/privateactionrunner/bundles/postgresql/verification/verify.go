// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pg_verification

import (
	"fmt"
	"strings"
)

func VerifyForbiddenPgExpressions(query string) error {
	expressions := make([]string, 0, len(InfoFunctions)+len(AdminFunctions)+len(Tables))
	expressions = append(expressions, InfoFunctions...)
	expressions = append(expressions, AdminFunctions...)
	expressions = append(expressions, Tables...)

	lowerQuery := strings.ToLower(query)
	for _, expression := range expressions {
		if strings.Contains(lowerQuery, expression) {
			return fmt.Errorf("usage of %s in a query is forbidden", expression)
		}
	}
	return nil
}
