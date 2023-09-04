// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/config"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
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

func TestCustomQueries(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	dbMock.ExpectExec("alter.*").WillReturnResult(sqlmock.NewResult(1, 1))

	rows := sqlmock.NewRows([]string{"c1", "c2"}).
		AddRow(1, "A").
		AddRow(2, "B")
	c1 := config.CustomQueryColumns{"c1", "gauge"}
	c2 := config.CustomQueryColumns{"c2", "tag"}
	columns := []config.CustomQueryColumns{c1, c2}
	dbMock.ExpectQuery("SELECT c1, c2 FROM t").WillReturnRows(rows)
	q := config.CustomQuery{
		MetricPrefix: "oracle.custom_query.test",
		Query:        "SELECT c1, c2 FROM t",
		Columns:      columns,
	}

	initAndStartAgentDemultiplexer(t)
	chk.Run()

	sender := mocksender.NewMockSender(chk.ID())
	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	chk.config.CustomQueries = []config.CustomQuery{q}

	err = chk.CustomQueries()
	assert.NoError(t, err, "failed to execute custom query")
	sender.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.c1", []string{"c2:A"})
}
