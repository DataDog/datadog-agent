// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runners

import (
	"context"
)

// Runner defines the standard interface for all runners
type Runner interface {
	// Start starts the runner with the given context
	Start(ctx context.Context) error
	// Stop gracefully stops the runner with the given context
	Stop(ctx context.Context) error
}
