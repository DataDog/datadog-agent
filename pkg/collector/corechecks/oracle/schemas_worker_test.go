// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle_test

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/benbjohnson/clock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
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

func TestSchemaParallelWindowsKeepOrderedSnapshots(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "failure_then_retry"}[failFirst], func(t *testing.T) {
			c, _, _, cleanup := newSchemaCheck(t)
			defer cleanup()
			keys := schemaHydrationKeys(4001, "APP", 3)
			selected := make(map[tableKey]struct{}, len(keys))
			for _, key := range keys {
				selected[key] = struct{}{}
			}
			owners := map[ownerKey]string{{conID: 3, owner: "APP"}: "104"}
			for attempt := range 2 {
				fail := failFirst && attempt == 0
				var queryers []schemaQueryer
				var mocks []sqlmock.Sqlmock
				for worker := range schemaWindowWorkers {
					db, mock, err := sqlmock.New()
					require.NoError(t, err)
					t.Cleanup(func() { _ = db.Close() })
					queryers = append(queryers, sqlx.NewDb(db, "sqlmock"))
					mocks = append(mocks, mock)
					for window := worker; window*schemaConstraintPageSize < len(keys); window += schemaWindowWorkers {
						start, end := window*schemaConstraintPageSize, min((window+1)*schemaConstraintPageSize, len(keys))
						mock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
						if fail && worker == 1 {
							mock.ExpectQuery("WITH ranked_columns").WillReturnError(context.DeadlineExceeded)
							break
						}
						for page := start; page < end; page += schemaRelationPageSize {
							last := min(page+schemaRelationPageSize, end)
							expectSchemaHydrationRows(mock, keys[page:last], func(rows *sqlmock.Rows, _ int, key tableKey) {
								addTableWithoutColumns(rows, key.conID, key.owner, key.table)
							})
							for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments", "cdb_indexes"} {
								mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
							}
						}
					}
				}
				var events []schemaEvent
				coordinator := newSchemaSnapshotCoordinator(c.nextSnapshotID, func(payload []byte) {
					var event schemaEvent
					require.NoError(t, json.Unmarshal(payload, &event))
					events = append(events, event)
				})
				total, err := c.schemaTableWindowsOnConnections(context.Background(), keys, selected, owners, map[int64]string{3: "APP_PDB"}, nil, queryers, func(event schemaEvent) error {
					coordinator.add(event)
					return coordinator.err
				})
				if fail {
					require.ErrorIs(t, err, context.DeadlineExceeded)
					require.NoError(t, coordinator.abortContainer(3))
					for _, event := range events {
						require.Zero(t, event.CollectionPayloadsCount)
					}
					continue
				}
				require.NoError(t, err)
				require.Equal(t, len(keys), total)
				require.NoError(t, coordinator.complete())
				require.Len(t, events, 41)
				var tables []string
				for i, event := range events {
					require.Equal(t, events[0].CollectionStartedAt, event.CollectionStartedAt)
					if i < len(events)-1 {
						require.Zero(t, event.CollectionPayloadsCount)
					} else {
						require.Equal(t, len(events), event.CollectionPayloadsCount)
					}
					for _, table := range event.Metadata[0].Schemas[0].Tables {
						tables = append(tables, table.Name)
					}
				}
				require.Len(t, tables, len(keys))
				for i, key := range keys {
					require.Equal(t, key.table, tables[i])
				}
				for _, mock := range mocks {
					require.NoError(t, mock.ExpectationsWereMet())
				}
			}
		})
	}
}

func TestSchemaParallelConnectionAcquisitionCancellation(t *testing.T) {
	c, pool, mock, cleanup := newSchemaCheck(t)
	defer cleanup()
	conn, err := pool.Connx(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	c.schemaPool, c.schemaQueryer = pool, conn
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.schemaTableWindows(ctx, schemaHydrationKeys(4001, "APP", 3), nil, nil, nil, nil, func(schemaEvent) error { return errors.New("unexpected event") })
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, pool.Stats().InUse)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSchemaParallelConnectionAcquisitionCancellationWhileWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer db.Close()
		pool := sqlx.NewDb(db, "sqlmock")
		pool.SetMaxOpenConns(2)
		conn, err := pool.Connx(context.Background())
		require.NoError(t, err)
		defer conn.Close()
		c := Check{config: &config.CheckConfig{}, schemaPool: pool, schemaQueryer: conn}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := c.schemaTableWindows(ctx, schemaHydrationKeys(4001, "APP", 3), nil, nil, nil, nil, func(schemaEvent) error { return errors.New("unexpected event") })
			done <- err
		}()
		synctest.Wait()
		require.EqualValues(t, 1, pool.Stats().WaitCount)
		require.Equal(t, 2, pool.Stats().InUse)
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		require.Equal(t, 1, pool.Stats().InUse)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestSchemaParallelConnectionFallback(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		tables      int
		connections int
		closePool   bool
		waits       int64
	}{
		{"closed_pool", 1001, 1, true, 0},
		{"reserved_connection_only", 1001, 1, false, 1},
		{"partially_available_pool", 2001, 2, false, 1},
		{"two_windows_need_two_connections", 1001, 2, false, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer db.Close()
				mock.MatchExpectationsInOrder(false)
				pool := sqlx.NewDb(db, "sqlmock")
				pool.SetMaxOpenConns(scenario.connections)
				conn, err := pool.Connx(context.Background())
				require.NoError(t, err)
				defer conn.Close()
				c := Check{config: &config.CheckConfig{}, clock: clock.New(), dbVersion: "23.0.0.0.0", schemaPool: pool, schemaQueryer: conn}
				c.config.QueryTimeout = 1
				c.config.Schemas.MaxTables = scenario.tables
				c.config.Schemas.MaxColumns = 100
				c.config.Schemas.MaxQueryDuration = 60
				views := false
				c.config.Schemas.CollectViews = &views
				keys := schemaHydrationKeys(scenario.tables, "APP", 3)
				mock.ExpectQuery(regexp.QuoteMeta(containerNamesQuery)).WillReturnRows(
					sqlmock.NewRows([]string{"CON_ID", "NAME"}).AddRow(3, "APP_PDB"))
				mock.ExpectQuery("cdb_users").WillReturnRows(
					sqlmock.NewRows([]string{"CON_ID", "USERNAME", "USER_ID"}).AddRow(3, "APP", 104))
				identities := identityRows()
				for _, key := range keys {
					identities.AddRow(key.conID, key.owner, key.table)
				}
				mock.ExpectQuery("SELECT con_id, owner, table_name").WillReturnRows(identities)
				for start := 0; start < len(keys); start += schemaConstraintPageSize {
					mock.ExpectQuery("FROM cdb_constraints").WillReturnRows(emptyConstraintRows())
				}
				for start := 0; start < len(keys); start += schemaRelationPageSize {
					page := keys[start:min(start+schemaRelationPageSize, len(keys))]
					rows := emptyTablesRows()
					names := make([]string, len(page))
					for i, key := range page {
						addTableWithoutColumns(rows, key.conID, key.owner, key.table)
						names[i] = key.table
					}
					args := append(schemaRelationArgs(3, "APP", names...), schemaBindTestArgs("pageOwner", []string{"APP"}, schemaRelationPageSize)...)
					mock.ExpectQuery("WITH ranked_columns").WithArgs(append([]driver.Value{sql.Named("max_columns", 100)}, args...)...).WillReturnRows(rows).RowsWillBeClosed()
					mock.ExpectQuery(regexp.QuoteMeta("c.data_default_vc AS data_default")).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
					for _, view := range []string{"cdb_blockchain_tables", "cdb_immutable_tables", "cdb_mviews", "cdb_tab_comments", "cdb_col_comments", "cdb_indexes"} {
						mock.ExpectQuery("FROM " + view).WillReturnRows(sqlmock.NewRows([]string{"UNUSED"}))
					}
				}
				expectSchemaContainerAvailable(mock, 3)
				if scenario.closePool {
					mock.ExpectClose()
					require.NoError(t, pool.Close())
				}
				var events []schemaEvent
				c.schemaEmitter = func(payload []byte) {
					var event schemaEvent
					require.NoError(t, json.Unmarshal(payload, &event))
					events = append(events, event)
				}
				require.NoError(t, c.schemaCollection(context.Background()))
				require.NotEmpty(t, events)
				var observed []tableKey
				for i, event := range events {
					require.Equal(t, events[0].CollectionStartedAt, event.CollectionStartedAt)
					if i == len(events)-1 {
						require.Equal(t, len(events), event.CollectionPayloadsCount)
					} else {
						require.Zero(t, event.CollectionPayloadsCount)
					}
					for _, schema := range event.Metadata[0].Schemas {
						for _, table := range schema.Tables {
							observed = append(observed, tableKey{conID: 3, owner: schema.Owner, table: table.Name})
						}
					}
				}
				require.Equal(t, keys, observed)
				require.Equal(t, scenario.waits, pool.Stats().WaitCount)
				require.Equal(t, 1, pool.Stats().InUse)
				require.NoError(t, conn.Close())
				require.NoError(t, mock.ExpectationsWereMet())
			})
		})
	}
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
	expectSchemaContainerAvailable(dbMock, 3)
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
	expectSchemaContainerAvailable(dbMock, 3)
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
	expectSchemaContainerAvailable(dbMock, 3)
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
	c.tableDetailsForPage(context.Background(), map[tableKey]struct{}{{conID: 3, owner: "APP", table: "T"}: {}}, nil)
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
	expectSchemaContainerAvailable(dbMock, 3)
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
	expectSchemaContainerAvailable(dbMock, 4)
	require.ErrorIs(t, c.schemaCollection(context.Background()), context.DeadlineExceeded)
	require.Len(t, emitted, 1)
	var event schemaEvent
	require.NoError(t, json.Unmarshal(emitted[0], &event))
	require.Equal(t, "4", event.Metadata[0].ID)
	require.Equal(t, 1, event.CollectionPayloadsCount)
	require.NoError(t, dbMock.ExpectationsWereMet())
}
