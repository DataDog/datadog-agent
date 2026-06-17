// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	"encoding/json"
	"errors"
	"fmt"

	sdsscanner "github.com/DataDog/datadog-agent/comp/core/sdsscanner/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	sdspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	sds "github.com/DataDog/datadog-agent/pkg/util/sds"
)

// sdsResultTransformer is the EventTransformer for sds-result events: it scans a
// database query result with the SDS scanner and re-encodes it as the SDS
// (Data Security) protobuf payload.
type sdsResultTransformer struct {
	scanners sdsscanner.Component
}

// newSDSResultTransformer returns the EventTransformer for sds-result events.
func newSDSResultTransformer() eventplatform.EventTransformer {
	return &sdsResultTransformer{}
}

// Init implements eventplatform.EventTransformer.
func (t *sdsResultTransformer) Init(deps eventplatform.TransformerDependencies) error {
	t.scanners = deps.Scanners
	return nil
}

// Transform implements eventplatform.EventTransformer.
func (t *sdsResultTransformer) Transform(e *message.Message) error {
	if t.scanners == nil {
		return errors.New("sds scanner component not available")
	}
	payload, err := buildSDSResultPayload(t.scanners, e.GetContent())
	if err != nil {
		return err
	}
	e.SetContent(payload)
	return nil
}

// queryColumn is one column of a database query result: its name and the
// values returned for that column across all scanned rows.
type queryColumn struct {
	Name   string        `json:"name"`
	Values []interface{} `json:"values"`
}

// queryResultEvent is the database query result submitted by an integration via
// submit_event_platform_event: the connection/query metadata plus the
// column-oriented result set.
type queryResultEvent struct {
	Timestamp int64         `json:"timestamp"`
	TaskID    string        `json:"task_id"`
	SubTaskID string        `json:"sub_task_id"`
	DBType    string        `json:"db_type"`
	DBHost    string        `json:"db_host"`
	DBPort    int           `json:"db_port"`
	DBName    string        `json:"db_name"`
	Query     string        `json:"query"`
	Columns   []queryColumn `json:"columns"`
	RowCount  int           `json:"row_count"`
	DurationS float64       `json:"duration_s"`
}

// buildSDSResultPayload decodes a database query result event, scans every
// column value with the SDS scanner registered under the event's task id, and
// returns the protobuf-marshaled SdsResultPayload ready to forward to the SDS
// (Data Security) intake.
func buildSDSResultPayload(scanners sdsscanner.Component, raw []byte) ([]byte, error) {
	var event queryResultEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, fmt.Errorf("sdsresult: decoding query result event: %w", err)
	}

	scanner, ok := scanners.Get(event.TaskID)
	if !ok {
		return nil, fmt.Errorf("sdsresult: no scanner registered for task %q", event.TaskID)
	}

	tableMatches, err := scanColumns(scanner, event.Columns)
	if err != nil {
		return nil, fmt.Errorf("sdsresult: scanning query result for task %q: %w", event.TaskID, err)
	}

	payload := &sdspb.SdsResultPayload{
		Timestamp: event.Timestamp,
		Resource: &sdspb.SdsResultPayload_Resource{
			Type: event.DBType,
			Name: event.DBName,
		},
		ScanResults: []*sdspb.SdsResultPayload_ScanResult{
			{
				Duration:     int64(event.DurationS * 1000),
				TableMatches: tableMatches,
			},
		},
	}

	out, err := payload.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("sdsresult: marshaling sds result for task %q: %w", event.TaskID, err)
	}
	return out, nil
}

// scanColumns runs every value of every column through the scanner and returns,
// for each column, one TableMatch per rule that fired, counting how many rows
// (values) in that column matched the rule.
func scanColumns(scanner sds.Scanner, columns []queryColumn) ([]*sdspb.SdsResultPayload_TableMatch, error) {
	var tableMatches []*sdspb.SdsResultPayload_TableMatch
	for _, col := range columns {
		counts := make(map[string]int64)
		var ruleOrder []string
		for _, v := range col.Values {
			hits, err := scanner.Scan([]byte(fmt.Sprintf("%v", v)))
			if err != nil {
				return nil, err
			}
			// A value can match a rule more than once; count the row only once
			// per rule.
			seen := make(map[string]struct{}, len(hits))
			for _, h := range hits {
				if _, dup := seen[h.RuleID]; dup {
					continue
				}
				seen[h.RuleID] = struct{}{}
				if _, ok := counts[h.RuleID]; !ok {
					ruleOrder = append(ruleOrder, h.RuleID)
				}
				counts[h.RuleID]++
			}
		}
		for _, ruleID := range ruleOrder {
			tableMatches = append(tableMatches, &sdspb.SdsResultPayload_TableMatch{
				RuleId:           ruleID,
				ColumnName:       col.Name,
				CountMatchedRows: counts[ruleID],
			})
		}
	}
	return tableMatches, nil
}
