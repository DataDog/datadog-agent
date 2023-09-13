// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"database/sql"
	"fmt"

	"testing"
	"time"

	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/jmoiron/sqlx"
	go_ora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	_ "github.com/godror/godror"
)

func TestConnectionGoOra(t *testing.T) {
	databaseUrl := go_ora.BuildUrl(HOST, PORT, SERVICE_NAME, USER, PASSWORD, nil)
	conn, err := sql.Open("oracle", databaseUrl)
	assert.NoError(t, err)

	err = conn.Ping()
	assert.NoError(t, err)

}

func TestConnection(t *testing.T) {
	databaseUrl := fmt.Sprintf(`user="%s" password="%s" connectString="%s"`, USER, PASSWORD, TNS_ALIAS)
	db, err := sqlx.Open("godror", databaseUrl)
	assert.NoError(t, err)
	err = db.Ping()
	assert.NoError(t, err)

	databaseUrl = fmt.Sprintf(`user="%s" password="%s" connectString="%s:%d/%s"`, USER, PASSWORD, HOST, PORT, SERVICE_NAME)
	_, err = sqlx.Open("oracle", databaseUrl)
	assert.NoError(t, err)
	err = db.Ping()
	assert.NoError(t, err)

}

func demuxOpts() aggregator.AgentDemultiplexerOptions {
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 1 * time.Hour
	opts.DontStartForwarders = true
	return opts
}

func connectToDB(driver string) (*sqlx.DB, error) {
	var connStr string
	if driver == "godror" {
		connStr = fmt.Sprintf(`user="%s" password="%s" connectString="%s:%d/%s"`, USER, PASSWORD, HOST, PORT, SERVICE_NAME)
	} else if driver == "oracle" {
		connStr = go_ora.BuildUrl(HOST, PORT, SERVICE_NAME, USER, PASSWORD, map[string]string{})
	} else {
		return nil, fmt.Errorf("wrong driver: %s", driver)
	}

	db, err := sqlx.Open(driver, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to oracle instance: %w", err)
	}
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping oracle instance: %w", err)
	}
	// https://github.com/jmoiron/sqlx/issues/854#issuecomment-1504070464
	if driver == "oracle" {
		sqlx.BindDriver("oracle", sqlx.NAMED)
	}
	return db, nil
}

func initAndStartAgentDemultiplexer(t *testing.T) {
	deps := fxutil.Test[aggregator.AggregatorTestDeps](t, defaultforwarder.MockModule, config.MockModule, log.MockModule)
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.DontStartForwarders = true

	_ = aggregator.InitAndStartAgentDemultiplexerForTest(deps, opts, "hostname")
}

func getUsedPGA(db *sqlx.DB) (float64, error) {
	var pga float64
	err := chk.db.Get(&pga, `SELECT 
	sum(p.pga_used_mem)
FROM   v$session s,
	v$process p
WHERE  s.paddr = p.addr AND s.username = 'C##DATADOG'`)
	return pga, err
}

func getSession(db *sqlx.DB) (string, error) {
	var r string
	err := chk.db.Get(&r, `SELECT sid || 'X' || serial# FROM v$session WHERE username = 'C##DATADOG'`)
	return r, err
}

func getLOBReads(db *sqlx.DB) (float64, error) {
	var r float64
	err := chk.db.Get(&r, `SELECT name,value 
	FROM v$sesstat  m, v$statname n, v$session s 
	WHERE n.statistic# = m.statistic# AND s.sid = m.sid AND s.username = 'C##DATADOG' AND n.name = 'lob reads'`)
	return r, err
}

func getTemporaryLobs(db *sqlx.DB) (int, error) {
	var r int
	err := chk.db.Get(&r, `SELECT SUM(cache_lobs) + SUM(nocache_lobs) + SUM(abstract_lobs) 
	FROM v$temporary_lobs l, v$session s WHERE s.SID = l.SID AND s.username = 'C##DATADOG'`)
	return r, err
}

func TestChkRun(t *testing.T) {
	initAndStartAgentDemultiplexer(t)
	chk.dbmEnabled = true
	chk.config.InstanceConfig.InstantClient = false

	// This is to ensure that query samples return rows
	chk.config.QuerySamples.IncludeAllSessions = true

	type RowsStruct struct {
		N int `db:"N"`
	}

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

		chk.statementsLastRun = time.Now().Add(-48 * time.Hour)
		err := chk.Run()
		assert.NoError(t, err, "check run with %s driver", driver)

		sessionBefore, _ := getSession(chk.db)

		pgaBefore, err := getUsedPGA(chk.db)
		assert.NoError(t, err, "get used pga with %s driver", driver)
		chk.statementsLastRun = time.Now().Add(-48 * time.Hour)

		tempLobsBefore, _ := getTemporaryLobs(chk.db)

		_, err = chk.db.Exec(`begin
				for i in 1..1000
				loop
					execute immediate 'insert into t values (' || i || ')';
				end loop;
				end ;`)
		assert.NoError(t, err, "error generating statements with %s driver", driver)

		chk.Run()

		pgaAfter1StRun, _ := getUsedPGA(chk.db)
		diff1 := (pgaAfter1StRun - pgaBefore) / 1024
		assert.Less(t, diff1, float64(1024), "extreme PGA usage (%f KB) with the %s driver", diff1, driver)

		chk.statementsLastRun = time.Now().Add(-48 * time.Hour)
		chk.Run()

		pgaAfter2ndRun, _ := getUsedPGA(chk.db)
		diff2 := (pgaAfter2ndRun - pgaAfter1StRun) / 1024
		percGrowth := (diff2 - diff1) * 100 / diff1
		assert.Less(t, percGrowth, float64(10), "PGA memory leak (%f %% increase between two consecutive runs) with the %s driver", percGrowth, driver)

		tempLobsAfter, _ := getTemporaryLobs(chk.db)
		diffTempLobs := tempLobsAfter - tempLobsBefore
		assert.Equal(t, 0, diffTempLobs, "temporary LOB leak (%d) with %s driver", diffTempLobs, driver)

		sessionAfter, _ := getSession(chk.db)
		assert.Equal(t, sessionBefore, sessionAfter, "The agent reconnected")
	}
}

func TestLicense(t *testing.T) {
	oracleDriver := "oracle"
	connStr := go_ora.BuildUrl(HOST, PORT, SERVICE_NAME, USER, PASSWORD, map[string]string{})
	db, err := sqlx.Open(oracleDriver, connStr)
	if err != nil {
		fmt.Printf("failed to connect to oracle instance: %s", err)
	}
	err = db.Ping()
	if err != nil {
		fmt.Printf("failed to ping oracle instance: %s", err)
	}
	var usedFeaturesCount int
	err = db.Get(&usedFeaturesCount, `SELECT NVL(SUM(detected_usages),0)
	FROM dba_feature_usage_statistics
 	WHERE name in (
		'ADDM', 
		'Automatic SQL Tuning Advisor', 
		'Automatic Workload Repository', 
		'AWR Baseline', 
		'AWR Baseline Template', 
		'AWR Report', 
		'EM Performance Page', 
		'Real-Time SQL Monitoring', 
		'SQL Access Advisor', 
		'SQL Monitoring and Tuning pages', 
		'SQL Performance Analyzer', 
		'SQL Tuning Advisor', 
		'SQL Tuning Set (user)'
		)
 `)
	if err != nil {
		fmt.Printf("failed to query license info: %s", err)
	}
	assert.Equal(t, 0, usedFeaturesCount)
}

var DRIVERS = []string{"oracle", "godror"}

func TestBindingSimple(t *testing.T) {
	result := 3
	for _, driver := range DRIVERS {
		db, _ := connectToDB(driver)
		stmt, err := db.Prepare(fmt.Sprintf("SELECT %d FROM dual WHERE rownum = :1", result))
		if err != nil {
			fmt.Printf("preparing statement with driver %s %s", driver, err)
		}
		row := stmt.QueryRow(1)
		if row.Err() != nil {
			fmt.Printf("row error with driver %s %s", driver, row.Err())
			return
		}
		var retValue int
		err = row.Scan(&retValue)
		if err != nil {
			fmt.Printf("scanning with driver %s %s", driver, err)
		}
		assert.Equal(t, retValue, result, driver)
	}
}

func TestSQLXIn(t *testing.T) {
	slice := []any{1}
	result := 7
	for _, driver := range DRIVERS {
		db, _ := connectToDB(driver)

		var rows *sql.Rows
		var err error

		rows, err = db.Query(fmt.Sprintf("SELECT %d FROM dual WHERE rownum IN (:1)", result), slice...)
		if err != nil {
			fmt.Printf("row error with driver %s %s", driver, err)
			return
		}

		rows.Next()
		var retValue int
		err = rows.Scan(&retValue)
		rows.Close()
		if err != nil {
			fmt.Printf("scan error %s", err)
		}
		assert.Equal(t, retValue, result, driver)

		query, args, err := sqlx.In(fmt.Sprintf("SELECT %d FROM dual WHERE rownum IN (?)", result), slice)
		if err != nil {
			fmt.Println(err)
		}

		rows, err = db.Query(db.Rebind(query), args...)

		assert.NoErrorf(t, err, "preparing statement with IN clause for %s driver", driver)

		rows.Next()
		err = rows.Scan(&retValue)
		rows.Close()
		if err != nil {
			fmt.Printf("scan error %s", err)
		}
		assert.Equal(t, retValue, result, driver)
	}

}

func TestLargeUint64Binding(t *testing.T) {
	largeUint64 := uint64(18446744073709551615)
	//largeUint64 = 1
	var result uint64
	for _, driver := range DRIVERS {
		db, _ := connectToDB(driver)
		err := db.Get(&result, "SELECT n FROM T WHERE n = :1", largeUint64)
		assert.NoError(t, err, "running test statement with %s driver", driver)
		if err != nil {
			continue
		}
		assert.Equal(t, result, largeUint64, "simple uint64 binding with %s driver", driver)
	}
}
