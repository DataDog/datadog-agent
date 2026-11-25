// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package lsof

import "context"

// ListLoadedModulesReportJSON is only meaningful on Windows; on other platforms it returns nil content.
func ListLoadedModulesReportJSON(_ context.Context) ([]byte, error) {
	return nil, nil
}
