// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !process

package processflare

import "errors"

// GetSystemProbeTelemetry is not supported without the process agent
func GetSystemProbeTelemetry(_socketPath string) ([]byte, error) {
	return nil, errors.New("getSystemProbeTelemetry not supported on build without the process agent")
}
