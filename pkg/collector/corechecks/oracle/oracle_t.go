// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
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
var TNS_ADMIN = "/Users/nenad.noveljic/go/src/github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/testutil/etc/netadmin"

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
	opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
	opts.FlushInterval = 1 * time.Hour
	opts.DontStartForwarders = true
	return opts
}

func TestChkRun(t *testing.T) {
	aggregator.InitAndStartAgentDemultiplexer(demuxOpts(), "")
	err := chk.Run()
	assert.NoError(t, err)
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
	row := db.QueryRow(`SELECT SUM(detected_usages) 
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
	var usedFeaturesCount int
	err = row.Scan(&usedFeaturesCount)
	if err != nil {
		fmt.Printf("failed to query hostname and version: %s", err)
	}
	assert.Equal(t, 0, usedFeaturesCount)
}
