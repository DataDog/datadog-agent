// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package systemd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// RotateJournal requires a Linux systemd-journald control endpoint.
func (c *Client) RotateJournal(context.Context) error {
	return fmt.Errorf("%w: journal rotation requires Linux", builtins.ErrSystemdUnsupported)
}
