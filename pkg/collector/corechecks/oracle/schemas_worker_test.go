// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

func TestSchemaWorkerCancelWhileAcquiring(t *testing.T) {
	c, pool, _, cleanup := newSchemaCheck(t)
	defer cleanup()
	pool.SetMaxOpenConns(1)
	held, err := pool.Connx(context.Background())
	require.NoError(t, err)
	defer held.Close()
	c.dbmEnabled = true
	require.NoError(t, c.collectSchemasIfDue())
	c.schemaWorkerMu.Lock()
	done := c.schemaWorkerDone
	require.True(t, c.schemaWorkerRunning)
	firstRun := c.schemasLastRun
	c.schemaWorkerMu.Unlock()
	c.clock.(*clock.Mock).Add(601 * time.Second)
	require.NoError(t, c.collectSchemasIfDue())
	require.Equal(t, firstRun, c.schemasLastRun)
	c.Cancel()
	select {
	case <-done:
	default:
		t.Fatal("Cancel returned before schema worker completed")
	}
	require.False(t, c.schemaWorkerRunning)
	require.True(t, c.schemaWorkerStopped)
	require.NoError(t, c.collectSchemasIfDue())
	require.Equal(t, firstRun, c.schemasLastRun)
}

func TestSchemaWorkerFailureAllowsNextAttempt(t *testing.T) {
	c, pool, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	c.dbmEnabled = true
	c.db = nil
	require.NoError(t, c.collectSchemasIfDue())
	c.stopSchemaWorker(false)
	require.False(t, c.schemaWorkerRunning)
	require.False(t, c.schemaWorkerStopped)
	firstRun := c.schemasLastRun
	c.clock.(*clock.Mock).Add(601 * time.Second)
	c.db = pool
	c.config.AgentSQLTrace.Enabled = true
	expectEmptySchemaSnapshot(dbMock)
	require.NoError(t, c.collectSchemasIfDue())
	require.True(t, c.schemasLastRun.After(firstRun))
	require.Positive(t, c.lastSnapshotID)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func expectEmptySchemaSnapshot(dbMock sqlmock.Sqlmock) {
	dbMock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(
		sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
}

func TestSchemaWorkerPanicReleasesConnection(t *testing.T) {
	c, pool, dbMock, cleanup := newSchemaCheck(t)
	defer cleanup()
	run, err := c.newSchemaCollectionRun()
	require.NoError(t, err)
	run.schemaEmitter = func([]byte) { panic("test emitter failure") }
	expectEmptySchemaSnapshot(dbMock)
	ctx, cancel := context.WithCancel(context.Background())
	c.schemaWorkerRunning = true
	c.schemaWorkerCancel = cancel
	c.schemaWorkerDone = make(chan struct{})
	err = c.runSchemaWorker(ctx, pool, run)
	require.ErrorContains(t, err, "test emitter failure")
	require.Zero(t, pool.Stats().InUse)
	require.False(t, c.schemaWorkerRunning)
	previousSnapshot := c.lastSnapshotID
	c.dbmEnabled = true
	c.config.AgentSQLTrace.Enabled = true
	c.clock.(*clock.Mock).Add(601 * time.Second)
	expectEmptySchemaSnapshot(dbMock)
	require.NoError(t, c.collectSchemasIfDue())
	require.Greater(t, c.lastSnapshotID, previousSnapshot)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaWorkerExpiredAcquisitionDeadline(t *testing.T) {
	c, pool, _, cleanup := newSchemaCheck(t)
	defer cleanup()
	run, err := c.newSchemaCollectionRun()
	require.NoError(t, err)
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	c.schemaWorkerRunning = true
	c.schemaWorkerCancel = cancel
	c.schemaWorkerDone = make(chan struct{})
	err = c.runSchemaWorker(ctx, pool, run)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, c.schemaWorkerRunning)
	require.Zero(t, pool.Stats().InUse)
}

func TestSchemaWorkerConcurrentCancelAndLaunch(t *testing.T) {
	for range 25 {
		c, pool, _, cleanup := newSchemaCheck(t)
		pool.SetMaxOpenConns(1)
		held, err := pool.Connx(context.Background())
		require.NoError(t, err)
		c.dbmEnabled = true
		start := make(chan struct{})
		launched := make(chan error, 1)
		canceled := make(chan struct{})
		go func() {
			<-start
			launched <- c.collectSchemasIfDue()
		}()
		go func() {
			<-start
			c.Cancel()
			close(canceled)
		}()
		close(start)
		require.NoError(t, <-launched)
		<-canceled
		require.True(t, c.schemaWorkerStopped)
		require.False(t, c.schemaWorkerRunning)
		require.NoError(t, held.Close())
		cleanup()
	}
}

type schemaQueryFunc func(context.Context, string, ...interface{}) (*sqlx.Rows, error)

func (f schemaQueryFunc) QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	return f(ctx, query, args...)
}

func TestSchemaWorkerCopiesMutableInputs(t *testing.T) {
	c, db, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	c.tags = []string{"original"}
	c.dbHostname = "original-host"
	c.dbInstanceIdentifier = "original-instance"
	c.config.Schemas.IncludeSchemas = []string{"APP"}
	views := true
	c.config.Schemas.CollectViews = &views
	c.lastSnapshotID = 123
	run, err := c.newSchemaCollectionRun()
	require.NoError(t, err)
	c.tags[0] = "changed"
	c.config.Schemas.IncludeSchemas[0] = "OTHER"
	views = false
	c.dbHostname = "changed"
	c.dbInstanceIdentifier = "changed-instance"
	c.config.Schemas.CollectionInterval = 999
	require.Equal(t, []string{"original"}, run.tags)
	require.Equal(t, []string{"APP"}, run.config.Schemas.IncludeSchemas)
	require.True(t, run.config.Schemas.ViewsEnabled())
	require.NotEqual(t, c.dbHostname, run.dbHostname)
	require.Equal(t, int64(123), run.lastSnapshotID)
	require.Nil(t, run.db)
	conn, err := db.Connx(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	run.schemaQueryer = conn
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows())
	require.NoError(t, run.schemaCollection(context.Background()))
	sender, err := c.GetRawSender()
	require.NoError(t, err)
	mockSender, ok := sender.(*mocksender.MockSender)
	require.True(t, ok)
	var events []schemaEvent
	for _, call := range mockSender.Calls {
		if call.Method == "EventPlatformEvent" {
			var event schemaEvent
			require.NoError(t, json.Unmarshal(call.Arguments.Get(0).([]byte), &event))
			events = append(events, event)
		}
	}
	require.Len(t, events, 1)
	require.Equal(t, []string{"original"}, events[0].Tags)
	require.Equal(t, "original-host", events[0].Host)
	require.Equal(t, "original-instance", events[0].DatabaseInstance)
	require.Equal(t, int64(600), events[0].CollectionInterval)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaWorkerUsesReservedConnectionWithoutCommit(t *testing.T) {
	c, db, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	run, err := c.newSchemaCollectionRun()
	require.NoError(t, err)
	conn, err := db.Connx(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	run.schemaQueryer = conn
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
	require.NoError(t, run.schemaCollection(context.Background()))
	require.NoError(t, dbMock.ExpectationsWereMet())
	sender, err := c.GetRawSender()
	require.NoError(t, err)
	mockSender, ok := sender.(*mocksender.MockSender)
	require.True(t, ok)
	mockSender.AssertNotCalled(t, "Commit")
}

func TestSchemaWorkerCanceledDiscoveryDoesNotEmitEmptySnapshot(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.schemaQueryer = schemaQueryFunc(func(context.Context, string, ...interface{}) (*sqlx.Rows, error) {
		cancel()
		return nil, context.Canceled
	})
	emitted := false
	c.schemaEmitter = func([]byte) { emitted = true }
	require.ErrorIs(t, c.schemaCollection(ctx), context.Canceled)
	require.False(t, emitted)
}

func TestSchemaWorkerDetailTimeoutStopsFollowingQueries(t *testing.T) {
	c, _, _, closeDB := newSchemaCheck(t)
	defer closeDB()
	queries := 0
	c.schemaQueryer = schemaQueryFunc(func(context.Context, string, ...interface{}) (*sqlx.Rows, error) {
		queries++
		return nil, context.DeadlineExceeded
	})
	c.tableDetailsForPage(context.Background(), map[tableKey]struct{}{{conID: 3, owner: "APP", table: "T"}: {}}, nil, nil)
	require.Equal(t, 1, queries)
	require.ErrorIs(t, c.schemaContextError(context.Background()), context.DeadlineExceeded)
}

func TestSchemaWorkerDetailTimeoutDoesNotCompleteAndNextRunSucceeds(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	var emitted [][]byte
	c.schemaEmitter = func(payload []byte) { emitted = append(emitted, payload) }
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())
	dbMock.ExpectQuery("SELECT con_id, owner, view_name").WillReturnRows(identityRows(tableKey{conID: 3, owner: "APP", table: "V1"}))
	dbMock.ExpectQuery(`(?s)WITH ranked_columns.*FROM cdb_views v`).WillReturnRows(addViewRow(sqlmock.NewRows(viewRelationColumns), 3, "APP", "V1", 1))
	dbMock.ExpectQuery("SELECT con_id, owner, view_name, text_vc").WillReturnError(context.DeadlineExceeded)
	require.ErrorIs(t, c.schemaCollection(context.Background()), context.DeadlineExceeded)
	require.Empty(t, emitted)

	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}))
	require.NoError(t, c.schemaCollection(context.Background()))
	require.Len(t, emitted, 1)
	require.NoError(t, dbMock.ExpectationsWereMet())
}

func TestSchemaWorkerQueryTimeoutPreservesOtherContainers(t *testing.T) {
	c, _, dbMock, closeDB := newSchemaCheck(t)
	defer closeDB()
	views := false
	c.config.Schemas.CollectViews = &views
	var emitted [][]byte
	c.schemaEmitter = func(payload []byte) { emitted = append(emitted, payload) }
	dbMock.ExpectQuery(`v\$containers`).WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "PDB1").AddRow(4, "PDB2"))
	dbMock.ExpectQuery("cdb_users").WillReturnRows(sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104).AddRow(4, "APP", 104))
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnError(context.DeadlineExceeded)
	dbMock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identityRows())
	require.ErrorIs(t, c.schemaCollection(context.Background()), context.DeadlineExceeded)
	require.Len(t, emitted, 1)
	var event schemaEvent
	require.NoError(t, json.Unmarshal(emitted[0], &event))
	require.Equal(t, "4", event.Metadata[0].ID)
	require.Equal(t, 1, event.CollectionPayloadsCount)
	require.NoError(t, dbMock.ExpectationsWereMet())
}
