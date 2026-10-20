// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package systemd

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const journalRotationTimeout = 30 * time.Second

// RotateJournal asks journald to rotate active journals and waits for the
// daemon to report completion.
func (c *Client) RotateJournal(ctx context.Context) error {
	if c.target.MachineIDPath == "" {
		return fmt.Errorf("systemd target machine ID path is unavailable")
	}
	if _, err := c.readMachineID(); err != nil {
		return fmt.Errorf("validate systemd target machine ID: %w", err)
	}
	if c.target.JournalControlSocket == "" {
		return fmt.Errorf("systemd target journal control socket is unavailable")
	}

	rotationCtx, cancel := context.WithTimeout(ctx, journalRotationTimeout)
	defer cancel()
	if err := c.rotateJournalControl(rotationCtx, c.target.JournalControlSocket); err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return fmt.Errorf("journal rotation timed out after %s", journalRotationTimeout)
		}
		return fmt.Errorf("rotate journal: %w", err)
	}
	return nil
}
