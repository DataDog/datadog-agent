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

const PGA_QUERY = `SELECT 
	name pdb_name, 
	pid, 
	program, 
	nvl(pga_used_mem,0) pga_used_mem, 
	nvl(pga_alloc_mem,0) pga_alloc_mem, 
	nvl(pga_freeable_mem,0) pga_freeable_mem, 
	nvl(pga_max_mem,0) pga_max_mem
  FROM v$process p, v$containers c
  WHERE
  	c.con_id(+) = p.con_id`

type ProcessesRowDB struct {
	PdbName        sql.NullString `db:"PDB_NAME"`
	PID            uint64         `db:"PID"`
	Program        sql.NullString `db:"PROGRAM"`
	PGAUsedMem     float64        `db:"PGA_USED_MEM"`
	PGAAllocMem    float64        `db:"PGA_ALLOC_MEM"`
	PGAFreeableMem float64        `db:"PGA_FREEABLE_MEM"`
	PGAMaxMem      float64        `db:"PGA_MAX_MEM"`
}

func (c *Check) ProcessMemory() error {
	rows := []ProcessesRowDB{}
	err := selectWrapper(c, &rows, PGA_QUERY)
	if err != nil {
		return fmt.Errorf("failed to collect processes info: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	for _, r := range rows {
		tags := appendPDBTag(c.tags, r.PdbName)
		tags = append(tags, "pid:"+strconv.FormatUint(r.PID, 10))
		if r.Program.Valid {
			tags = append(tags, "program:"+r.Program.String)
		}
		sender.Gauge(fmt.Sprintf("%s.process.pga_used_memory", common.IntegrationName), r.PGAUsedMem, "", tags)
		sender.Gauge(fmt.Sprintf("%s.process.pga_allocated_memory", common.IntegrationName), r.PGAAllocMem, "", tags)
		sender.Gauge(fmt.Sprintf("%s.process.pga_freeable_memory", common.IntegrationName), r.PGAFreeableMem, "", tags)
		sender.Gauge(fmt.Sprintf("%s.process.pga_max_memory", common.IntegrationName), r.PGAMaxMem, "", tags)
	}
	sender.Commit()
	return nil
}
