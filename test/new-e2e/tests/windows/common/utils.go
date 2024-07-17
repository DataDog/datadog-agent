// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package common

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"

	"golang.org/x/text/encoding/unicode"
)

// ConvertUTF16ToUTF8 converts a byte slice from UTF-16 to UTF-8
//
// UTF-16 little-endian (UTF-16LE) is the encoding standard in the Windows operating system.
// https://learn.microsoft.com/en-us/globalization/encoding/transformations-of-unicode-code-points
func ConvertUTF16ToUTF8(content []byte) ([]byte, error) {
	utf16 := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	utf8, err := utf16.NewDecoder().Bytes(content)
	if err != nil {
		return nil, fmt.Errorf("failed to convert UTF-16 to UTF-8: %v", err)
	}
	return utf8, nil
}

// TrimTrailingSlashesAndLower trims trailing slashes and lowercases the path for use in simple comparisons.
//
// Some cases may require a more comprehensive comparison, which could be made by normalizing the path on the host
// via PowerShell, to support removing dot paths, resolving links, etc
func TrimTrailingSlashesAndLower(path string) string {
	// Normalize paths
	// trim trailing slashes
	path = strings.TrimSuffix(path, `\`)
	// windows paths are case-insensitive
	path = strings.ToLower(path)
	return path
}

// MeasureCommand uses Measure-Command and returns time taken (in milliseconds), out, err
func MeasureCommand(host *components.RemoteHost, command string) (time.Duration, string, error) {
	// *>&1 redirects all streams
	// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_redirection
	powershellCommand := fmt.Sprintf(`
		$ErrorActionPreference = "Stop"
		$taken = Measure-Command { $cmdout = $( %s ) *>&1 | Out-String }
		@{
			TotalMilliseconds=$taken.TotalMilliseconds
			Output=$cmdout
		} | ConvertTo-JSON`, command)
	out, err := host.Execute(powershellCommand)
	if err != nil {
		return 0, out, err
	}
	type measureCommandOutput struct {
		TotalMilliseconds float64 `json:"TotalMilliseconds"`
		Output            string  `json:"Output"`
	}
	var m measureCommandOutput
	err = json.Unmarshal([]byte(out), &m)
	if err != nil {
		return 0, "", fmt.Errorf("failed to unmarshal Measure-Command output: %w\n%s", err, out)
	}
	return time.Duration(m.TotalMilliseconds) * time.Millisecond, m.Output, nil
}
