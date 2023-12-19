// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
)

const locksQuery12 = `SELECT 
  MAX((SYSDATE - start_date)*24*3600) as seconds, 
	s.sid,s.serial#, s.username, s.osuser, s.machine, s.program,
  s.status, 
	LISTAGG(d.owner || '.' || d.object_name, ',') WITHIN GROUP (ORDER BY d.owner, d.object_name) object, 
	c.name as pdb_name
    FROM v$transaction t, v$session s, v$locked_object v, dba_objects d, v$containers c
    WHERE s.saddr = t.ses_addr 
      AND v.object_id = d.object_id AND v.session_id = s.sid AND v.con_id(+) = c.con_id AND temporary = 'N'
		GROUP BY s.sid,s.serial#, s.username, s.osuser, s.machine, s.program,
			s.status, c.name`

const locksQuery11 = `SELECT 
  MAX((SYSDATE - start_date)*24*3600) as seconds, s.sid,s.serial#, s.username, s.osuser, s.machine, s.program,
  s.status, 
	LISTAGG(d.owner || '.' || d.object_name, ',') WITHIN GROUP (ORDER BY d.owner, d.object_name) object
  FROM v$transaction t, v$session s, v$locked_object v, dba_objects d
  WHERE s.saddr = t.ses_addr 
    AND v.object_id = d.object_id AND v.session_id = s.sid AND temporary = 'N'
	GROUP BY s.sid,s.serial#, s.username, s.osuser, s.machine, s.program,
		s.status`

type locksRowDB struct {
	Seconds  sql.NullFloat64 `db:"SECONDS"`
	Sid      sql.NullInt64   `db:"SID"`
	Serial   sql.NullInt64   `db:"SERIAL#"`
	Username sql.NullString  `db:"USERNAME"`
	Program  sql.NullString  `db:"PROGRAM"`
	Machine  sql.NullString  `db:"MACHINE"`
	OsUser   sql.NullString  `db:"OSUSER"`
	Status   sql.NullString  `db:"STATUS"`
	Object   sql.NullString  `db:"OBJECT"`
	PdbName  sql.NullString  `db:"PDB_NAME"`
}

func (c *Check) locks() error {
	rows := []locksRowDB{}

	var query string
	if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
		query = locksQuery12
	} else {
		query = locksQuery11
	}
	err := selectWrapper(c, &rows, query)
	if err != nil {
		return fmt.Errorf("failed to collect locks info: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	for _, r := range rows {
		if !r.Seconds.Valid {
			continue
		}
		tags := appendPDBTag(c.tags, r.PdbName)
		if r.Sid.Valid {
			tags = append(tags, "sid:"+strconv.FormatInt(r.Sid.Int64, 10))
		}
		if r.Serial.Valid {
			tags = append(tags, "serial:"+strconv.FormatInt(r.Serial.Int64, 10))
		}
		if r.Username.Valid {
			tags = append(tags, "username:"+r.Username.String)
		}
		if r.Program.Valid {
			tags = append(tags, "program:"+r.Program.String)
		}
		if r.Machine.Valid {
			tags = append(tags, "machine:"+r.Machine.String)
		}
		if r.OsUser.Valid {
			tags = append(tags, "osuser:"+r.OsUser.String)
		}
		if r.Status.Valid {
			tags = append(tags, "status:"+r.Status.String)
		}
		if r.Object.Valid {
			tags = append(tags, "objects:"+r.Object.String)
		}

		sender.Gauge(fmt.Sprintf("%s.seconds_in_transaction", common.IntegrationName), r.Seconds.Float64, "", tags)
	}
	sender.Commit()
	return nil
}
