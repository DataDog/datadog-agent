// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !process

// Package sysprobe contains flare logic that only imports pkg/process/net when the process build tag is included
package sysprobe

import "errors"

// GetSystemProbeTelemetry is not supported without the process agent
func GetSystemProbeTelemetry(_socketPath string) ([]byte, error) {
	return nil, errors.New("GetSystemProbeTelemetry not supported on builds without the process agent")
}
