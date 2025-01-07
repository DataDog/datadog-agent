// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package installinfo

// WriteInstallInfo write install info and signature files
func WriteInstallInfo(_, _, _ string) error {
	// Placeholder for Windows, this is done in tools/windows/DatadogAgentInstaller
	return nil
}
