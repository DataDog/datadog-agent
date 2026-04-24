// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows

package windowseventlogchannels

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
)

// Check is a noop on non-Windows platforms
func Check(_ config.Component) (*healthplatform.IssueReport, error) {
	return nil, nil
}
