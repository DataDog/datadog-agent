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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	go_ora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionGoOra(t *testing.T) {
	databaseUrl := go_ora.BuildUrl(HOST, PORT, SERVICE_NAME, USER, PASSWORD, nil)
	conn, err := sql.Open("oracle", databaseUrl)
	assert.NoError(t, err)

	err = conn.Ping()
	assert.NoError(t, err)

}

func TestConnection(t *testing.T) {
	var databaseUrl string
	var err error
	var db *sqlx.DB

	if !skipGodror() {
		databaseUrl = fmt.Sprintf(`user="%s" password="%s" connectString="%s"`, USER, PASSWORD, TNS_ALIAS)
		db, err = sqlx.Open(common.Godror, databaseUrl)
		assert.NoError(t, err)
		err = db.Ping()
		assert.NoError(t, err)
	}

	c, _ := newRealCheck(t, "")
	_, err = c.Connect()
	assert.NoError(t, err)
}

func connectToDB(driver string) (*sqlx.DB, error) {
	var connStr string
	if driver == common.Godror {
		connStr = fmt.Sprintf(`user="%s" password="%s" connectString="%s:%d/%s"`, USER, PASSWORD, HOST, PORT, SERVICE_NAME)
	} else if driver == common.GoOra {
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
	chk.dbmEnabled = true
	chk.config.InstanceConfig.OracleClient = false

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
			chk.config.InstanceConfig.OracleClient = false
		} else {
			driver = common.Godror
		}
		if driver == common.Godror && skipGodror() {
			continue
		}

		chk.statementsLastRun = time.Now().Add(-48 * time.Hour)
		err := chk.Run()
		assert.NoError(t, err, "check run with %s driver", driver)

		sessionBefore, _ := getSession(chk.db)

		pgaBefore, err := getUsedPGA(chk.db)
		assert.NoError(t, err, "get used pga with %s driver", driver)
		chk.statementsLastRun = time.Now().Add(-48 * time.Hour)

		tempLobsBefore, _ := getTemporaryLobs(chk.db)

		/* Requires:
		 * create table sys.t(n number);
		 * grant insert on sys.t to c##datadog
		 */
		_, err = chk.db.Exec(`begin
				for i in 1..1000
				loop
					execute immediate 'insert into sys.t values (' || i || ')';
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

func TestBindingSimple(t *testing.T) {
	result := 3
	for _, driver := range getDrivers() {
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
	for _, driver := range getDrivers() {
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

func TestObfuscator(t *testing.T) {
	obfuscatorOptions := obfuscate.SQLConfig{}
	obfuscatorOptions.DBMS = common.IntegrationName
	obfuscatorOptions.TableNames = true
	obfuscatorOptions.CollectCommands = true
	obfuscatorOptions.CollectComments = true

	o := obfuscate.NewObfuscator(obfuscate.Config{SQL: config.GetDefaultObfuscatorOptions()})
	for _, statement := range []string{
		// needs https://datadoghq.atlassian.net/browse/DBM-2295
		`UPDATE /* comment */ SET t n=1`,
		`SELECT /* comment */ from dual`} {
		obfuscatedStatement, err := o.ObfuscateSQLString(statement)
		assert.NoError(t, err, "obfuscator error")
		assert.NotContains(t, obfuscatedStatement.Query, "comment", "comment wasn't removed by the obfuscator")
	}

	_, err := o.ObfuscateSQLString(`SELECT TRUNC(SYSDATE@!) from dual`)
	assert.NoError(t, err, "can't obfuscate @!")

	sql := "begin null ; end;"
	obfuscatedStatement, err := o.ObfuscateSQLString(sql)
	assert.Equal(t, obfuscatedStatement.Query, "begin null; end;")

	sql = "select count (*) from dual"
	obfuscatedStatement, err = o.ObfuscateSQLString(sql)
	assert.Equal(t, sql, obfuscatedStatement.Query)

	sql = "select file# from dual"
	obfuscatedStatement, err = o.ObfuscateSQLString(sql)
	assert.Equal(t, sql, obfuscatedStatement.Query)
}

func TestLegacyMode(t *testing.T) {
	c, s := newLegacyCheck(t, "")
	err := c.Run()
	require.NoError(t, err)

	canConnectServiceCheckName := "oracle.can_query"

	s.AssertServiceCheck(t, canConnectServiceCheckName, servicecheck.ServiceCheckOK, "", []string{"server:localhost"}, "")
	s.AssertServiceCheck(t, serviceCheckName, servicecheck.ServiceCheckOK, "", []string{"server:localhost"}, "")

	c, s = newLegacyCheck(t, "only_custom_queries: true")
	err = c.Run()
	require.NoError(t, err)

	s.AssertServiceCheck(t, canConnectServiceCheckName, servicecheck.ServiceCheckOK, "", []string{"server:localhost"}, "")
	s.AssertServiceCheck(t, serviceCheckName, servicecheck.ServiceCheckOK, "", []string{"server:localhost"}, "")
}
