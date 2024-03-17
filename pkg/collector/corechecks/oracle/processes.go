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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

const pgaQuery12 = `SELECT
	c.name as pdb_name,
	p.pid as pid, p.program as server_process,
	s.sid as sid, s.username as username, s.program as program, s.machine as machine, s.osuser as osuser,
	s.status as status, last_call_et,
	module, client_info,
	nvl(pga_used_mem,0) pga_used_mem,
	nvl(pga_alloc_mem,0) pga_alloc_mem,
	nvl(pga_freeable_mem,0) pga_freeable_mem,
	nvl(pga_max_mem,0) pga_max_mem
FROM v$process p, v$containers c, v$session s
WHERE
  c.con_id(+) = p.con_id
	AND s.paddr(+) = p.addr`

const pgaQuery11 = `SELECT
	p.pid as pid, p.program as server_process,
	s.sid as sid, s.username as username, s.program as program, s.machine as machine, s.osuser as osuser,
	s.status as status, last_call_et,
	nvl(pga_used_mem,0) pga_used_mem,
	nvl(pga_alloc_mem,0) pga_alloc_mem,
	nvl(pga_freeable_mem,0) pga_freeable_mem,
	nvl(pga_max_mem,0) pga_max_mem
FROM v$process p, v$session s
WHERE s.paddr(+) = p.addr`

const pgaQueryOldIntegration = `SELECT
	p.pid as pid, p.program as server_process,
	nvl(pga_used_mem,0) pga_used_mem,
	nvl(pga_alloc_mem,0) pga_alloc_mem,
	nvl(pga_freeable_mem,0) pga_freeable_mem,
	nvl(pga_max_mem,0) pga_max_mem
FROM gv$process p`

type sessionTagColumns struct {
	Sid      sql.NullInt64  `db:"SID"`
	Username sql.NullString `db:"USERNAME"`
	Program  sql.NullString `db:"PROGRAM"`
	Machine  sql.NullString `db:"MACHINE"`
	OsUser   sql.NullString `db:"OSUSER"`
}

//nolint:revive // TODO(DBM) Fix revive linter
type ProcessesRowDB struct {
	PdbName        sql.NullString `db:"PDB_NAME"`
	PID            uint64         `db:"PID"`
	ServerProcess  sql.NullString `db:"SERVER_PROCESS"`
	PGAUsedMem     float64        `db:"PGA_USED_MEM"`
	PGAAllocMem    float64        `db:"PGA_ALLOC_MEM"`
	PGAFreeableMem float64        `db:"PGA_FREEABLE_MEM"`
	PGAMaxMem      float64        `db:"PGA_MAX_MEM"`
	LastCallEt     sql.NullInt64  `db:"LAST_CALL_ET"`
	Status         sql.NullString `db:"STATUS"`
	Module         sql.NullString `db:"MODULE"`
	ClientInfo     sql.NullString `db:"CLIENT_INFO"`
	sessionTagColumns
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) ProcessMemory() error {
	rows := []ProcessesRowDB{}

	var pgaQuery string
	if c.legacyIntegrationCompatibilityMode {
		pgaQuery = pgaQueryOldIntegration
	} else {
		if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
			pgaQuery = pgaQuery12
		} else {
			pgaQuery = pgaQuery11
		}
	}

	err := selectWrapper(c, &rows, pgaQuery)
	if err != nil {
		return fmt.Errorf("failed to collect processes info: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	for _, r := range rows {
		tags := appendPDBTag(c.tags, r.PdbName)
		if r.Sid.Valid {
			tags = append(tags, "sid:"+strconv.FormatInt(r.Sid.Int64, 10))
		} else {
			tags = append(tags, "pid:"+strconv.FormatUint(r.PID, 10))
		}
		if r.Username.Valid {
			tags = append(tags, "username:"+r.Username.String)
		}
		if r.Program.Valid {
			tags = append(tags, "program:"+r.Program.String)
		} else if r.ServerProcess.Valid {
			tags = append(tags, "program:"+r.ServerProcess.String)
		}
		if r.Machine.Valid {
			tags = append(tags, "machine:"+r.Machine.String)
		}
		if r.OsUser.Valid {
			tags = append(tags, "osuser:"+r.OsUser.String)
		}
		if c.config.ProcessMemory.Enabled {
			sendMetric(c, gauge, fmt.Sprintf("%s.process.pga_used_memory", common.IntegrationName), r.PGAUsedMem, tags)
			sendMetric(c, gauge, fmt.Sprintf("%s.process.pga_allocated_memory", common.IntegrationName), r.PGAAllocMem, tags)
			sendMetric(c, gauge, fmt.Sprintf("%s.process.pga_freeable_memory", common.IntegrationName), r.PGAFreeableMem, tags)
			sendMetric(c, gauge, fmt.Sprintf("%s.process.pga_max_memory", common.IntegrationName), r.PGAMaxMem, tags)
			// we send pga_maximum_memory for backward compatibility with the old Oracle integration
			sendMetric(c, gauge, fmt.Sprintf("%s.process.pga_maximum_memory", common.IntegrationName), r.PGAMaxMem, tags)
		}

		if c.config.InactiveSessions.Enabled && r.Status.Valid && r.Status.String == "INACTIVE" && r.LastCallEt.Valid {
			if r.Module.Valid {
				tags = append(tags, "module:"+r.Module.String)
			}
			if r.ClientInfo.Valid {
				tags = append(tags, "client_info:"+r.ClientInfo.String)
			}
			sendMetric(c, gauge, fmt.Sprintf("%s.session.inactive_seconds", common.IntegrationName), float64(r.LastCallEt.Int64), tags)
		}
	}
	sender.Commit()
	return nil
}
