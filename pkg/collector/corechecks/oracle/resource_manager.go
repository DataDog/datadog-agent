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

const resourceManagerQuery = `SELECT c.name name, consumer_group_name, plan_name, cpu_consumed_time, cpu_wait_time
FROM v$rsrcmgrmetric r, v$containers c
WHERE c.con_id(+) = r.con_id`

const resourceManagerQueryNonCdb = `SELECT consumer_group_name, plan_name, cpu_consumed_time, cpu_wait_time
FROM v$rsrcmgrmetric r`

const resourceManagerQuery11 = `SELECT consumer_group_name, cpu_consumed_time, cpu_wait_time
FROM v$rsrcmgrmetric r`

type resourceManagerRow struct {
	PdbName           sql.NullString `db:"NAME"`
	ConsumerGroupName string         `db:"CONSUMER_GROUP_NAME"`
	PlanName          sql.NullString `db:"PLAN_NAME"`
	CPUConsumedTime   float64        `db:"CPU_CONSUMED_TIME"`
	CPUWaitTime       float64        `db:"CPU_WAIT_TIME"`
}

func (c *Check) resourceManager() error {
	rows := []resourceManagerRow{}
	var q string
	if c.multitenant {
		q = resourceManagerQuery
	} else {
		if isDbVersionGreaterOrEqualThan(c, "12") {
			q = resourceManagerQueryNonCdb
		} else {
			q = resourceManagerQuery11
		}
	}
	err := selectWrapper(c, &rows, q)
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
		if r.PlanName.Valid && r.PlanName.String != "" {
			tags = append(tags, "plan_name:"+r.PlanName.String)
		}
		sendMetric(c, gauge, fmt.Sprintf("%s.resource_manager.cpu_consumed_time", common.IntegrationName), r.CPUConsumedTime, tags)
		sendMetric(c, gauge, fmt.Sprintf("%s.resource_manager.cpu_wait_time", common.IntegrationName), r.CPUWaitTime, tags)
	}
	sender.Commit()
	return nil
}
