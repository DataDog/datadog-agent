// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

func TestQueryMetrics(t *testing.T) {
	initAndStartAgentDemultiplexer(t)
	chk.dbmEnabled = true
	chk.statementsLastRun = time.Now().Add(-48 * time.Hour)
	count, err := chk.StatementMetrics()
	assert.NoError(t, err, "failed to run query metrics")
	assert.NotEmpty(t, count, "No statements processed in query metrics")
}

func TestUInt64Binding(t *testing.T) {
	initAndStartAgentDemultiplexer(t)

	chk.dbmEnabled = true
	chk.config.QueryMetrics.Enabled = true

	chk.config.InstanceConfig.InstantClient = false

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
			chk.config.InstanceConfig.InstantClient = false
		} else {
			driver = common.Godror
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
			log.Fatalf("%S row error with driver %s %s", chk.logPrompt, driver, err)
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
