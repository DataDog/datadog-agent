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
	Seconds  sql.NullFloat64 `db:"SECONDS"`
	Sid      sql.NullInt64   `db:"SID"`
	Username sql.NullString  `db:"USERNAME"`
	Program  sql.NullString  `db:"PROGRAM"`
	Machine  sql.NullString  `db:"MACHINE"`
	OsUser   sql.NullString  `db:"OSUSER"`
	Status   sql.NullString  `db:"STATUS"`
	Object   sql.NullString  `db:"OBJECT"`
	PdbName  sql.NullString  `db:"PDB_NAME"`
}

func (c *Check) locks() error {
	if isDbVersionLessThan("12.2") {
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
	for _, r := range rows {
		if !r.Seconds.Valid {
			continue
		}
		tags := appendPDBTag(c.tags, r.PdbName)
		if r.Sid.Valid {
			tags = append(tags, "sid:"+strconv.FormatInt(r.Sid.Int64, 10))
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
