// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package verification

import (
	"fmt"
	"strings"
)

func VerifyForbiddenPgExpressions(query string) error {
	expressions := append(InfoFunctions, AdminFunctions...)
	expressions = append(expressions, Tables...)

	for _, expression := range expressions {
		err := fmt.Errorf("Usage of %s in a query is forbidden", expression)
		if strings.Contains(strings.ToLower(query), expression) {
			return err
		}
	}
	return nil
}
