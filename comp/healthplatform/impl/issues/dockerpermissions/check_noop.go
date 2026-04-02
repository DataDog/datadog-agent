// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux && !windows

package dockerpermissions

import (
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// Check is a noop on unsupported platforms
func Check() (*healthplatformdef.IssueReport, error) {
	return nil, nil
}
