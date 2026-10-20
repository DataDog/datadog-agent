// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const maxJournalFieldSize = 64 * 1024

func validateJournalQuery(query builtins.JournalQuery) error {
	if query.MaxEntries < 0 || query.MaxEntries > builtins.MaxJournalQueryEntries {
		return fmt.Errorf("journal query entry limit must be between 0 and %d", builtins.MaxJournalQueryEntries)
	}
	if query.Kernel && len(query.Units) > 0 {
		return errors.New("journal query cannot combine kernel and unit scopes")
	}
	if !query.Kernel && len(query.Units) == 0 {
		return errors.New("journal query requires a kernel or unit scope")
	}
	if len(query.Units) > builtins.MaxJournalQueryUnits {
		return fmt.Errorf("journal query has too many units (maximum %d)", builtins.MaxJournalQueryUnits)
	}
	for _, unit := range query.Units {
		if unit == "" || len(unit) > 256 || strings.IndexByte(unit, 0) >= 0 {
			return errors.New("journal query contains an invalid unit name")
		}
	}
	return nil
}
