// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package runtime

import (
	"errors"
)

// SetGoMemLimit configures Go memory limit based on cgroups. Only supported on Linux.
func SetGoMemLimit(isContainerized bool) error {
	return errors.New("unsupported on non-linux")
}
