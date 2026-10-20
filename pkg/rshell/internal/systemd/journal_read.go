// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// ReadJournal reads a bounded structured query from the configured journal
// files. Results are buffered until the file snapshot is stable, then yielded
// oldest first as required by builtins.JournalReader.
func (c *Client) ReadJournal(ctx context.Context, query builtins.JournalQuery, yield func(builtins.JournalEntry) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if yield == nil {
		return fmt.Errorf("journal entry yield function is nil")
	}
	entries, err := c.queryJournalEntries(ctx, query)
	if err != nil {
		return err
	}
	for index := len(entries) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := yield(entries[index].selected); err != nil {
			return err
		}
	}
	return nil
}
