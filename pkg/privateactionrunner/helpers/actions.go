package helpers

import (
	"fmt"
	"strings"
)

// QualifyName returns the fully-qualified name for an action. For
// example, "com.datadoghq.core" and "if", becomes "com.datadoghq.core.if".
func QualifyName(bundleName, actionName string) string {
	return fmt.Sprintf("%s.%s", bundleName, actionName)
}

// SplitFQN returns the bundle ID and unqualified action name from a
// fully-qualified name. For example, "com.datadoghq.core.if", becomes
// ("com.datadoghq.core", "if").
func SplitFQN(fqn string) (string, string) {
	index := strings.LastIndex(fqn, ".")
	if index == -1 {
		return "", ""
	}
	return fqn[:index], fqn[index+1:]
}
