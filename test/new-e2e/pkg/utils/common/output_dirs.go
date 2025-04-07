// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"strings"
)

// SanitizeDirectoryName replace invalid characters in a directory name underscores.
//
// Example:
//   - name: TestInstallSuite/TestInstall/install_version=7.50.0
//   - output directory: <root>/TestInstallSuite/TestInstall/install_version_7_50_0
func SanitizeDirectoryName(name string) string {
	// https://en.wikipedia.org/wiki/Filename#Reserved_characters_and_words
	invalidPathChars := strings.Join([]string{"?", "%", "*", ":", "|", "\"", "<", ">", ".", ",", ";", "="}, "")
	return strings.ReplaceAll(name, invalidPathChars, "_")
}
