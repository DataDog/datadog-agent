// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

func TestDataObservabilityAgainstDatabase(t *testing.T) {
	for _, oracleDriver := range []string{common.GoOra, common.Godror} {
		t.Run(oracleDriver, func(t *testing.T) {
			connection := getConnectData(t, useDefaultUser)
			connection.OracleClient = oracleDriver == common.Godror
			c, _ := newTestCheck(t, connection, "data_observability:\n  enabled: true", "")
			t.Cleanup(c.Teardown)
			var err error
			c.db, err = c.Connect()
			require.NoError(t, err)
			require.NoError(t, c.init())
			require.NoError(t, c.connectDataObservability(context.Background()))
			t.Cleanup(c.closeDataObservabilityConnection)
			initialContainer := c.dataObservabilityContainer
			containers, err := c.dataObservabilityContainers(context.Background())
			require.NoError(t, err)
			var pdb string
			for _, name := range containers {
				if name != initialContainer && name != "PDB$SEED" {
					pdb = name
					break
				}
			}
			require.NotEmpty(t, pdb, "test requires an accessible PDB beside the root container")
			queries := []config.DataObservabilityQuery{
				dataObservabilityTestQuery(c.getFullPDBName(initialContainer), dataObservabilityCurrentContainerQuery),
				dataObservabilityTestQuery(c.getFullPDBName(pdb), dataObservabilityCurrentContainerQuery),
				dataObservabilityTestQuery(c.getFullPDBName(pdb), "SELECT LEVEL AS N FROM dual CONNECT BY LEVEL <= 10001"),
				dataObservabilityTestQuery(c.getFullPDBName(pdb), "SELECT DD_MISSING_COLUMN FROM dual"),
				dataObservabilityTestQuery(c.getFullPDBName(pdb), "ALTER SESSION SET NLS_DATE_FORMAT = 'YYYY-MM-DD'"),
				dataObservabilityTestQuery(c.getFullPDBName(pdb), "SELECT 1 FROM dual"),
			}
			var results []dataObservabilityQueryResult
			c.dataObservabilityQueries = newDataObservabilityScheduledQueries(queries)
			c.dataObservabilityEmitter = func(due dataObservabilityDueQuery, result dataObservabilityQueryResult) {
				if due.scheduledQuery.query.Query == dataObservabilityCurrentContainerQuery {
					require.Nil(t, result.Error)
					require.Equal(t, [][]any{{containers[due.scheduledQuery.query.DBName]}}, result.Rows)
				}
				results = append(results, result)
			}
			require.NoError(t, c.collectDataObservability(context.Background()))
			require.Len(t, results, len(queries))
			assert.Equal(t, dataObservabilityMaxResultRows, results[2].RowCount)
			assert.Equal(t, "error", results[3].Status)
			require.NotNil(t, results[4].Error)
			assert.Contains(t, *results[4].Error, "only SELECT statements are supported")
			assert.Equal(t, "success", results[5].Status)
			var regularContainer string
			require.NoError(t, c.db.Get(&regularContainer, dataObservabilityCurrentContainerQuery))
			assert.Equal(t, initialContainer, regularContainer)

			t.Run("timeout", func(t *testing.T) {
				query := dataObservabilityTestQuery(c.getFullPDBName(pdb), "SELECT SUM(SQRT(LEVEL)) FROM dual CONNECT BY LEVEL <= 10000000")
				query.QueryTimeout = 100
				start := time.Now()
				result, err := c.executeDataObservabilityQuery(context.Background(), query, pdb)
				require.ErrorIs(t, err, context.DeadlineExceeded)
				assert.Equal(t, "error", result.Status)
				assert.Less(t, time.Since(start), 4*time.Second)
				c.closeDataObservabilityConnection()
				require.NoError(t, c.connectDataObservability(context.Background()))
				query = dataObservabilityTestQuery(c.getFullPDBName(pdb), "SELECT 1 FROM dual")
				result, err = c.executeDataObservabilityQuery(context.Background(), query, pdb)
				require.NoError(t, err)
				assert.Equal(t, "success", result.Status)
			})

			t.Run("direct_pdb", func(t *testing.T) {
				connection.ServiceName = pdb
				direct, _ := newTestCheck(t, connection, "data_observability:\n  enabled: true", "")
				t.Cleanup(direct.Teardown)
				direct.db, err = direct.Connect()
				require.NoError(t, err)
				require.NoError(t, direct.init())
				direct.dataObservabilityQueries = newDataObservabilityScheduledQueries(queries[:2])
				t.Cleanup(direct.closeDataObservabilityConnection)
				var statuses []string
				direct.dataObservabilityEmitter = func(_ dataObservabilityDueQuery, result dataObservabilityQueryResult) {
					statuses = append(statuses, result.Status)
				}
				require.NoError(t, direct.collectDataObservability(context.Background()))
				assert.Equal(t, []string{"error", "success"}, statuses)
			})
		})
	}
}
