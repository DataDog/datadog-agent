// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsimpl is the implementation for the secrets component
package secretsimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	cutoffLimit       = time.Hour * 24 * 90 // prune entries older than 90 days
	timeNowFunc       = time.Now
	apiKeyStringRegex = regexp.MustCompile(`^[[:xdigit:]]{32}(?:[[:xdigit]]{8})?$`)
)

// auditRow represents when a single secret is refreshed at a moment in time
type auditRow struct {
	When   time.Time `json:"when"`
	Handle string    `json:"handle"`
	Value  string    `json:"value,omitempty"`
}

// addToAuditFile adds rows to the audit file based upon newly refreshed secrets
func addToAuditFile(filename string, secretResponse map[string]string, origin handleToContext, maxSize int) error {
	if filename == "" {
		return nil
	}
	now := timeNowFunc().UTC()
	auditRows, err := loadAuditFile(filename, maxSize)
	if err != nil {
		if sizeErr, ok := err.(*filesystem.FileSizeLimitError); ok {
			return fmt.Errorf(`%s cannot be loaded, file size is too large: %v > %v
You can work-around this limit by setting the flag "secret_audit_file_max_size" to a larger value`, filename, sizeErr.Size, sizeErr.Limit)
		}
		if _, ok := err.(*os.PathError); !ok {
			return fmt.Errorf(`%s cannot be loaded: %s`, filename, err)
		}
	}

	// iterate keys in deterministic order by sorting
	handles := make([]string, 0, len(secretResponse))
	for handle := range secretResponse {
		handles = append(handles, handle)
	}
	sort.Strings(handles)
	// add the newly refreshed secrets to the list of rows
	for _, handle := range handles {
		secretValue := secretResponse[handle]
		scrubbedValue := ""
		if isLikelyAPIOrAppKey(handle, secretValue, origin) {
			scrubbedValue = scrubber.HideKeyExceptLastFiveChars(secretValue)
		}
		auditRows = append(auditRows, auditRow{When: now, Handle: handle, Value: scrubbedValue})
	}

	return saveAuditFile(filename, auditRows)
}

// return whether the secret is likely an API key or App key based whether it is 32 or 40 hex
// characters, as well as the setting name where it is found in the config
func isLikelyAPIOrAppKey(handle, secretValue string, origin handleToContext) bool {
	if !apiKeyStringRegex.MatchString(secretValue) {
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

// loadAuditFile loads the rows from the audit file and returns them
func loadAuditFile(filename string, maxSize int) ([]auditRow, error) {
	bytes, err := filesystem.ReadFileWithSizeLimit(filename, int64(maxSize))
	if err != nil {
		return nil, err
	}
	var rows []auditRow
	if len(bytes) == 0 {
		return rows, nil
	}
	err = json.Unmarshal(bytes, &rows)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// saveAuditFile saves the rows to the audit file, pruning those older than the cutoffLimit
func saveAuditFile(filename string, rows []auditRow) error {
	keepRows := make([]auditRow, 0, len(rows))

	// iterate rows, prune any that are older than cutoffLimit
	for _, row := range rows {
		delta := time.Since(row.When)
		if delta < cutoffLimit {
			keepRows = append(keepRows, row)
		}
	}

	data, err := json.MarshalIndent(keepRows, "", "  ")
	if err != nil {
		return err
	}
	err = os.WriteFile(filename, data, 0640)
	if err != nil {
		return err
	}
	return nil
}
