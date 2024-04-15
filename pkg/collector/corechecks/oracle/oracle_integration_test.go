// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"database/sql"
	"fmt"
	"strings"
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
	connection := getConnectData(t, useDefaultUser)
	databaseUrl := go_ora.BuildUrl(connection.Server, connection.Port, connection.ServiceName, connection.Username, connection.Password, nil)
	conn, err := sql.Open("oracle", databaseUrl)
	assert.NoError(t, err)

	err = conn.Ping()
	assert.NoError(t, err)

}

func TestConnection(t *testing.T) {
	var err error

	c, _ := newDefaultCheck(t, "", "")
	_, err = c.Connect()
	assert.NoError(t, err)
}

func connectToDB(driver string) (*sqlx.DB, error) {
	var connStr string
	connectionConfig := getConnectData(nil, useDefaultUser)
	if driver == common.Godror {
		godrorConnectionConfig := connectionConfig
		godrorConnectionConfig.OracleClient = true
		connStr = buildConnectionString(godrorConnectionConfig)
	} else if driver == common.GoOra {
		connStr = buildConnectionString(connectionConfig)
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

func getUsedPGA(t *testing.T, c *Check) (float64, error) {
	var pga float64
	require.NotEqual(t, "", c.config.InstanceConfig.Username, "username is nil")
	err := c.db.Get(&pga, `SELECT
	sum(p.pga_used_mem)
FROM   v$session s,
	v$process p
WHERE  s.paddr = p.addr AND s.username = :username`, strings.ToUpper(c.config.InstanceConfig.Username))
	return pga, err
}

func getSession(c *Check) (string, error) {
	var r string
	err := c.db.Get(&r, `SELECT sid || 'X' || serial# FROM v$session WHERE username = :username`, strings.ToUpper(c.config.InstanceConfig.Username))
	return r, err
}

func getLOBReads(c *Check) (float64, error) {
	var r float64
	err := c.db.Get(&r, `SELECT name,value
	FROM v$sesstat  m, v$statname n, v$session s
	WHERE n.statistic# = m.statistic# AND s.sid = m.sid AND s.username = :username AND n.name = 'lob reads'`, strings.ToUpper(c.config.InstanceConfig.Username))
	return r, err
}

func getTemporaryLobs(c *Check) (int, error) {
	var r int
	err := c.db.Get(&r, `SELECT SUM(cache_lobs) + SUM(nocache_lobs) + SUM(abstract_lobs)
	FROM v$temporary_lobs l, v$session s WHERE s.SID = l.SID AND s.username = :username`, strings.ToUpper(c.config.InstanceConfig.Username))
	return r, err
}

func TestChkRun(t *testing.T) {
	c, _ := newDefaultCheck(t, "", "")
	c.dbmEnabled = true
	c.config.InstanceConfig.OracleClient = false

	// This is to ensure that query samples return rows
	c.config.QuerySamples.IncludeAllSessions = true

	type RowsStruct struct {
		N int `db:"N"`
	}

	c.db = nil

	c.statementsLastRun = time.Now().Add(-48 * time.Hour)
	err := c.Run()
	assert.NoError(t, err, "check run")

	sessionBefore, _ := getSession(&c)

	pgaBefore, err := getUsedPGA(t, &c)
	assert.NoError(t, err, "get used pga")
	c.statementsLastRun = time.Now().Add(-48 * time.Hour)

	tempLobsBefore, _ := getTemporaryLobs(&c)

	/* Requires:
	 * create table sys.t(n number);
	 * grant insert on sys.t to c##datadog
	 */
	_, err = c.db.Exec(`begin
				for i in 1..1000
				loop
					execute immediate 'insert into sys.t values (' || i || ')';
				end loop;
				end ;`)
	assert.NoError(t, err, "error generating statements")

	c.Run()

	pgaAfter1StRun, _ := getUsedPGA(t, &c)
	diff1 := (pgaAfter1StRun - pgaBefore) / 1024
	var extremePGAUsage float64
	if isDbVersionGreaterOrEqualThan(&c, "12.2") {
		extremePGAUsage = 1024
	} else {
		extremePGAUsage = 8192
	}
	assert.Less(t, diff1, float64(extremePGAUsage), "extreme PGA usage (%f KB)", diff1)

	c.statementsLastRun = time.Now().Add(-48 * time.Hour)
	c.Run()

	pgaAfter2ndRun, _ := getUsedPGA(t, &c)
	diff2 := (pgaAfter2ndRun - pgaAfter1StRun) / 1024
	percGrowth := (diff2 - diff1) * 100 / diff1
	assert.Less(t, percGrowth, float64(10), "PGA memory leak (%f %% increase between two consecutive runs %d bytes)", percGrowth, pgaAfter2ndRun)
	time.Sleep(1 * time.Hour)

	tempLobsAfter, _ := getTemporaryLobs(&c)
	diffTempLobs := tempLobsAfter - tempLobsBefore
	assert.Equal(t, 0, diffTempLobs, "temporary LOB leak (%d)", diffTempLobs)

	sessionAfter, _ := getSession(&c)
	assert.Equal(t, sessionBefore, sessionAfter, "The agent reconnected")
}

func TestLicense(t *testing.T) {
	oracleDriver := "oracle"
	connection := getConnectData(t, useDefaultUser)
	connStr := go_ora.BuildUrl(
		connection.Server, connection.Port, connection.ServiceName, connection.Username, connection.Password, map[string]string{})
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

	driver := "oracle"
	db, _ := connectToDB(driver)
	stmt, err := db.Prepare(fmt.Sprintf("SELECT %d FROM dual WHERE rownum = :1", result))
	if err != nil {
		fmt.Printf("preparing statement %s", err)
	}
	row := stmt.QueryRow(1)
	if row.Err() != nil {
		fmt.Printf("row error %s", row.Err())
		return
	}
	var retValue int
	err = row.Scan(&retValue)
	if err != nil {
		fmt.Printf("scanning %s", err)
	}
	assert.Equal(t, retValue, result)
}

func TestSQLXIn(t *testing.T) {
	slice := []any{1}
	result := 7
	driver := common.GoOra
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
	canConnectServiceCheckName := "oracle.can_query"

	for _, config := range []string{
		"",
		"only_custom_queries: true",
	} {
		c, s := newLegacyCheck(t, config, "")
		err := c.Run()
		assert.NoError(t, err)
		expectedServerTag := fmt.Sprintf("server:%s", c.config.InstanceConfig.Server)
		s.AssertServiceCheck(t, canConnectServiceCheckName, servicecheck.ServiceCheckOK, "", []string{expectedServerTag}, "")
		s.AssertServiceCheck(t, serviceCheckName, servicecheck.ServiceCheckOK, "", []string{expectedServerTag}, "")
	}
}

func buildConnectionString(connectionConfig config.ConnectionConfig) string {
	var connStr string
	if connectionConfig.OracleClient {
		protocolString := ""
		walletString := ""
		if connectionConfig.Protocol == "TCPS" {
			protocolString = "tcps://"
			if connectionConfig.Wallet != "" {
				walletString = fmt.Sprintf("?wallet_location=%s", connectionConfig.Wallet)
			}
		}
		connStr = fmt.Sprintf(`user="%s" password="%s" connectString="%s%s:%d/%s%s"`,
			connectionConfig.Username, connectionConfig.Password, protocolString, connectionConfig.Server,
			connectionConfig.Port, connectionConfig.ServiceName, walletString)
	} else {
		connectionOptions := map[string]string{"TIMEOUT": DB_TIMEOUT}
		if connectionConfig.Protocol == "TCPS" {
			connectionOptions["SSL"] = "TRUE"
			if connectionConfig.Wallet != "" {
				connectionOptions["WALLET"] = connectionConfig.Wallet
			}
		}
		connStr = go_ora.BuildUrl(
			connectionConfig.Server, connectionConfig.Port, connectionConfig.ServiceName, connectionConfig.Username, connectionConfig.Password, connectionOptions)
	}
	return connStr
}
