// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ActivitySnapshot is a payload containing database activity samples. It is parsed from the intake payload.
// easyjson:json
type ActivitySnapshot struct {
	Metadata
	// Tags should be part of the common Metadata struct but because Activity payloads use a string array
	// and samples use a comma-delimited list of tags in a single string, both flavors have to be handled differently
	Tags               []string            `json:"ddtags,omitempty"`
	CollectionInterval float64             `json:"collection_interval,omitempty"`
	OracleActivityRows []OracleActivityRow `json:"oracle_activity,omitempty"`
}

const ACTIVITY_QUERY = `SELECT /* DD_ACTIVITY_SAMPLING */
	SYSDATE as now,
	sid,
	serial#,
	username,
	status,
    osuser,
    process, 
    machine, 
    program ,
    type,
    sql_id,
	force_matching_signature,
    sql_plan_hash_value,
    sql_exec_start,
    module,
    action,
    client_info,
    logon_time,
    client_identifier,
    CASE WHEN blocking_session_status = 'VALID' THEN
	  blocking_instance
	ELSE
	  null
	END blocking_instance,
    CASE WHEN blocking_session_status = 'VALID' THEN
		blocking_session
	ELSE
		null
	END blocking_session,
	CASE WHEN final_blocking_session_status = 'VALID' THEN
    	final_blocking_instance
	ELSE
		null
	END final_blocking_instance,
	CASE WHEN final_blocking_session_status = 'VALID' THEN
    	final_blocking_session
	ELSE
		null
	END final_blocking_session,
    CASE WHEN state = 'WAITING' THEN
		event
	ELSE
		'CPU'
	END event,
	CASE WHEN state = 'WAITING' THEN
    	wait_class
	ELSE
		'CPU'
	END wait_class,
	wait_time_micro,
	sql_text,
	pdb_name
FROM sys.dd_session
WHERE 
	( sql_text NOT LIKE '%DD_ACTIVITY_SAMPLING%' OR sql_text is NULL ) 
	AND (
		NOT (state = 'WAITING' AND wait_class = 'Idle')
		OR state = 'WAITING' AND event = 'fbar timer' AND type = 'USER'
	)
	AND status = 'ACTIVE'`

type RowMetadata struct {
	Commands       []string `json:"dd_commands,omitempty"`
	Tables         []string `json:"dd_tables,omitempty"`
	Comments       []string `json:"dd_comments,omitempty"`
	QueryTruncated string   `json:"query_truncated,omitempty"`
}

type OracleActivityRow struct {
	Now                    string  `db:"NOW" json:"now"`
	SessionID              uint64  `db:"SID" json:"sid,omitempty"`
	SessionSerial          uint64  `db:"SERIAL#" json:"serial,omitempty"`
	User                   *string `db:"USERNAME" json:"user,omitempty"`
	Status                 string  `db:"STATUS" json:"status"`
	OsUser                 *string `db:"OSUSER" json:"os_user,omitempty"`
	Process                *string `db:"PROCESS" json:"process,omitempty"`
	Client                 *string `db:"MACHINE" json:"client,omitempty"`
	Port                   *string `db:"PORT" json:"port,omitempty"`
	Program                *string `db:"PROGRAM" json:"program,omitempty"`
	Type                   *string `db:"TYPE" json:"type,omitempty"`
	SqlID                  *string `db:"SQL_ID" json:"sql_id,omitempty"`
	ForceMatchingSignature *uint64 `db:"FORCE_MATCHING_SIGNATURE" json:"force_matching_signature,omitempty"`
	SqlPlanHashValue       *uint64 `db:"SQL_PLAN_HASH_VALUE" json:"sql_plan_hash_value,omitempty"`
	SqlExecStart           *string `db:"SQL_EXEC_START" json:"sql_exec_start,omitempty"`
	Module                 *string `db:"MODULE" json:"module,omitempty"`
	Action                 *string `db:"ACTION" json:"action,omitempty"`
	ClientInfo             *string `db:"CLIENT_INFO" json:"client_info,omitempty"`
	LogonTime              *string `db:"LOGON_TIME" json:"logon_time,omitempty"`
	ClientIdentifier       *string `db:"CLIENT_IDENTIFIER" json:"client_identifier,omitempty"`
	BlockingInstance       *uint64 `db:"BLOCKING_INSTANCE" json:"blocking_instance,omitempty"`
	BlockingSession        *uint64 `db:"BLOCKING_SESSION" json:"blocking_session,omitempty"`
	FinalBlockingInstance  *uint64 `db:"FINAL_BLOCKING_INSTANCE" json:"final_blocking_instance,omitempty"`
	FinalBlockingSession   *uint64 `db:"FINAL_BLOCKING_SESSION" json:"final_blocking_session,omitempty"`
	WaitEvent              *string `db:"EVENT" json:"wait_event,omitempty"`
	WaitEventGroup         *string `db:"WAIT_CLASS" json:"wait_event_group,omitempty"`
	WaitTimeMicro          *string `db:"WAIT_TIME_MICRO" json:"wait_time_micro,omitempty"`
	Statement              *string `db:"SQL_TEXT" json:"statement,omitempty"`
	PdbName                *string `db:"PDB_NAME" json:"pdb_name,omitempty"`
	QuerySignature         string  `json:"query_signature,omitempty"`
	RowMetadata
}

// Metadata contains the metadata fields common to all events processed
type Metadata struct {
	Timestamp      float64 `json:"timestamp,omitempty"`
	Host           string  `json:"host,omitempty"`
	Source         string  `json:"ddsource,omitempty"`
	DBMType        string  `json:"dbm_type,omitempty"`
	DDAgentVersion string  `json:"ddagentversion,omitempty"`
}

/*
type MetricSender struct {
	sender           aggregator.Sender
	hostname         string
	submittedMetrics int
}
*/

func (c *Check) SampleSession() error {
	start := time.Now()

	sessionSamples := []OracleActivityRow{}
	err := c.db.Select(&sessionSamples, ACTIVITY_QUERY)

	if err != nil {
		log.Errorf("Session sampling %s", err)
		return err
	}
	o := obfuscate.NewObfuscator(obfuscate.Config{SQL: c.config.ObfuscatorOptions})
	if c.config.ObfuscatorOn {
		for i, sample := range sessionSamples {
			if sample.Statement != nil && *sample.Statement != "" {
				obfuscatedQuery, err := o.ObfuscateSQLString(*sample.Statement)
				if err != nil {
					error_text := fmt.Sprintf("query obfuscation failed for SQL_ID: %s", *sample.SqlID)
					if c.config.InstanceConfig.LogUnobfuscatedQueries {
						error_text = error_text + fmt.Sprintf(" SQL: %s", *sample.Statement)
					}
					log.Error(error_text)
				} else {
					*sample.Statement = obfuscatedQuery.Query
					sessionSamples[i].Commands = obfuscatedQuery.Metadata.Commands
					sessionSamples[i].Tables = strings.Split(obfuscatedQuery.Metadata.TablesCSV, ",")
					sessionSamples[i].Comments = obfuscatedQuery.Metadata.Comments
					h := fnv.New64a()
					h.Write([]byte(*sample.Statement))
					sessionSamples[i].QuerySignature = strconv.FormatUint(h.Sum64(), 10)
				}
			}
		}
	}
	o.Stop()

	payload := ActivitySnapshot{
		Metadata: Metadata{
			Timestamp:      float64(time.Now().UnixMilli()),
			Host:           c.hostname,
			Source:         common.IntegrationName,
			DBMType:        "activity",
			DDAgentVersion: c.agentVersion,
		},
		CollectionInterval: c.checkInterval,
		Tags:               c.tags,
		OracleActivityRows: sessionSamples,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
		return err
	}

	log.Tracef("JSON payload %s", strings.ReplaceAll(string(payloadBytes), "@", "xXx"))
	//fmt.Println("JSON payload", string(payloadBytes))

	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("GetSender SampleSession %s", string(payloadBytes))
		return err
	}
	sender.EventPlatformEvent(string(payloadBytes), "dbm-activity")
	sender.Gauge("dd.oracle.activity.time_ms", float64(time.Since(start).Milliseconds()), c.hostname, c.tags)
	sender.Commit()
	return nil
}
