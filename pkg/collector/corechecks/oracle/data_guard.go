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

const invalidLagValue = 10000000

type dataguardStats struct {
	Name  string          `db:"NAME"`
	Value sql.NullFloat64 `db:"VALUE"`
}

func (c *Check) dataGuard() error {
	if c.hostingType != selfManaged {
		return nil
	}
	var n uint16
	err := getWrapper(c, &n, "SELECT 1 n FROM v$dataguard_config WHERE ROWNUM = 1")
	if err != nil {
		return fmt.Errorf("%s failed to query v$dataguard_config %w", c.logPrompt, err)
	}
	if n != 1 {
		return nil
	}
	var d vDatabase
	err = getWrapper(c, &d, "SELECT database_role, open_mode FROM v$database")
	if err != nil {
		return fmt.Errorf("%s failed to query database role %w", c.logPrompt, err)
	}
	c.databaseRole = d.DatabaseRole
	c.openMode = d.OpenMode

	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	var stats []dataguardStats
	err = selectWrapper(c, &stats, `SELECT name, EXTRACT(DAY from TO_DSINTERVAL(value)*86400) value
	FROM v$dataguard_stats WHERE name IN ('apply lag','transport lag')`)
	if err != nil {
		return fmt.Errorf("failed to query data guard statistics %w", err)
	}
	for _, s := range stats {
		var v float64
		if s.Value.Valid {
			v = s.Value.Float64
		} else {
			v = invalidLagValue
		}
		sendMetricWithDefaultTags(c, gauge, fmt.Sprintf("%s.data_guard.%s", common.IntegrationName, strings.ReplaceAll(string(s.Name), " ", "_")), v)
	}
	sender.Commit()
	return nil
}
