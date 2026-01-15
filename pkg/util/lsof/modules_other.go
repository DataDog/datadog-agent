// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package lsof

// ListLoadedModulesReportJSON is only meaningful on Windows; on other platforms it returns nil content.
func ListLoadedModulesReportJSON() ([]byte, error) {
	return nil, nil
}
