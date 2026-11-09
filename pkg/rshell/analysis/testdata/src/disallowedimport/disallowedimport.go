// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package disallowedimport imports a package not in the allowlist.
package disallowedimport

import (
	"bufio" // want `import of "bufio" is not in the allowlist`
	"fmt"
)

// Read uses an import that is not allowlisted.
func Read() {
	fmt.Println("read")
	_ = bufio.NewScanner(nil)
}
