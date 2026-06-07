// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strconv"
	"time"

	sdspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/pkg/util/sds"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// dbScanEvent is the raw payload produced by database integrations: a sampling
// of rows from a table, together with the table/database metadata.
type dbScanEvent struct {
	Host              string                   `json:"host"`
	DatabaseInstance  string                   `json:"database_instance"`
	Table             string                   `json:"table"`
	ScanType          string                   `json:"scan_type"`
	EstimatedRowCount int64                    `json:"estimated_row_count"`
	SampleSize        int64                    `json:"sample_size"`
	Rows              []map[string]interface{} `json:"rows"`
	Timestamp         int64                    `json:"timestamp"`
}

// buildSdsResultPayload converts a scanned database event into a single
// SdsResultPayload protobuf, wire-compatible with the sds-intake.
func buildSdsResultPayload(event dbScanEvent, rowMatches [][]sds.Match, scanDuration time.Duration) *sdspb.SdsResultPayload {
	durationMs := scanDuration.Milliseconds()

	totalRows := event.SampleSize
	if totalRows == 0 {
		totalRows = int64(len(event.Rows))
	}

	var (
		scannedBytes int64
		rules        = map[string]*sdspb.SdsResultPayload_RuleInfo{}
		// matchedRows[ruleID][column] = set of matched row indices
		matchedRows = map[string]map[string]map[int]struct{}{}
	)

	for i, row := range event.Rows {
		for _, v := range row {
			if s, ok := v.(string); ok {
				scannedBytes += int64(len(s))
			}
		}

		for _, m := range rowMatches[i] {
			ruleID := sds.RuleID(m.RuleIdx)
			if ruleID == "" {
				ruleID = strconv.FormatUint(uint64(m.RuleIdx), 10)
			}
			column := m.Path

			if _, ok := rules[ruleID]; !ok {
				rules[ruleID] = &sdspb.SdsResultPayload_RuleInfo{Id: ruleID}
			}
			if matchedRows[ruleID] == nil {
				matchedRows[ruleID] = map[string]map[int]struct{}{}
			}
			if matchedRows[ruleID][column] == nil {
				matchedRows[ruleID][column] = map[int]struct{}{}
			}
			matchedRows[ruleID][column][i] = struct{}{}
		}
	}

	var dbMatches []*sdspb.SdsResultPayload_DbMatch
	for ruleID, columns := range matchedRows {
		for column, rowsSet := range columns {
			dbMatches = append(dbMatches, &sdspb.SdsResultPayload_DbMatch{
				RuleId:           ruleID,
				ColumnName:       column,
				CountMatchedRows: int64(len(rowsSet)),
				CountTotalRows:   totalRows,
			})
		}
	}

	// RDS resources are read from the rdsTable oneof; the deprecated flat
	// database/table fields are kept for backward compatibility with the intake.
	location := &sdspb.SdsResultPayload_ScanLocation{
		Table:            event.Table,
		ScanLocationType: &sdspb.SdsResultPayload_ScanLocation_Database{Database: event.DatabaseInstance},
		ScanLocation: &sdspb.SdsResultPayload_ScanLocation_RdsTable{
			RdsTable: &sdspb.SdsResultPayload_RdsTable{
				DatabaseName: event.DatabaseInstance,
				TableName:    event.Table,
			},
		},
	}

	timestamp := event.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().UnixMilli()
	}

	payload := &sdspb.SdsResultPayload{
		ScanSource: sdspb.SdsResultPayload_AGENTLESS,
		Timestamp:  timestamp,
		Resource: &sdspb.SdsResultPayload_Resource{
			Type: "aws_rds_instance",
			Name: event.DatabaseInstance,
		},
		ScanResults: []*sdspb.SdsResultPayload_ScanResult{
			{
				Duration:  durationMs,
				DbMatches: dbMatches,
				Location:  location,
			},
		},
		ScanStats: &sdspb.ScanStats{
			ScanDurationMs:        durationMs,
			TotalDataScannedBytes: scannedBytes,
		},
		Rules: rules,
	}

	if v := version.AgentVersion; v != "" {
		payload.ScannerMetadata = &sdspb.ScannerMetadata{Version: &v}
	}

	return payload
}
