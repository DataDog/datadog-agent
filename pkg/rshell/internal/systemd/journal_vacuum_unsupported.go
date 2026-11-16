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

// VacuumJournal requires rooted Unix filesystem operations.
func (c *Client) VacuumJournal(context.Context, builtins.JournalVacuumRequest) (builtins.JournalVacuumResult, error) {
	return builtins.JournalVacuumResult{}, fmt.Errorf("%w: journal vacuum requires Linux or macOS", builtins.ErrSystemdUnsupported)
}
