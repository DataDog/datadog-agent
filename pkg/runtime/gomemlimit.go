// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux || !go1.19

package runtime

import (
	"errors"
)

// SetGoMemLimit configures Go memory limit based on cgroups. Only supported on Linux.
func SetGoMemLimit(_ bool) (int64, error) {
	return 0, errors.New("unsupported")
}
