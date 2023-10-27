// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
)

const resourceManagerQuery = `SELECT c.name name, consumer_group_name, plan_name, cpu_consumed_time, cpu_wait_time 
FROM v$rsrcmgrmetric r, v$containers c
WHERE c.con_id(+) = r.con_id`

type resourceManagerRow struct {
	PdbName           sql.NullString `db:"NAME"`
	ConsumerGroupName string         `db:"CONSUMER_GROUP_NAME"`
	PlanName          string         `db:"PLAN_NAME"`
	CPUConsumedTime   float64        `db:"CPU_CONSUMED_TIME"`
	CPUWaitTime       float64        `db:"CPU_WAIT_TIME"`
}

func (c *Check) resourceManager() error {
	rows := []resourceManagerRow{}
	err := selectWrapper(c, &rows, resourceManagerQuery)
	if err != nil {
		return fmt.Errorf("failed to collect resource manager statistics: %w", err)
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	for _, r := range rows {
		tags := appendPDBTag(c.tags, r.PdbName)
		tags = append(tags, "consumer_group_name:"+r.ConsumerGroupName)
		tags = append(tags, "plan_name:"+r.ConsumerGroupName)
		sender.Gauge(fmt.Sprintf("%s.resource_manager.cpu_consumed_time", common.IntegrationName), r.CPUConsumedTime, "", tags)
		sender.Gauge(fmt.Sprintf("%s.resource_manager.cpu_wait_time", common.IntegrationName), r.CPUWaitTime, "", tags)
	}
	sender.Commit()
	return nil
}
