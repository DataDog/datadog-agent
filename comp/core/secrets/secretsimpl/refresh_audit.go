// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl is the implementation for the secrets component
package secretsimpl

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	cutoffLimit    = time.Hour * 24 * 90 // prune entries older than 90 days
	timeNowFunc    = time.Now
	hexStringRegex = regexp.MustCompile(`\b[a-fA-F0-9]+\b`)
)

// RefreshAuditRow represents when a single secret is refreshed at a moment in time
type RefreshAuditRow struct {
	When   time.Time
	Handle string
	Value  string
}

// AddToRefreshAuditFile adds rows to the audit file based upon newly refreshed secrets
func AddToRefreshAuditFile(filename string, secretResponse map[string]string, origin handleToContext) error {
	now := timeNowFunc().UTC()
	auditRows := loadRefreshAuditFile(filename)

	// add the newly refreshed secrets to the list of rows
	for handle, secretValue := range secretResponse {
		scrubbedValue := "********"
		if isLikelyAPIOrAppKey(handle, secretValue, origin) {
			scrubbedValue = scrubber.HideKeyExceptLastFiveChars(secretValue)
		}
		auditRows = append(auditRows, RefreshAuditRow{When: now, Handle: handle, Value: scrubbedValue})
	}

	return saveRefreshAuditFile(filename, auditRows)
}

// return whether the secret is likely an API key or App key based upon a regex, its
// length (must be 32 or 40), as well as the setting name where it is found in the config
func isLikelyAPIOrAppKey(handle, secretValue string, origin handleToContext) bool {
	if origin == nil || !hexStringRegex.MatchString(secretValue) || (len(secretValue) != 32 && len(secretValue) != 40) {
		return false
	}
	for _, secretCtx := range origin[handle] {
		lastElem := secretCtx.path[len(secretCtx.path)-1]
		if strings.HasSuffix(strings.ToLower(lastElem), "key") {
			return true
		}
	}
	return false
}

// loadRefreshAuditFile loads the rows from the audit file and returns them
func loadRefreshAuditFile(filename string) []RefreshAuditRow {
	bytes, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}
	var rows []RefreshAuditRow
	err = json.Unmarshal(bytes, &rows)
	if err != nil {
		return nil
	}
	return rows
}

// saveRefreshAuditFile saves the rows to the audit file, removing those older than the cutoffLimit
func saveRefreshAuditFile(filename string, rows []RefreshAuditRow) error {
	var keepRows []RefreshAuditRow

	// iterate rows, drop any that are older than cutoffLimit
	for i, row := range rows {
		delta := time.Since(row.When)
		if delta < cutoffLimit {
			if keepRows == nil {
				keepRows = make([]RefreshAuditRow, 0, len(rows)-i)
			}
			keepRows = append(keepRows, row)
		}
	}

	data, err := json.MarshalIndent(keepRows, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
