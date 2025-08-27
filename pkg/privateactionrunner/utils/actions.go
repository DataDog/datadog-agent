// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

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
