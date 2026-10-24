// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin

package systemd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// JournalDiskUsage requires Unix allocation metadata.
func (c *Client) JournalDiskUsage(context.Context) (builtins.JournalUsage, error) {
	return builtins.JournalUsage{}, fmt.Errorf("%w: journal disk usage requires Linux or macOS", builtins.ErrSystemdUnsupported)
}
