// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !docker

// Package rofspermissions provides a complete module for handling Read-Only Filesystem permission issues specifically
// checking if the Agent has write permissions to all the expected directories.
package rofspermissions

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Check is a noop on unsupported platforms.
func Check(_ config.Component) ([]runnerdef.IssueReport, error) {
	return nil, nil
}
