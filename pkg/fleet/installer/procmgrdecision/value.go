// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package procmgrdecision

import (
	"fmt"
	"strings"
)

// ParseStrict parses a DD_PROCESS_MANAGER_ENABLED value for entry points that can
// reject it (install scripts, MSI). Surrounding spaces are ignored. An empty value
// is not set. Only true and false (case-insensitive) are accepted.
func ParseStrict(raw string) (enabled bool, set bool, err error) {
	v := strings.TrimSpace(raw)
	switch {
	case v == "":
		return false, false, nil
	case strings.EqualFold(v, "true"):
		return true, true, nil
	case strings.EqualFold(v, "false"):
		return false, true, nil
	default:
		return false, false, fmt.Errorf("invalid value %q for DD_PROCESS_MANAGER_ENABLED: the accepted values are true and false", v)
	}
}

// ParseLenient parses a DD_PROCESS_MANAGER_ENABLED value for entry points that
// cannot reject it (deb, rpm, hook reruns, CLI). It mirrors how the installer reads
// the variable: an empty value is not set, and anything other than false
// (case-insensitive) is true. A value that is neither true nor false also gets a
// warning, so the caller can log it.
func ParseLenient(raw string) (enabled bool, set bool, warning string) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return false, false, ""
	}
	enabled = !strings.EqualFold(v, "false")
	if !strings.EqualFold(v, "true") && enabled {
		warning = fmt.Sprintf("invalid value %q for DD_PROCESS_MANAGER_ENABLED: treating it as true", v)
	}
	return enabled, true, warning
}
