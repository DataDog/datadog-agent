// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

const shmQuery12 = `SELECT
    c.name pdb_name, s.name, ROUND(bytes/1024/1024,2) size_
  FROM v$sgainfo s, v$containers c
  WHERE
    c.con_id(+) = s.con_id
    AND s.name NOT IN ('Maximum SGA Size','Startup overhead in Shared Pool','Granule Size','Shared IO Pool Size')`

const shmQuery11 = `SELECT
	s.name, ROUND(bytes/1024/1024,2) size_
FROM v$sgainfo s
WHERE
  s.name NOT IN ('Maximum SGA Size','Startup overhead in Shared Pool','Granule Size','Shared IO Pool Size')`

//nolint:revive // TODO(DBM) Fix revive linter
type SHMRow struct {
	PdbName sql.NullString `db:"PDB_NAME"`
	Memory  string         `db:"NAME"`
	Size    float64        `db:"SIZE_"`
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) SharedMemory() error {
	rows := []SHMRow{}
	var shmQuery string
	if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
		shmQuery = shmQuery12
	} else {
		shmQuery = shmQuery11
	}
	err := c.db.Select(&rows, shmQuery)
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
		sendMetric(c, gauge, fmt.Sprintf("%s.shared_memory.size", common.IntegrationName), r.Size, tags)
	}
	sender.Commit()
	return nil
}
