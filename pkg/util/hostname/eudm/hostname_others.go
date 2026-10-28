// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin && !windows

package eudm

import (
	"errors"
	"fmt"
)

// Get always fails: EUDM device identity is only available on macOS and Windows.
func Get() (string, error) {
	return "", fmt.Errorf("EUDM hostname is only supported on macOS and Windows: %w", errors.ErrUnsupported)
}
