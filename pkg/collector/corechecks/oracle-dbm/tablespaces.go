// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"database/sql"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
)

const QUERY = `SELECT
  c.name pdb_name,
  t.tablespace_name tablespace_name,
  NVL(m.used_space * t.block_size, 0) used,
  NVL(m.tablespace_size * t.block_size, 0) size_,
  NVL(m.used_percent, 0) in_use,
  NVL2(m.used_space, 0, 1) offline_
FROM
  cdb_tablespace_usage_metrics m, cdb_tablespaces t, v$containers c
WHERE
  m.tablespace_name(+) = t.tablespace_name and c.con_id(+) = t.con_id`

type RowDB struct {
	PdbName        sql.NullString `db:"PDB_NAME"`
	TablespaceName string         `db:"TABLESPACE_NAME"`
	Used           float64        `db:"USED"`
	Size           float64        `db:"SIZE_"`
	InUse          float64        `db:"IN_USE"`
	Offline        float64        `db:"OFFLINE_"`
}

func (c *Check) Tablespaces() error {
	rows := []RowDB{}
	err := selectWrapper(c, &rows, QUERY)
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
		sender.Gauge(fmt.Sprintf("%s.tablespace.used", common.IntegrationName), r.Used, "", tags)
		sender.Gauge(fmt.Sprintf("%s.tablespace.size", common.IntegrationName), r.Size, "", tags)
		sender.Gauge(fmt.Sprintf("%s.tablespace.in_use", common.IntegrationName), r.InUse, "", tags)
		sender.Gauge(fmt.Sprintf("%s.tablespace.offline", common.IntegrationName), r.Offline, "", tags)
	}
	sender.Commit()
	return nil
}
