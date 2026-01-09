// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// main is a dummy program used for testing Go language detection.
package main

import (
	"bufio"
	"fmt"
	"os"
)

func main() {
	// Signal that we're ready
	fmt.Fprintln(os.Stdout, "READY")

	// Wait for stdin input before exiting
	reader := bufio.NewReader(os.Stdin)
	_, _ = reader.ReadByte()
}
