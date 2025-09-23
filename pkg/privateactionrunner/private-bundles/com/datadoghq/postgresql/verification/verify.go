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
