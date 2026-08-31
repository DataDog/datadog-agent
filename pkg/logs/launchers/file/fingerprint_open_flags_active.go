// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package file

import (
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/file"
)

// directOpenFlagsActiveForFile reports whether configured open_flags should be
// applied when fingerprinting file on this platform.
func directOpenFlagsActiveForFile(fingerprinter tailer.Fingerprinter, file *tailer.File) bool {
	return tailer.FingerprintOpenFlagsActive(fingerprinter.GetEffectiveConfigForFile(file))
}
