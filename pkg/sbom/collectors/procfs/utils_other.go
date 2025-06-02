// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package procfs

import "errors"

// IsAgentContainer returns whether the container ID is the agent one
func IsAgentContainer(_ string) (bool, error) {
	return false, errors.New("not supported")
}
