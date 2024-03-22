// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
)

const asmDiskgroupQuery = `SELECT name, free_mb, total_mb, state, offline_disks FROM v$asm_diskgroup`

type asmDiskgroupRow struct {
	DiskgroupName string  `db:"NAME"`
	Free          float64 `db:"FREE_MB"`
	Total         float64 `db:"TOTAL_MB"`
	State         string  `db:"STATE"`
	OfflineDisks  int64   `db:"OFFLINE_DISKS"`
}

func (c *Check) asmDiskgroups() error {
	rows := []asmDiskgroupRow{}
	err := selectWrapper(c, &rows, asmDiskgroupQuery)
	if err != nil {
		return fmt.Errorf("failed to collect asm diskgroup info: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	for _, r := range rows {
		tags := append(c.tags, "asm_diskgroup_name:"+r.DiskgroupName)
		tags = append(tags, "state:"+r.State)
		sendMetric(c, gauge, fmt.Sprintf("%s.asm_diskgroup.free_mb", common.IntegrationName), r.Free, tags)
		sendMetric(c, gauge, fmt.Sprintf("%s.asm_diskgroup.total_mb", common.IntegrationName), r.Total, tags)
		sendMetric(c, gauge, fmt.Sprintf("%s.asm_diskgroup.offline_disks", common.IntegrationName), float64(r.OfflineDisks), tags)
	}
	sender.Commit()
	return nil
}
