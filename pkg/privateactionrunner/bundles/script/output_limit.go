// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_script

import (
	"errors"
	"fmt"
)

const defaultMaxOutputSize = 10 * 1024 * 1024 // 10MB

// errOutputLimitExceeded is a sentinel used for errors.Is matching.
var errOutputLimitExceeded = errors.New("script output limit exceeded")

func newOutputLimitError(limit int64) error {
	return fmt.Errorf("script output exceeded %dMB limit: %w", limit/(1024*1024), errOutputLimitExceeded)
}
