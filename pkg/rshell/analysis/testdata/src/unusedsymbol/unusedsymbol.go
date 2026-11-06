// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package unusedsymbol has an allowlisted symbol that is never used.
package unusedsymbol // want `allowlist symbol "fmt.Sprintf" is not used`

import "fmt"

// Hello uses only fmt.Println; fmt.Sprintf is in the allowlist but unused.
func Hello() {
	fmt.Println("hello")
}
