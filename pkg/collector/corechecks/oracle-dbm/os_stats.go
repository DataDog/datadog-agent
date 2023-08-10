// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//revive:disable:var-naming

//go:build oracle

package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strings"
)

// OSSTATS_QUERY exported const should have comment or be unexported
// don't use ALL_CAPS in Go names; use CamelCase
const OSSTATS_QUERY = `SELECT stat_name, value
  FROM v$osstat WHERE stat_name in ('NUM_CPUS','PHYSICAL_MEMORY_BYTES')`

// OSStatsRowDB exported type should have comment or be unexported
type OSStatsRowDB struct {
	StatName string  `db:"STAT_NAME"`
	Value    float64 `db:"VALUE"`
}

// OS_Stats exported method should have comment or be unexported
// don't use underscores in Go names; method OS_Stats should be OSStats
func (c *Check) OS_Stats() error {
	s, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}

	OSStatsRows := []OSStatsRowDB{}
	err = selectWrapper(c, &OSStatsRows, OSSTATS_QUERY)
	if err != nil {
		return fmt.Errorf("failed to collect OS stats: %w", err)
	}

	var numCPUsFound bool
	for _, r := range OSStatsRows {
		var name string
		var value float64
		if r.StatName == "PHYSICAL_MEMORY_BYTES" {
			name = "physical_memory_gb"
			value = r.Value / 1024 / 1024 / 1024
		} else {
			name = strings.ToLower(r.StatName)
			value = r.Value
		}
		if r.StatName == "NUM_CPUS" {
			numCPUsFound = true
		}
		s.Gauge(fmt.Sprintf("%s.%s", common.IntegrationName, name), value, "", c.tags)
	}

	var cpuCount float64
	if !numCPUsFound {
		if err := c.db.Get(&cpuCount, "SELECT value FROM v$parameter WHERE name = 'cpu_count'"); err == nil {
			s.Gauge(fmt.Sprintf("%s.num_cpus", common.IntegrationName), cpuCount, "", c.tags)
		} else {
			log.Errorf("failed to get cpu_count: %s", err)
		}
	}

	s.Commit()
	return nil
}
