// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package windows

import (
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// GetTemporaryFile returns a new temporary file path
// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.utility/new-temporaryfile?view=powershell-7.4
func GetTemporaryFile(host *components.RemoteHost) (string, error) {
	cmd := "(New-TemporaryFile).FullName"
	out, err := host.Execute(cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
