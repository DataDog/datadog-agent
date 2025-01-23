// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package tests holds tests related files
package tests

import "errors"

// getPIDCGroup returns the path of the first cgroup found for a PID
func getPIDCGroup(pid uint32) (string, error) {
	return "", errors.New("cgroups are not supported on Windows")
}

func preTestsHook() {}

func postTestsHook() {}
