// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package bannedimport imports a permanently banned package.
package bannedimport

import (
	"fmt"
	"os/exec" // want `import of "os/exec" is permanently banned`
)

// Run uses a banned import.
func Run() {
	fmt.Println("run")
	_ = exec.Command("ls")
}
