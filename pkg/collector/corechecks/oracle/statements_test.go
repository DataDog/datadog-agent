// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"log"

	//"log"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runStatementMetrics(c *Check, t *testing.T) {
	c.statementsLastRun = time.Now().Add(-48 * time.Hour)
	count, err := c.StatementMetrics()
	assert.NoError(t, err, "failed to run query metrics")
	assert.NotEmpty(t, count, "No statements processed in query metrics")
}

func initCheckReal(t *testing.T) Check {
	c := Check{}

	rawInstanceConfig := []byte(fmt.Sprintf(`
server: %s
port: %d
username: %s
password: %s
service_name: %s
dbm: true
query_metrics:
  trackers:
    - contains_text:
      - begin null;
    - contains_text:
      - dual
`, HOST, PORT, USER, PASSWORD, SERVICE_NAME))
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := c.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	assert.Equal(t, c.config.InstanceConfig.Server, HOST)
	assert.Equal(t, c.config.InstanceConfig.Port, PORT)
	assert.Equal(t, c.config.InstanceConfig.Username, USER)
	assert.Equal(t, c.config.InstanceConfig.Password, PASSWORD)
	assert.Equal(t, c.config.InstanceConfig.ServiceName, SERVICE_NAME)

	return c
}

func TestQueryMetrics(t *testing.T) {
	c := initCheckReal(t)

	var n int
	var err error
	err = c.Run()
	assert.NoError(t, err, "statement check run")

	testStatement := "begin null; end;"
	for i := 1; i <= 2; i++ {
		_, err = c.db.Exec(testStatement)
		assert.NoError(t, err, "failed to execute the test statement")

		for j := 1; j <= 10; j++ {
			err = getWrapper(&c, &n, fmt.Sprintf("select %d from dual", j))
			assert.NoError(t, err, "failed to execute the test query")
		}
		runStatementMetrics(&c, t)
	}

	var statementExecutions float64
	var queryExecutions float64
	for _, r := range c.lastOracleRows {
		if r.SQLText == testStatement {
			statementExecutions = r.Executions
		} else if r.SQLText == "select ? from dual" {
			queryExecutions = r.Executions
		}
	}
	assert.Equal(t, float64(1), statementExecutions, "PL/SQL execution not captured")
	assert.Equal(t, float64(10), queryExecutions)
}

func TestUInt64Binding(t *testing.T) {

	chk.dbmEnabled = true
	chk.config.QueryMetrics.Enabled = true

	chk.config.InstanceConfig.OracleClient = false

	type RowsStruct struct {
		N                      int    `db:"N"`
		SQLText                string `db:"SQL_TEXT"`
		ForceMatchingSignature uint64 `db:"FORCE_MATCHING_SIGNATURE"`
	}
	r := RowsStruct{}

	for _, tnsAlias := range []string{"", TNS_ALIAS} {
		chk.db = nil

		chk.config.InstanceConfig.TnsAlias = tnsAlias
		var driver string
		if tnsAlias == "" {
			driver = common.GoOra
			chk.config.InstanceConfig.OracleClient = false
		} else {
			driver = common.Godror
		}
		if driver == common.Godror && skipGodror() {
			continue
		}

		err := chk.Run()
		assert.NoError(t, err, "check run with %s driver", driver)

		m := make(map[string]int)
		m["2267897546238586672"] = 1

		err = chk.db.Get(&r, "select force_matching_signature, sql_text from v$sqlstats where sql_text like '%t111%'") // force_matching_signature=17202440635181618732
		assert.NoError(t, err, "running statement with large force_matching_signature with %s driver", driver)

		slice := []any{"17202440635181618732"}
		var retValue int
		err = chk.db.Get(&retValue, "SELECT COUNT(*) FROM v$sqlstats WHERE force_matching_signature IN (:1)", slice...)
		if err != nil {
			//log.Fatalf("%S row error with driver %s %s", chk.logPrompt, driver, err)
			return
		}
		assert.Equal(t, 1, retValue, "Testing IN slice uint64 overflow with driver %s", driver)

		//In Rebind with a large uint64 value
		query, args, err := sqlx.In("SELECT COUNT(*) FROM v$sqlstats WHERE force_matching_signature IN (?)", slice)
		if err != nil {
			fmt.Println(err)
		}

		rows, err := chk.db.Query(chk.db.Rebind(query), args...)

		assert.NoErrorf(t, err, "preparing statement with IN clause for %s driver", driver)

		if err == nil {
			defer rows.Close()
			rows.Next()
			err = rows.Scan(&retValue)

			if err != nil {
				log.Fatalf("%s scan error %s", chk.logPrompt, err)
			}
			assert.Equalf(t, retValue, 1, "IN uint64 with %s driver", driver)
		}
	}
}
