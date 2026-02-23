// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package actions

import (
	"strings"
)

func SplitFQN(fqn string) (string, string) {
	index := strings.LastIndex(fqn, ".")
	if index == -1 {
		return "", ""
	}
	return fqn[:index], fqn[index+1:]
}
func IsHttpBundle(bundleId string) bool {
	return bundleId == "com.datadoghq.http"
}
