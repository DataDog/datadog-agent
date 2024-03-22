// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

const tablespaceQuery12 = `SELECT
  c.name pdb_name,
  t.tablespace_name tablespace_name,
  NVL(m.used_space * t.block_size, 0) used,
  NVL(m.tablespace_size * t.block_size, 0) size_,
  NVL(m.used_percent, 0) in_use,
  NVL2(m.used_space, 0, 1) offline_
FROM
  cdb_tablespace_usage_metrics m, cdb_tablespaces t, v$containers c
WHERE
  m.con_id = t.con_id and m.tablespace_name(+) = t.tablespace_name and c.con_id(+) = t.con_id`

const tablespaceQuery11 = `SELECT
  t.tablespace_name tablespace_name,
  NVL(m.used_space * t.block_size, 0) used,
  NVL(m.tablespace_size * t.block_size, 0) size_,
  NVL(m.used_percent, 0) in_use,
  NVL2(m.used_space, 0, 1) offline_
FROM
  dba_tablespace_usage_metrics m, dba_tablespaces t
WHERE
  m.tablespace_name(+) = t.tablespace_name`

const (
	maxSizeQuery12 = `SELECT
  c.name pdb_name,
  f.tablespace_name tablespace_name,
  SUM(CASE WHEN autoextensible = 'YES' THEN maxbytes ELSE bytes END) maxsize
FROM cdb_data_files f, v$containers c
WHERE c.con_id(+) = f.con_id
GROUP BY c.name, f.tablespace_name`

	maxSizeQuery11 = `SELECT
	f.tablespace_name tablespace_name,
	SUM(CASE WHEN autoextensible = 'YES' THEN maxbytes ELSE bytes END) maxsize
	FROM dba_data_files f
	GROUP BY f.tablespace_name`
)

//nolint:revive // TODO(DBM) Fix revive linter
type RowDB struct {
	PdbName        sql.NullString `db:"PDB_NAME"`
	TablespaceName string         `db:"TABLESPACE_NAME"`
	Used           float64        `db:"USED"`
	Size           float64        `db:"SIZE_"`
	InUse          float64        `db:"IN_USE"`
	Offline        float64        `db:"OFFLINE_"`
}

type rowMaxSizeDB struct {
	PdbName        sql.NullString `db:"PDB_NAME"`
	TablespaceName string         `db:"TABLESPACE_NAME"`
	MaxSize        float64        `db:"MAXSIZE"`
}

//nolint:revive // TODO(DBM) Fix revive linter
func (c *Check) Tablespaces() error {
	rows := []RowDB{}
	var tablespaceQuery, maxSizeQuery string
	if c.legacyIntegrationCompatibilityMode {
		tablespaceQuery = tablespaceQuery11
		maxSizeQuery = maxSizeQuery11
	} else {
		if isDbVersionGreaterOrEqualThan(c, minMultitenantVersion) {
			tablespaceQuery = tablespaceQuery12
			maxSizeQuery = maxSizeQuery12
		} else {
			tablespaceQuery = tablespaceQuery11
			maxSizeQuery = maxSizeQuery11
		}
	}
	err := selectWrapper(c, &rows, tablespaceQuery)
	if err != nil {
		return fmt.Errorf("failed to collect tablespace info: %w", err)
	}

	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	for _, r := range rows {
		tags := appendPDBTag(c.tags, r.PdbName)
		tags = append(tags, "tablespace:"+r.TablespaceName)
		sendMetric(c, gauge, fmt.Sprintf("%s.tablespace.used", common.IntegrationName), r.Used, tags)
		sendMetric(c, gauge, fmt.Sprintf("%s.tablespace.size", common.IntegrationName), r.Size, tags)
		sendMetric(c, gauge, fmt.Sprintf("%s.tablespace.in_use", common.IntegrationName), r.InUse, tags)
		sendMetric(c, gauge, fmt.Sprintf("%s.tablespace.offline", common.IntegrationName), r.Offline, tags)
	}

	rowsMaxSize := []rowMaxSizeDB{}
	err = selectWrapper(c, &rowsMaxSize, maxSizeQuery)
	if err != nil {
		return fmt.Errorf("failed to collect max size tablespace info: %w", err)
	}

	for _, r := range rowsMaxSize {
		tags := appendPDBTag(c.tags, r.PdbName)
		tags = append(tags, "tablespace:"+r.TablespaceName)
		sendMetric(c, gauge, fmt.Sprintf("%s.tablespace.maxsize", common.IntegrationName), r.MaxSize, tags)
	}

	sender.Commit()
	return nil
}
