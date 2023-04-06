// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"database/sql"
	"encoding/json"
	"fmt"
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

// Metadata contains the metadata fields common to all events processed
type Metadata struct {
	Timestamp      float64 `json:"timestamp,omitempty"`
	Host           string  `json:"host,omitempty"`
	Source         string  `json:"ddsource,omitempty"`
	DBMType        string  `json:"dbm_type,omitempty"`
	DDAgentVersion string  `json:"ddagentversion,omitempty"`
}

type OracleActivityRow struct {
	Now                    string `json:"now"`
	SessionID              uint64 `json:"sid,omitempty"`
	SessionSerial          uint64 `json:"serial,omitempty"`
	User                   string `json:"user,omitempty"`
	Status                 string `json:"status"`
	OsUser                 string `json:"os_user,omitempty"`
	Process                string `json:"process,omitempty"`
	Client                 string `json:"client,omitempty"`
	Port                   uint64 `json:"port,omitempty"`
	Program                string `json:"program,omitempty"`
	Type                   string `json:"type,omitempty"`
	SQLID                  string `json:"sql_id,omitempty"`
	ForceMatchingSignature uint64 `json:"force_matching_signature,omitempty"`
	//ForceMatchingSignature string `json:"force_matching_signature,omitempty"`
	SQLPlanHashValue      uint64 `json:"sql_plan_hash_value,omitempty"`
	SQLExecStart          string `json:"sql_exec_start,omitempty"`
	Module                string `json:"module,omitempty"`
	Action                string `json:"action,omitempty"`
	ClientInfo            string `json:"client_info,omitempty"`
	LogonTime             string `json:"logon_time,omitempty"`
	ClientIdentifier      string `json:"client_identifier,omitempty"`
	BlockingInstance      uint64 `json:"blocking_instance,omitempty"`
	BlockingSession       uint64 `json:"blocking_session,omitempty"`
	FinalBlockingInstance uint64 `json:"final_blocking_instance,omitempty"`
	FinalBlockingSession  uint64 `json:"final_blocking_session,omitempty"`
	WaitEvent             string `json:"wait_event,omitempty"`
	WaitEventGroup        string `json:"wait_event_group,omitempty"`
	WaitTimeMicro         uint64 `json:"wait_time_micro,omitempty"`
	Statement             string `json:"statement,omitempty"`
	PdbName               string `json:"pdb_name,omitempty"`
	CdbName               string `json:"cdb_name,omitempty"`
	QuerySignature        string `json:"query_signature,omitempty"`
	RowMetadata
}

type OracleActivityRowDB struct {
	Now           string         `db:"NOW"`
	SessionID     uint64         `db:"SID"`
	SessionSerial uint64         `db:"SERIAL#"`
	User          sql.NullString `db:"USERNAME"`
	Status        string         `db:"STATUS"`
	OsUser        sql.NullString `db:"OSUSER"`
	Process       sql.NullString `db:"PROCESS"`
	Client        sql.NullString `db:"MACHINE"`
	Port          sql.NullInt64  `db:"PORT"`
	Program       sql.NullString `db:"PROGRAM"`
	Type          sql.NullString `db:"TYPE"`
	SQLID         sql.NullString `db:"SQL_ID"`
	//ForceMatchingSignature *uint64        `db:"FORCE_MATCHING_SIGNATURE"`
	ForceMatchingSignature *string        `db:"FORCE_MATCHING_SIGNATURE"`
	SQLPlanHashValue       *uint64        `db:"SQL_PLAN_HASH_VALUE"`
	SQLExecStart           sql.NullString `db:"SQL_EXEC_START"`
	Module                 sql.NullString `db:"MODULE"`
	Action                 sql.NullString `db:"ACTION"`
	ClientInfo             sql.NullString `db:"CLIENT_INFO"`
	LogonTime              sql.NullString `db:"LOGON_TIME"`
	ClientIdentifier       sql.NullString `db:"CLIENT_IDENTIFIER"`
	BlockingInstance       *uint64        `db:"BLOCKING_INSTANCE"`
	BlockingSession        *uint64        `db:"BLOCKING_SESSION"`
	FinalBlockingInstance  *uint64        `db:"FINAL_BLOCKING_INSTANCE"`
	FinalBlockingSession   *uint64        `db:"FINAL_BLOCKING_SESSION"`
	WaitEvent              sql.NullString `db:"EVENT"`
	WaitEventGroup         sql.NullString `db:"WAIT_CLASS"`
	WaitTimeMicro          *uint64        `db:"WAIT_TIME_MICRO"`
	Statement              sql.NullString `db:"SQL_TEXT"`
	PdbName                sql.NullString `db:"PDB_NAME"`
}

func (c *Check) SampleSession() error {
	start := time.Now()

	var sessionRows []OracleActivityRow
	sessionSamples := []OracleActivityRowDB{}
	err := c.db.Select(&sessionSamples, ACTIVITY_QUERY)

	if err != nil {
		return fmt.Errorf("failed to collect session sampling activity: %w", err)
	}

	log.Tracef("activity query returned %d rows\n", len(sessionSamples))

	//forceMatchingSignatures := make(map[uint64]int)
	forceMatchingSignatures := make(map[string]int)
	SQLIDs := make(map[string]int)

	o := obfuscate.NewObfuscator(obfuscate.Config{SQL: c.config.ObfuscatorOptions})
	for _, sample := range sessionSamples {
		var sessionRow OracleActivityRow
		forceMatchingSignature := ""
		sessionRow.Now = sample.Now
		sessionRow.SessionID = sample.SessionID
		sessionRow.SessionSerial = sample.SessionSerial
		if sample.User.Valid {
			sessionRow.User = sample.User.String
		}
		sessionRow.Status = sample.Status
		if sample.OsUser.Valid {
			sessionRow.OsUser = sample.OsUser.String
		}
		if sample.Process.Valid {
			sessionRow.Process = sample.Process.String
		}
		if sample.Client.Valid {
			sessionRow.Client = sample.Client.String
		}
		if sample.Port.Valid {
			sessionRow.Port = uint64(sample.Port.Int64)
		}
		if sample.Program.Valid {
			sessionRow.Program = sample.Program.String
		}
		if sample.Type.Valid {
			sessionRow.Type = sample.Type.String
		}
		if sample.SQLID.Valid {
			sessionRow.SQLID = sample.SQLID.String
		} else {
			sessionRow.SQLID = ""
		}
		if sample.ForceMatchingSignature != nil {
			//sessionRow.ForceMatchingSignature = *sample.ForceMatchingSignature
			forceMatchingSignature = *sample.ForceMatchingSignature
			if err != nil {
				return fmt.Errorf("failed converting force_matching_signature to uint64 %w", err)
			}
			sessionRow.ForceMatchingSignature, err = strconv.ParseUint(forceMatchingSignature, 10, 64)
			//forceMatchingSignatures[sessionRow.ForceMatchingSignature] = 1
			forceMatchingSignatures[*sample.ForceMatchingSignature] = 1
			//if sessionRow.ForceMatchingSignature == 0 && sample.SQLID.Valid {
			//if sessionRow.ForceMatchingSignature == "" && sample.SQLID.Valid {
			if *sample.ForceMatchingSignature == "" && sample.SQLID.Valid {
				SQLIDs[sessionRow.SQLID] = 1
			}

		} else {
			sessionRow.ForceMatchingSignature = 0
			//sessionRow.ForceMatchingSignature = ""
		}
		if sample.SQLPlanHashValue != nil {
			sessionRow.SQLPlanHashValue = *sample.SQLPlanHashValue
		}
		if sample.SQLExecStart.Valid {
			sessionRow.SQLExecStart = sample.SQLExecStart.String
		}
		if sample.Module.Valid {
			sessionRow.Module = sample.Module.String
		}
		if sample.Action.Valid {
			sessionRow.Action = sample.Action.String
		}
		if sample.ClientInfo.Valid {
			sessionRow.ClientInfo = sample.ClientInfo.String
		}
		if sample.LogonTime.Valid {
			sessionRow.LogonTime = sample.LogonTime.String
		}
		if sample.ClientIdentifier.Valid {
			sessionRow.ClientIdentifier = sample.ClientIdentifier.String
		}
		if sample.BlockingInstance != nil {
			sessionRow.BlockingInstance = *sample.BlockingInstance
		}
		if sample.BlockingSession != nil {
			sessionRow.BlockingSession = *sample.BlockingSession
		}
		if sample.FinalBlockingInstance != nil {
			sessionRow.FinalBlockingInstance = *sample.FinalBlockingInstance
		}
		if sample.FinalBlockingSession != nil {
			sessionRow.FinalBlockingSession = *sample.FinalBlockingSession
		}
		if sample.WaitEvent.Valid {
			sessionRow.WaitEvent = sample.WaitEvent.String
		}
		if sample.WaitEventGroup.Valid {
			sessionRow.WaitEventGroup = sample.WaitEventGroup.String
		}
		if sample.WaitTimeMicro != nil {
			sessionRow.WaitTimeMicro = *sample.WaitTimeMicro
		}
		if sample.Statement.Valid {
			obfuscatedStatement, err := c.GetObfuscatedStatement(o, sample.Statement.String, forceMatchingSignature, sessionRow.SQLID)
			sessionRow.Statement = obfuscatedStatement.Statement
			if err == nil {
				sessionRow.Commands = obfuscatedStatement.Commands
				sessionRow.Tables = obfuscatedStatement.Tables
				sessionRow.Comments = obfuscatedStatement.Comments
				sessionRow.QuerySignature = obfuscatedStatement.QuerySignature
			}
		}
		if sample.PdbName.Valid {
			sessionRow.PdbName = sample.PdbName.String
		}
		sessionRow.CdbName = c.cdbName
		sessionRows = append(sessionRows, sessionRow)
	}
	o.Stop()

	payload := ActivitySnapshot{
		Metadata: Metadata{
			Timestamp:      float64(time.Now().UnixMilli()),
			Host:           c.dbHostname,
			Source:         common.IntegrationName,
			DBMType:        "activity",
			DDAgentVersion: c.agentVersion,
		},
		CollectionInterval: c.checkInterval,
		Tags:               c.tags,
		OracleActivityRows: sessionRows,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("Error marshalling activity payload: %s", err)
		return err
	}

	log.Tracef("Activity payload %s", strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("GetSender SampleSession %s", string(payloadBytes))
		return err
	}

	//3.43
	//sender.EventPlatformEvent(string(payloadBytes), "dbm-activity")
	sender.EventPlatformEvent(payloadBytes, "dbm-activity")

	sender.Count("dd.oracle.activity.samples_count", float64(len(sessionRows)), c.hostname, c.tags)
	sender.Gauge("dd.oracle.activity.time_ms", float64(time.Since(start).Milliseconds()), c.hostname, c.tags)
	sender.Commit()

	c.statementsFilter.SQLIDs = SQLIDs
	c.statementsFilter.ForceMatchingSignatures = forceMatchingSignatures

	return nil
}
