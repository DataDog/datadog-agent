// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !linux

package systemprobeunreachable

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
)

// Check is a no-op on non-Linux platforms where system-probe is not supported.
func Check(_ config.Component) (*healthplatform.IssueReport, error) {
	return nil, nil
}
