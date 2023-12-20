// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	locksQuery12 = `SELECT
  (SYSDATE - start_date)*24*3600 as seconds,
	s.sid,s.serial#, s.username, s.osuser, s.machine, s.program, 
	s.client_info, s.module, s.action, s.client_identifier,
  s.status,
	d.owner,
	d.object_name,
	v.locked_mode,
	c.name as pdb_name
  FROM v$transaction t, v$session s, v$locked_object v, dba_objects d, v$containers c
  WHERE s.saddr = t.ses_addr
    AND v.object_id = d.object_id AND v.session_id = s.sid AND v.con_id(+) = c.con_id AND temporary = 'N'`

	locksQuery11 = `SELECT
  (SYSDATE - start_date)*24*3600 as seconds, 
	s.sid,s.serial#, s.username, s.osuser, s.machine, s.program,
  s.status,
	d.owner,
	d.object_name,
	v.locked_mode
  FROM v$transaction t, v$session s, v$locked_object v, dba_objects d
  WHERE s.saddr = t.ses_addr
    AND v.object_id = d.object_id AND v.session_id = s.sid AND temporary = 'N'
	GROUP BY s.sid,s.serial#, s.username, s.osuser, s.machine, s.program,
		s.status`
)

type lockRowDB struct {
	Seconds          sql.NullFloat64 `db:"SECONDS"`
	Sid              sql.NullInt64   `db:"SID"`
	Serial           sql.NullInt64   `db:"SERIAL#"`
	Username         sql.NullString  `db:"USERNAME"`
	Program          sql.NullString  `db:"PROGRAM"`
	Machine          sql.NullString  `db:"MACHINE"`
	OsUser           sql.NullString  `db:"OSUSER"`
	Module           sql.NullString  `db:"MODULE"`
	Action           sql.NullString  `db:"ACTION"`
	ClientInfo       sql.NullString  `db:"CLIENT_INFO"`
	ClientIdentifier sql.NullString  `db:"CLIENT_IDENTIFIER"`
	Status           sql.NullString  `db:"STATUS"`
	Owner            sql.NullString  `db:"OWNER"`
	ObjectName       sql.NullString  `db:"OBJECT_NAME"`
	LockedMode       sql.NullString  `db:"LOCKED_MODE"`
	PdbName          sql.NullString  `db:"PDB_NAME"`
}

type lockRowPayload struct {
	Seconds          float64 `json:"seconds,omitempty"`
	Sid              int64   `json:"sid,omitempty"`
	Serial           int64   `json:"serial,omitempty"`
	Username         string  `json:"username,omitempty"`
	Program          string  `json:"program,omitempty"`
	Machine          string  `json:"machine,omitempty"`
	OsUser           string  `json:"os_user,omitempty"`
	Module           string  `json:"module,omitempty"`
	Action           string  `json:"action,omitempty"`
	ClientInfo       string  `json:"client_info,omitempty"`
	ClientIdentifier string  `json:"client_identifier,omitempty"`
	Status           string  `json:"status,omitempty"`
	Owner            string  `json:"owner,omitempty"`
	ObjectName       string  `json:"object_name,omitempty"`
	LockedMode       string  `json:"locked_mode,omitempty"`
	PdbName          string  `json:"pdb_name,omitempty"`
}

type locksPayload struct {
	Metadata
	// Tags should be part of the common Metadata struct but because Activity payloads use a string array
	// and samples use a comma-delimited list of tags in a single string, both flavors have to be handled differently
	Tags               []string         `json:"ddtags,omitempty"`
	CollectionInterval float64          `json:"collection_interval,omitempty"`
	LockActivityRows   []lockRowPayload `json:"locks,omitempty"`
}

func (c *Check) locks() error {
	var query string
	if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
		query = locksQuery12
	} else {
		query = locksQuery11
	}
	var rowsDB []lockRowDB
	err := selectWrapper(c, &rowsDB, query)
	if err != nil {
		return fmt.Errorf("failed to collect locks info: %w", err)
	}
	var locksRowsPayload []lockRowPayload
	for _, rowDB := range rowsDB {
		var rowPayload lockRowPayload
		if rowDB.Seconds.Valid {
			rowPayload.Seconds = rowDB.Seconds.Float64
		}
		if rowDB.Sid.Valid {
			rowPayload.Sid = rowDB.Sid.Int64
		}
		if rowDB.Serial.Valid {
			rowPayload.Serial = rowDB.Serial.Int64
		}
		if rowDB.Username.Valid {
			rowPayload.Username = rowDB.Username.String
		}
		if rowDB.Program.Valid {
			rowPayload.Program = rowDB.Program.String
		}
		if rowDB.Machine.Valid {
			rowPayload.Machine = rowDB.Machine.String
		}
		if rowDB.OsUser.Valid {
			rowPayload.OsUser = rowDB.OsUser.String
		}
		if rowDB.Module.Valid {
			rowPayload.Module = rowDB.Module.String
		}
		if rowDB.Action.Valid {
			rowPayload.Action = rowDB.Action.String
		}
		if rowDB.ClientInfo.Valid {
			rowPayload.ClientInfo = rowDB.ClientInfo.String
		}
		if rowDB.ClientIdentifier.Valid {
			rowPayload.ClientIdentifier = rowDB.ClientIdentifier.String
		}
		if rowDB.Status.Valid {
			rowPayload.Status = rowDB.Status.String
		}
		if rowDB.Owner.Valid {
			rowPayload.Owner = rowDB.Owner.String
		}
		if rowDB.ObjectName.Valid {
			rowPayload.ObjectName = rowDB.ObjectName.String
		}
		if rowDB.LockedMode.Valid {
			rowPayload.LockedMode = rowDB.LockedMode.String
		}
		if rowDB.PdbName.Valid {
			rowPayload.PdbName = rowDB.PdbName.String
		}
		locksRowsPayload = append(locksRowsPayload, rowPayload)
	}
	payload := locksPayload{
		Metadata: Metadata{
			Timestamp:      float64(time.Now().UnixMilli()),
			Host:           c.dbHostname,
			Source:         common.IntegrationName,
			DBMType:        "locks",
			DDAgentVersion: c.agentVersion,
		},
		CollectionInterval: c.checkInterval,
		Tags:               c.tags,
		LockActivityRows:   locksRowsPayload,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("%s Error marshalling locks payload: %s", c.logPrompt, err)
		return err
	}

	log.Debugf("%s Locks payload %s", c.logPrompt, strings.ReplaceAll(string(payloadBytes), "@", "XX"))

	sender, err := c.GetSender()
	if err != nil {
		log.Errorf("%s GetSender SampleSession %s", c.logPrompt, string(payloadBytes))
		return err
	}
	sender.EventPlatformEvent(payloadBytes, "dbm-locks")

	sender.Commit()
	return nil
}
