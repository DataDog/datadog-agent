// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// 200 in SUBSTR is maximum tag size, see https://docs.datadoghq.com/getting_started/tagging/#define-tags
const locksQuery122 = `SELECT
  MAX((SYSDATE - start_date)*24*3600) as seconds,
  s.sid, s.username, s.osuser, s.machine, s.program,
  s.status,
  SUBSTR(LISTAGG(
							d.owner || '.' || d.object_name || ':' ||
							Decode(l.locked_mode, 0, 'None',
							1, 'Null/NULL/',
							2, 'Row-S/SS/',
							3, 'Row-X/SX/',
							4, 'Share/S/',
							5, 'S/Row-X/SSX/',
							6, 'Exclusive/X/',
							l.locked_mode)
							, ':' on overflow truncate) WITHIN GROUP (ORDER BY d.owner, d.object_name),1,200) object,
  c.name as pdb_name
	FROM v$transaction t, v$session s, v$locked_object l, dba_objects d, v$containers c
	WHERE s.saddr = t.ses_addr
		AND l.object_id = d.object_id AND l.session_id = s.sid AND l.con_id(+) = c.con_id AND temporary = 'N'
	GROUP BY s.sid, s.username, s.osuser, s.machine, s.program,
		s.status, c.name`

type locksRowDB struct {
	SID      sql.NullInt64   `db:"SID"`
	Username sql.NullString  `db:"USERNAME"`
	Program  sql.NullString  `db:"PROGRAM"`
	Machine  sql.NullString  `db:"MACHINE"`
	OsUser   sql.NullString  `db:"OSUSER"`
	Status   sql.NullString  `db:"STATUS"`
	Object   sql.NullString  `db:"OBJECT"`
	PDBName  sql.NullString  `db:"PDB_NAME"`
	Seconds  sql.NullFloat64 `db:"SECONDS"`
}

type oracleLockRow struct {
	SessionID string `json:"session_id,omitempty"`
	Username  string `json:"username,omitempty"`
	Program   string `json:"program,omitempty"`
	Machine   string `json:"machine,omitempty"`
	OsUser    string `json:"os_user,omitempty"`
	Status    string `json:"status,omitempty"`
	// Format: owner.object:lock_mode:
	Object               string  `json:"object,omitempty"`
	PDBName              string  `json:"pdb_name,omitempty"`
	SecondsInTransaction float64 `json:"seconds_in_transaction,omitempty"`
}

type metricsPayload struct {
	Host                  string   `json:"host,omitempty"` // Host is the database hostname, not the agent hostname
	Kind                  string   `json:"kind,omitempty"`
	Timestamp             float64  `json:"timestamp,omitempty"`
	MinCollectionInterval float64  `json:"min_collection_interval,omitempty"`
	Tags                  []string `json:"tags,omitempty"`
	AgentVersion          string   `json:"ddagentversion,omitempty"`
	AgentHostname         string   `json:"ddagenthostname,omitempty"`
	OracleVersion         string   `json:"oracle_version,omitempty"`
}

type lockMetricsPayload struct {
	metricsPayload
	OracleRows []oracleLockRow `json:"oracle_rows,omitempty"`
}

func (c *Check) locks() error {
	if isDbVersionLessThan(c, "12.2") {
		return nil
	}
	rows := []locksRowDB{}

	query := locksQuery122
	err := selectWrapper(c, &rows, query)
	if err != nil {
		return fmt.Errorf("failed to collect locks info: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	var oracleRows []oracleLockRow
	for _, r := range rows {
		if !r.Seconds.Valid {
			continue
		}
		var p oracleLockRow
		p.SecondsInTransaction = r.Seconds.Float64
		if r.PDBName.Valid {
			p.PDBName = fmt.Sprintf("%s.%s", c.cdbName, r.PDBName.String)
		}
		if r.SID.Valid {
			p.SessionID = strconv.FormatInt(r.SID.Int64, 10)
		}
		if r.Username.Valid {
			p.Username = r.Username.String
		}
		if r.Program.Valid {
			p.Program = r.Program.String
		}
		if r.Machine.Valid {
			p.Machine = r.Machine.String
		}
		if r.OsUser.Valid {
			p.OsUser = r.OsUser.String
		}
		if r.Status.Valid {
			p.Status = r.OsUser.String
		}
		if r.Object.Valid {
			p.Object = r.Object.String
		}
		oracleRows = append(oracleRows, p)
	}
	hname, _ := hostname.Get(context.TODO())
	m := metricsPayload{
		Host:                  c.dbHostname,
		Kind:                  "lock_metrics",
		Timestamp:             float64(time.Now().UnixMilli()),
		MinCollectionInterval: float64(c.config.MinCollectionInterval),
		Tags:                  c.tags,
		AgentVersion:          c.agentVersion,
		AgentHostname:         hname,
		OracleVersion:         c.dbVersion,
	}
	payload := lockMetricsPayload{
		OracleRows: oracleRows,
	}
	payload.metricsPayload = m

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal lock metrics payload: %w", err)
	}

	sender.EventPlatformEvent(payloadBytes, "dbm-metrics")
	log.Debugf("%s lock metrics payload %s", c.logPrompt, strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender.Commit()
	return nil
}
