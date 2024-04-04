// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// GetRegistryValue returns a registry value from a remote host
func GetRegistryValue(host *components.RemoteHost, path string, value string) (string, error) {
	cmd := fmt.Sprintf("Get-ItemPropertyValue -Path '%s' -Name '%s'", path, value)
	out, err := host.Execute(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
