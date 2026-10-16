// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datasecurity

import (
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/sds"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// sdsResourceTypePostgresTable is the sds-result resource type of a scanned postgres table.
// It matches the one the datasecurity check reports on success.
const sdsResourceTypePostgresTable = "postgres_table"

// sdsResultReporter reports sub task failures that happen before the datasecurity check could
// run (and report on its own) as `sds-result` events on the event platform.
type sdsResultReporter struct {
	eventPlatform eventplatform.Component
}

// reportFailure sends one ERROR sds-result per sub task. reasons maps a sub task ID to its failure
// reason; sub tasks absent from the map fall back to defaultReason.
func (r *sdsResultReporter) reportFailure(payload scanTaskPayload, reasons map[string]string, defaultReason string) {
	if len(payload.ScanData) == 0 {
		log.Debugf("no sub task to report as failed for Data Security scan task %q", payload.TaskID)
		return
	}
	forwarder, ok := r.forwarder()
	if !ok {
		log.Warnf("Event Platform forwarder unavailable, cannot report failure of Data Security scan task %q", payload.TaskID)
		return
	}

	ruleIDs := scanningRuleIDs(payload.ScanningRules)
	now := time.Now()
	for _, st := range payload.ScanData {
		reason, ok := reasons[st.SubTaskID]
		if !ok {
			reason = defaultReason
		}
		result := buildFailedSDSResult(payload.TaskID, st, ruleIDs, reason, now)
		if err := sendSDSResult(forwarder, result); err != nil {
			log.Warnf("failed to send sds-result for sub task %q of scan task %q: %v", st.SubTaskID, payload.TaskID, err)
		}
	}
}

func (r *sdsResultReporter) forwarder() (eventplatform.Forwarder, bool) {
	if r == nil || r.eventPlatform == nil {
		return nil, false
	}
	forwarder, ok := r.eventPlatform.Get()
	if !ok || forwarder == nil {
		return nil, false
	}
	return forwarder, true
}

// sendSDSResult marshals the result and hands it to the `sds-result` event platform track.
func sendSDSResult(forwarder eventplatform.Forwarder, result *sds.SdsResultPayload) error {
	raw, err := proto.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal sds-result: %w", err)
	}
	return forwarder.SendEventPlatformEvent(message.NewMessage(raw, nil, "", 0), eventplatform.EventTypeSDSResult)
}

// buildFailedSDSResult builds the sds-result of a sub task that failed without being scanned. It
// mirrors the payload the datasecurity check emits for a failed sub task.
func buildFailedSDSResult(taskID string, st subTask, ruleIDs []string, reason string, now time.Time) *sds.SdsResultPayload {
	e := st.Entity
	return &sds.SdsResultPayload{
		Timestamp: now.UnixMilli(),
		Resource: &sds.SdsResultPayload_Resource{
			Type: sdsResourceTypePostgresTable,
			Name: fmt.Sprintf("%s.%s.%s.%s", e.DatabaseInstanceName, e.Database, e.Schema, e.Table),
		},
		RuleIds: ruleIDs,
		// TODO(dsec-228): populate the agent hostname and version, as in the check's results.
		ScanningSource: &sds.ScanningSource{
			Source: &sds.ScanningSource_Agent_{Agent: &sds.ScanningSource_Agent{}},
		},
		ScanResults: []*sds.SdsResultPayload_ScanResult{{
			Location: &sds.SdsResultPayload_ScanLocation{
				ScanLocation: &sds.SdsResultPayload_ScanLocation_PostgresTable{
					PostgresTable: &sds.SdsResultPayload_PostgresTable{
						DatabaseClusterName:  e.DatabaseClusterName,
						DatabaseInstanceName: e.DatabaseInstanceName,
						// The local connection may not have been resolved, use the host the task targets.
						DatabaseHostName: e.DatabaseHostName,
						DatabaseName:     e.Database,
						SchemaName:       e.Schema,
						TableName:        e.Table,
					},
				},
			},
			ScanMetadata: &sds.SdsResultPayload_ScanMetadata{
				ScanTaskMetadata: &sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata{
					TaskId:        taskID,
					SubTaskId:     st.SubTaskID,
					Status:        sds.SdsResultPayload_ScanMetadata_ScanTaskMetadata_ERROR,
					FailureReason: proto.String(reason),
				},
			},
		}},
	}
}

// scanningRuleIDs extracts the id of each scanning rule, skipping rules that cannot be decoded.
func scanningRuleIDs(rules []json.RawMessage) []string {
	ids := make([]string, 0, len(rules))
	for _, raw := range rules {
		var rule struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &rule); err != nil || rule.ID == "" {
			continue
		}
		ids = append(ids, rule.ID)
	}
	return ids
}
