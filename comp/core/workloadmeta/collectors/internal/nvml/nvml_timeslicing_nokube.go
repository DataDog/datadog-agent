// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml && !kubelet

package nvml

import "context"

// fetchNodeLabels reads no labels without kubelet access.
func fetchNodeLabels(_ context.Context) (map[string]string, error) {
	return nil, nil
}
