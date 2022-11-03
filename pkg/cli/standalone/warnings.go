// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package standalone

import "fmt"

// PrintWindowsUserWarning prints a warning for standalone CLI commands on Windows.
func PrintWindowsUserWarning(op string) {
	fmt.Printf("\nNOTE:\n")
	fmt.Printf("The %s command runs in a different user context than the running service\n", op)
	fmt.Printf("This could affect results if the command relies on specific permissions and/or user contexts\n")
	fmt.Printf("\n")
}
