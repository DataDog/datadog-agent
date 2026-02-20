// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_script

import (
	"fmt"
	"strings"
)

// shellQuote returns s wrapped in POSIX single quotes with embedded
// single quotes properly escaped.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellQuoteArgs joins args into a single shell-safe command string.
func shellQuoteArgs(args []string) (string, error) {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsRune(a, '\x00') {
			return "", fmt.Errorf("argument %d contains null byte", i)
		}
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " "), nil
}
