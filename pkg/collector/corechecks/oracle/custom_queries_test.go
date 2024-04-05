// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

var chk Check

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	err := chk.Configure(senderManager, integration.FakeConfigHash, rawInstanceConfig, []byte(``), "oracle_test")
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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	chk, err := initCheck(t, senderManager, "localhost", 1523, "a", "a", "a")
	chk.Run()

	sender := mocksender.NewMockSenderWithSenderManager(chk.ID(), senderManager)
	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	chk.config.InstanceConfig.CustomQueries = []config.CustomQuery{q}
	chk.dbCustomQueries = sqlx.NewDb(db, "sqlmock")

	err = chk.CustomQueries()
	assert.NoError(t, err, "failed to execute custom query")
	sender.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.c1", []string{"c2:A"})
}

const customQueryTestConfig = `- metric_prefix: oracle.custom_query.test
  query: |
    select 'TAG1', 1.012345 value from dual
  columns:
    - name: name
      type: tag
    - name: value
      type: gauge
`

func assertCustomQuery(t *testing.T, c *Check, s *mocksender.MockSender) {
	err := c.Run()
	require.NoError(t, err)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.value", []string{"name:TAG1"})
	s.AssertMetric(t, "Gauge", "oracle.custom_query.test.value", 1.012345, c.dbHostname, []string{})
}

func TestFloat(t *testing.T) {
	c, s := newRealCheck(t, `custom_queries:
  - metric_prefix: oracle.custom_query.test
    query: |
      select 'TAG1', 1.012345 value from dual
    columns:
      - name: name
        type: tag
      - name: value
        type: gauge
`)
	err := c.Run()
	require.NoError(t, err)
	s.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s.AssertMetricTaggedWith(t, "Gauge", "oracle.custom_query.test.value", []string{"name:TAG1"})
	s.AssertMetric(t, "Gauge", "oracle.custom_query.test.value", 1.012345, c.dbHostname, []string{})
}

func TestGlobalCustomQueries(t *testing.T) {
	globalCustomQueries := fmt.Sprintf("global_custom_queries:\n%s", customQueryTestConfig)
	c, s := newTestCheck(t, getDefaultConnectData(USER), "", globalCustomQueries)
	assertCustomQuery(t, &c, s)
}
