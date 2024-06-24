// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLocks(t *testing.T) {
	db, dbMock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	c, sender := newDbDoesNotExistCheck(t, "", "")
	c.dbVersion = "19.2"
	c.db = sqlx.NewDb(db, "sqlmock")
	c.Run()

	sender.SetupAcceptAll()
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	hname, _ := hostname.Get(context.TODO())
	m := metricsPayload{
		Host:                  c.dbHostname,
		Kind:                  "lock_metrics",
		Timestamp:             float64(time.Now().UnixMilli()),
		MinCollectionInterval: float64(c.config.MinCollectionInterval),
		Tags:                  c.tags,
		AgentVersion:          c.agentVersion,
		AgentHostname:         hname,
		OracleVersion:         c.dbVersion,
	}

	dbMock.ExpectQuery("SELECT.*transaction.*").WillReturnError(sql.ErrNoRows)
	err = c.locks()
	assert.NoError(t, err, "failed to execute locks query")
	emptyPayload := lockMetricsPayload{
		OracleRows: []oracleLockRow{},
	}
	emptyPayload.metricsPayload = m
	payloadBytes, err := json.Marshal(emptyPayload)
	require.NoError(t, err, "failed to marshal lock metrics payload")
	sender.AssertNotCalled(t, "EventPlatformEvent", payloadBytes, "dbm-metrics")

	rows := sqlmock.NewRows([]string{"SECONDS", "PROGRAM"}).
		AddRow(17, "test-program")
	dbMock.ExpectQuery("SELECT.*transaction.*").WillReturnRows(rows)
	err = c.locks()
	assert.NoError(t, err, "failed to execute locks query")
	payload := lockMetricsPayload{
		OracleRows: []oracleLockRow{
			{
				SecondsInTransaction: 17,
				Program:              "test-program",
			},
		},
	}
	payload.metricsPayload = m
	payloadBytes, err = json.Marshal(payload)
	require.NoError(t, err, "failed to marshal lock metrics payload")
	sender.AssertEventPlatformEvent(t, payloadBytes, "dbm-metrics")
}
