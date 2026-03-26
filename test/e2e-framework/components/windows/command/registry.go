// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package command

import (
	"fmt"
)

// GetRegistryValue returns a command string to get a registry value
func GetRegistryValue(path string, value string) string {
	return fmt.Sprintf("Get-ItemPropertyValue -Path '%s' -Name '%s'", path, value)
}

// RegistryKeyExists returns a command to check if a registry path exists
func RegistryKeyExists(path string) string {
	return fmt.Sprintf("Test-Path -Path '%s'", path)
}
