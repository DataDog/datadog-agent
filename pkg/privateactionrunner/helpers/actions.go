package helpers

import "fmt"

// QualifyName returns the fully-qualified name for an action. For
// example, "com.datadoghq.core" and "if", becomes "com.datadoghq.core.if".
func QualifyName(bundleName, actionName string) string {
	return fmt.Sprintf("%s.%s", bundleName, actionName)
}
