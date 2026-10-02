// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !darwin && !windows

// Package native isolates platform collection from portable replay.
package native

import (
	"context"
	"fmt"
)

func Run(context.Context, string) error {
	return fmt.Errorf("native capture requires Windows or macOS; supply a separate capture bundle for replay")
}
