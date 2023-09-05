// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"

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
dbm: true
`, HOST, PORT, USER, PASSWORD, SERVICE_NAME, TNS_ALIAS, TNS_ADMIN))

	err := chk.Configure(aggregator.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
	require.NoError(t, err)

	assert.Equal(t, chk.config.InstanceConfig.Server, HOST)
	assert.Equal(t, chk.config.InstanceConfig.Port, PORT)
	assert.Equal(t, chk.config.InstanceConfig.Username, USER)
	assert.Equal(t, chk.config.InstanceConfig.Password, PASSWORD)
	assert.Equal(t, chk.config.InstanceConfig.ServiceName, SERVICE_NAME)
	assert.Equal(t, chk.config.InstanceConfig.TnsAlias, TNS_ALIAS)
	assert.Equal(t, chk.config.InstanceConfig.TnsAdmin, TNS_ADMIN)
}

func TestGetFullSqlText(t *testing.T) {
	initAndStartAgentDemultiplexer(t)
	for _, tnsAlias := range []string{"", TNS_ALIAS} {
		chk.db = nil

		chk.config.InstanceConfig.TnsAlias = tnsAlias
		chk.dbmEnabled = false

		var driver string
		if tnsAlias == "" {
			driver = common.GoOra
			chk.config.InstanceConfig.InstantClient = false
		} else {
			driver = common.Godror
		}
		chk.Run()

		var SQLStatement string
		err := getFullSQLText(&chk, &SQLStatement, "sql_id", "A")
		assert.ErrorContainsf(t, err, "no rows", "getFullSQLText didn't return `no rows` error with %s driver", driver)
	}
}
