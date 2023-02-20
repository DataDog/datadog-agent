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
	AND NOT (state = 'WAITING' and wait_class = 'Idle')`

type OracleActivityRow struct {
	SessionID             int64   `db:"SID" json:"sid,omitempty"`
	SessionSerial         int64   `db:"SERIAL#" json:"serial,omitempty"`
	Username              *string `db:"USERNAME" json:"username,omitempty"`
	OsUser                *string `db:"OSUSER" json:"os_user,omitempty"`
	Process               *string `db:"PROCESS" json:"process,omitempty"`
	Machine               *string `db:"MACHINE" json:"machine,omitempty"`
	Program               *string `db:"PROGRAM" json:"program,omitempty"`
	Type                  *string `db:"TYPE" json:"type,omitempty"`
	SqlID                 *string `db:"SQL_ID" json:"sql_id,omitempty"`
	SqlPlanHashValue      *int64  `db:"SQL_PLAN_HASH_VALUE" json:"sql_plan_hash_value,omitempty"`
	SqlExecStart          *string `db:"SQL_EXEC_START" json:"sql_exec_start,omitempty"`
	Module                *string `db:"MODULE" json:"module,omitempty"`
	Action                *string `db:"ACTION" json:"action,omitempty"`
	ClientInfo            *string `db:"CLIENT_INFO" json:"client_info,omitempty"`
	LogonTime             *string `db:"LOGON_TIME" json:"logon_time,omitempty"`
	ClientIdentifier      *string `db:"CLIENT_IDENTIFIER" json:"client_identifier,omitempty"`
	BlockingInstance      *int64  `db:"BLOCKING_INSTANCE" json:"blocking_instance,omitempty"`
	BlockingSession       *int64  `db:"BLOCKING_SESSION" json:"blocking_session,omitempty"`
	FinalBlockingInstance *int64  `db:"FINAL_BLOCKING_INSTANCE" json:"final_blocking_instance,omitempty"`
	FinalBlockingSession  *int64  `db:"FINAL_BLOCKING_SESSION" json:"final_blocking_session,omitempty"`
	Event                 *string `db:"EVENT" json:"event,omitempty"`
	WaitClass             *string `db:"WAIT_CLASS" json:"wait_class,omitempty"`
	SqlText               *string `db:"SQL_TEXT" json:"sql_text,omitempty"`
	PdbName               *string `db:"PDB_NAME" json:"pdb_name,omitempty"`
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
