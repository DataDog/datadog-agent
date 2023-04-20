// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"
	"log"
	"testing"

	//"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

func TestUInt64Binding(t *testing.T) {
	//aggregator.InitAndStartAgentDemultiplexer(demuxOpts(), "")
	initAndStartAgentDemultiplexer()

	chk.dbmEnabled = true
	chk.config.QueryMetrics = true

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
		chk.statementsFilter = StatementsFilter{ForceMatchingSignatures: m}

		statementMetrics, err := GetStatementsMetricsForKeys(&chk, "force_matching_signature", "AND force_matching_signature != 0", chk.statementsFilter.ForceMatchingSignatures)
		assert.NoError(t, err, "running GetStatementsMetricsForKeys with %s driver", driver)
		assert.Lenf(t, statementMetrics, 1, "test query metrics captured with %s driver", driver)
		err = chk.db.Get(&r, "select force_matching_signature, sql_text from v$sqlstats where sql_text like '%t111%'") // force_matching_signature=17202440635181618732
		assert.NoError(t, err, "running statement with large force_matching_signature with %s driver", driver)

		m = make(map[string]int)
		m["17202440635181618732"] = 1
		chk.statementsFilter = StatementsFilter{ForceMatchingSignatures: m}
		assert.Lenf(t, statementMetrics, 1, "test query metrics for uint64 overflow with %s driver", driver)

		chk.statementsFilter = StatementsFilter{ForceMatchingSignatures: m}
		n, err := chk.StatementMetrics()
		assert.NoError(t, err, "query metrics with %s driver", driver)
		assert.Equal(t, 1, n, "total query metrics captured with %s driver", driver)

		//slice := []any{uint64(17202440635181618732)}
		slice := []any{"17202440635181618732"}
		var retValue int
		err = chk.db.Get(&retValue, "SELECT COUNT(*) FROM v$sqlstats WHERE force_matching_signature IN (:1)", slice...)
		if err != nil {
			log.Fatalf("row error with driver %s %s", driver, err)
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
				log.Fatalf("scan error %s", err)
			}
			assert.Equalf(t, retValue, 1, "IN uint64 with %s driver", driver)
		}
	}
}
