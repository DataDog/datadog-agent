// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"encoding/json"
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
			c, sender := newTestCheck(t, connection, "data_observability:\n  enabled: true", "")
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

			t.Run("payload", func(t *testing.T) {
				var emptyLOB any = ""
				if oracleDriver == common.GoOra {
					// go-ora v2.9.0 returns nil for both NULL and empty LOBs.
					emptyLOB = nil
				}
				query := dataObservabilityTestQuery(c.getFullPDBName(pdb), `SELECT
					42.25 AS N, '42.25' AS TEXT, HEXTORAW('68656C6C6F') AS BYTES,
					TO_CLOB('clob') AS CLOB_VALUE, TO_NCLOB('nclob') AS NCLOB_VALUE,
					TO_BLOB(HEXTORAW('626C6F62')) AS BLOB_VALUE,
					CAST(NULL AS NUMBER) AS NULL_VALUE,
					CAST(NULL AS VARCHAR2(10)) AS NULL_VARCHAR, CAST(NULL AS NVARCHAR2(10)) AS NULL_NVARCHAR,
					CAST(NULL AS CHAR(10)) AS NULL_CHAR, CAST(NULL AS NCHAR(10)) AS NULL_NCHAR,
					TO_CLOB(NULL) AS NULL_CLOB, TO_NCLOB(NULL) AS NULL_NCLOB, TO_BLOB(NULL) AS NULL_BLOB,
					EMPTY_CLOB() AS EMPTY_CLOB_VALUE, TO_NCLOB(EMPTY_CLOB()) AS EMPTY_NCLOB_VALUE, EMPTY_BLOB() AS EMPTY_BLOB_VALUE,
					TO_DATE('2026-01-02 03:04:05', 'YYYY-MM-DD HH24:MI:SS') AS DATE_VALUE,
					TIMESTAMP '2026-01-02 03:04:05.123456' AS TIMESTAMP_VALUE,
					TO_TIMESTAMP_TZ('2026-01-02 03:04:05.123456 -05:00', 'YYYY-MM-DD HH24:MI:SS.FF TZH:TZM') AS TZ_VALUE,
					BINARY_DOUBLE_NAN AS NAN_VALUE FROM dual`)
				result, err := c.executeDataObservabilityQuery(context.Background(), query, pdb)
				require.NoError(t, err)
				emit, err := c.newDataObservabilityEmitter()
				require.NoError(t, err)
				emit(dataObservabilityDueQuery{scheduledQuery: &dataObservabilityScheduledQuery{query: query}, scheduledTime: c.clock.Now()}, result)
				sender.AssertNumberOfCalls(t, "EventPlatformEvent", 1)
				for _, call := range sender.Calls {
					if call.Method != "EventPlatformEvent" {
						continue
					}
					assert.Equal(t, "do-query-results", call.Arguments.String(1))
					var payload map[string]any
					require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &payload))
					assert.Equal(t, "success", payload["status"])
					assert.Equal(t, query.DBName, payload["db_name"])
					assert.Equal(t, []any{[]any{42.25, "42.25", "hello", "clob", "nclob", "blob", nil,
						nil, nil, nil, nil, nil, nil, nil, emptyLOB, emptyLOB, emptyLOB,
						"2026-01-02T03:04:05", "2026-01-02T03:04:05.123456", "2026-01-02T03:04:05.123456-05:00", nil}}, payload["rows"])
				}
			})

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
