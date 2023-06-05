// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	go_ora "github.com/sijms/go-ora/v2"

	_ "github.com/godror/godror"
)

var chk Check

var HOST = "localhost"
var PORT = 1521
var USER = "c##datadog"
var PASSWORD = "datadog"
var SERVICE_NAME = "XE"
var TNS_ALIAS = "XE"
var TNS_ADMIN = "/Users/nenad.noveljic/go/src/github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/testutil/etc/netadmin"

func TestBasic(t *testing.T) {
	chk = Check{}

	// language=yaml
	rawInstanceConfig := []byte(fmt.Sprintf(`
server: %s
port: %d
username: %s
password: %s
service_name: %s
tns_alias: %s
tns_admin: %s
`, HOST, PORT, USER, PASSWORD, SERVICE_NAME, TNS_ALIAS, TNS_ADMIN))

	err := chk.Configure(integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	assert.Equal(t, chk.config.InstanceConfig.Server, HOST)
	assert.Equal(t, chk.config.InstanceConfig.Port, PORT)
	assert.Equal(t, chk.config.InstanceConfig.Username, USER)
	assert.Equal(t, chk.config.InstanceConfig.Password, PASSWORD)
	assert.Equal(t, chk.config.InstanceConfig.ServiceName, SERVICE_NAME)
	assert.Equal(t, chk.config.InstanceConfig.TnsAlias, TNS_ALIAS)
	assert.Equal(t, chk.config.InstanceConfig.TnsAdmin, TNS_ADMIN)
}

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
	_, err = sqlx.Open("godror", databaseUrl)
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

func initAndStartAgentDemultiplexer() {
	aggregator.InitAndStartAgentDemultiplexer(nil, demuxOpts(), "")
}

func TestChkRun(t *testing.T) {
	initAndStartAgentDemultiplexer()

	chk.dbmEnabled = true
	chk.config.InstanceConfig.InstantClient = false

	type RowsStruct struct {
		N int `db:"N"`
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

		err = chk.db.Get(&r, "select /* DDTEST */ 1 n from dual")
		assert.NoError(t, err, "running test statement with %s driver", driver)
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
	//err = row.Scan(&usedFeaturesCount)
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
			log.Fatalf("preparing statement with driver %s %s", driver, err)
		}
		row := stmt.QueryRow(1)
		if row.Err() != nil {
			log.Fatalf("row error with driver %s %s", driver, row.Err())
			return
		}
		var retValue int
		err = row.Scan(&retValue)
		if err != nil {
			log.Fatalf("scanning with driver %s %s", driver, err)
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
			log.Fatalf("row error with driver %s %s", driver, err)
			return
		}

		rows.Next()
		var retValue int
		err = rows.Scan(&retValue)
		rows.Close()
		if err != nil {
			log.Fatalf("scan error %s", err)
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
			log.Fatalf("scan error %s", err)
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
