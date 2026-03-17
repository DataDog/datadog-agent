// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux && !windows

package dockerpermissions

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// Check is a noop on unsupported platforms
func Check() (*healthplatform.IssueReport, error) {
	return nil, nil
}
