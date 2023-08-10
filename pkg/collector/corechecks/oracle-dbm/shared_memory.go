// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
)

// QUERY_SHM exported const should have comment or be unexported
// don't use ALL_CAPS in Go names; use CamelCase
const QUERY_SHM = `SELECT 
    c.name pdb_name, s.name, ROUND(bytes/1024/1024,2) size_
  FROM v$sgainfo s, v$containers c
  WHERE 
    c.con_id(+) = s.con_id
    AND s.name NOT IN ('Maximum SGA Size','Startup overhead in Shared Pool','Granule Size','Shared IO Pool Size')
`

// SHMRow exported type should have comment or be unexported
type SHMRow struct {
	PdbName sql.NullString `db:"PDB_NAME"`
	Memory  string         `db:"NAME"`
	Size    float64        `db:"SIZE_"`
}

// SharedMemory exported method should have comment or be unexported
func (c *Check) SharedMemory() error {
	rows := []SHMRow{}
	err := c.db.Select(&rows, QUERY_SHM)
	if err != nil {
		return fmt.Errorf("failed to collect shared memory info: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	for _, r := range rows {
		tags := appendPDBTag(c.tags, r.PdbName)
		memoryTag := strings.ReplaceAll(r.Memory, " ", "_")
		memoryTag = strings.ToLower(memoryTag)
		memoryTag = strings.ReplaceAll(memoryTag, "_size", "")
		tags = append(tags, fmt.Sprintf("memory:%s", memoryTag))
		sender.Gauge(fmt.Sprintf("%s.shared_memory.size", common.IntegrationName), r.Size, "", tags)
	}
	sender.Commit()
	return nil
}
