package oracle

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/godror/godror"
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

const ACTIVITY_QUERY = `SELECT 
	sid,
	serial#,
	username,
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
    final_blocking_instance,
    final_blocking_session, 
    CASE WHEN state = 'WAITING' THEN
		event
	ELSE
		'CPU'
	END event,
    wait_class,
	sql_text,
	pdb_name
FROM sys.dd_session
WHERE 
	( module != 'datadog agent' and action != 'session sampling' or module is null or action is null) 
	AND NOT (state = 'WAITING' and wait_class = 'Idle' and event != 'fbar timer')`

type OracleActivityRow struct {
	SessionID              uint64  `db:"SID" json:"oracle.sid,omitempty"`
	SessionSerial          uint64  `db:"SERIAL#" json:"oracle.serial,omitempty"`
	Username               *string `db:"USERNAME" json:"oracle.username,omitempty"`
	OsUser                 *string `db:"OSUSER" json:"oracle.os_user,omitempty"`
	Process                *string `db:"PROCESS" json:"oracle.process,omitempty"`
	Machine                *string `db:"MACHINE" json:"oracle.machine,omitempty"`
	Program                *string `db:"PROGRAM" json:"oracle.program,omitempty"`
	Type                   *string `db:"TYPE" json:"oracle.type,omitempty"`
	SqlID                  *string `db:"SQL_ID" json:"oracle.sql_id,omitempty"`
	ForceMatchingSignature *uint64 `db:"FORCE_MATCHING_SIGNATURE" json:"oracle.force_matching_signature,omitempty"`
	SqlPlanHashValue       *uint64 `db:"SQL_PLAN_HASH_VALUE" json:"oracle.sql_plan_hash_value,omitempty"`
	SqlExecStart           *string `db:"SQL_EXEC_START" json:"oracle.sql_exec_start,omitempty"`
	Module                 *string `db:"MODULE" json:"oracle.module,omitempty"`
	Action                 *string `db:"ACTION" json:"oracle.action,omitempty"`
	ClientInfo             *string `db:"CLIENT_INFO" json:"oracle.client_info,omitempty"`
	LogonTime              *string `db:"LOGON_TIME" json:"oracle.logon_time,omitempty"`
	ClientIdentifier       *string `db:"CLIENT_IDENTIFIER" json:"oracle.client_identifier,omitempty"`
	BlockingInstance       *uint64 `db:"BLOCKING_INSTANCE" json:"oracle.blocking_instance,omitempty"`
	BlockingSession        *uint64 `db:"BLOCKING_SESSION" json:"oracle.blocking_session,omitempty"`
	FinalBlockingInstance  *uint64 `db:"FINAL_BLOCKING_INSTANCE" json:"oracle.final_blocking_instance,omitempty"`
	FinalBlockingSession   *uint64 `db:"FINAL_BLOCKING_SESSION" json:"oracle.final_blocking_session,omitempty"`
	Event                  *string `db:"EVENT" json:"oracle.event,omitempty"`
	WaitClass              *string `db:"WAIT_CLASS" json:"oracle.wait_class,omitempty"`
	SqlText                *string `db:"SQL_TEXT" json:"oracle.sql_text,omitempty"`
	PdbName                *string `db:"PDB_NAME" json:"oracle.pdb_name,omitempty"`
}

// Metadata contains the metadata fields common to all events processed
type Metadata struct {
	Timestamp      float64 `json:"timestamp,omitempty"`
	Host           string  `json:"host,omitempty"`
	Source         string  `json:"ddsource,omitempty"`
	DBMType        string  `json:"dbm_type,omitempty"`
	DDAgentVersion string  `json:"ddagentversion,omitempty"`
}

type MetricSender struct {
	sender           aggregator.Sender
	hostname         string
	submittedMetrics int
}

func (c *Check) SampleSession() error {
	sessionSamples := []OracleActivityRow{}
	// err := c.db.Select(&sessionSamples, ACTIVITY_QUERY)

	err := c.db.SelectContext(
		godror.ContextWithTraceTag(context.Background(), godror.TraceTag{
			Module: "datadog agent",
			Action: "session sampling",
		}), &sessionSamples, ACTIVITY_QUERY)

	if err != nil {
		log.Errorf("Session sampling ", err)
		return err
	}
	//log.Tracef("orasample %#v", sessionSamples)

	payload := ActivitySnapshot{
		Metadata: Metadata{
			Timestamp:      float64(time.Now().UnixMilli()),
			Host:           c.hostname,
			Source:         common.IntegrationName,
			DBMType:        "activity",
			DDAgentVersion: c.agentVersion,
		},
		CollectionInterval: c.checkInterval,
		Tags:               []string{},
		OracleActivityRows: sessionSamples,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Error("Error marshalling device metadata: %s", err)
		return err
	}
	fmt.Println("JSON payload", string(payloadBytes))

	sender, err := c.GetSender()
	if err != nil {
		log.Tracef("GetSender SampleSession ", string(payloadBytes))
		return err
	}
	sender.EventPlatformEvent(string(payloadBytes), "dbm-activity")
	sender.Commit()
	return nil
}
