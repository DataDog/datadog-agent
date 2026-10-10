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

const userSessionsMetricName = common.IntegrationName + ".user_sessions"

const sessionCountQuery = `SELECT
	count(*) as count
FROM v$session
WHERE type != 'BACKGROUND'`

// UserSessionsCount collects the number of non-background sessions.
func (c *Check) UserSessionsCount() error {
	if c.legacyIntegrationCompatibilityMode {
		return nil
	}
	sender, err := c.GetSender()
	if err != nil {
		return fmt.Errorf("failed to initialize sender: %w", err)
	}
	var sessionCount int
	if err := getWrapper(c, &sessionCount, sessionCountQuery); err != nil {
		return fmt.Errorf("failed to collect session count: %w", err)
	}
	sendMetric(c, gauge, userSessionsMetricName, float64(sessionCount), c.tags)
	sender.Commit()
	return nil
}
