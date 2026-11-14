// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/jmoiron/sqlx"
	goora "github.com/sijms/go-ora/v2"
	"github.com/stretchr/testify/require"
)

type operationConnector struct {
	connect func(context.Context) (driver.Conn, error)
}

func (d operationConnector) Connect(ctx context.Context) (driver.Conn, error) { return d.connect(ctx) }
func (operationConnector) Driver() driver.Driver                              { return nil }

type operationConn struct {
	query func(context.Context, string) (driver.Rows, error)
	exec  func(context.Context, string) (driver.Result, error)
	close func() error
}

func (operationConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (operationConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c operationConn) Close() error {
	if c.close != nil {
		return c.close()
	}
	return nil
}
func (c operationConn) ExecContext(ctx context.Context, statement string, _ []driver.NamedValue) (driver.Result, error) {
	if c.exec == nil {
		return nil, errors.New("unexpected exec")
	}
	return c.exec(ctx, statement)
}
func (c operationConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query)
}

type operationRows struct {
	read     bool
	fetchErr error
	closeErr error
	closed   bool
}

func (*operationRows) Columns() []string { return []string{"VALUE"} }
func (r *operationRows) Close() error    { r.closed = true; return r.closeErr }
func (r *operationRows) Next(values []driver.Value) error {
	if r.read {
		if r.fetchErr != nil {
			return r.fetchErr
		}
		return io.EOF
	}
	r.read = true
	values[0] = int64(42)
	return nil
}

func operationPool(t *testing.T, name string, conn operationConn) *sqlx.DB {
	t.Helper()
	db := sqlx.NewDb(sql.OpenDB(operationConnector{connect: func(context.Context) (driver.Conn, error) { return conn, nil }}), name)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func operationCheck(t *testing.T) (*Check, *mocksender.MockSender) {
	t.Helper()
	c := newCheck().(*Check)
	manager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.Configure(manager, integration.FakeConfigHash, []byte(`server: 127.0.0.1
port: 1
service_name: unused
username: unused
password: unused
query_timeout: 1
only_custom_queries: true
propagate_agent_tags: false
`), []byte("{}"), "test", ""))
	sender := mocksender.NewMockSenderWithSenderManager(c.ID(), manager)
	sender.SetupAcceptAll()
	c.initialized, c.connectedToPdb = true, true
	t.Cleanup(c.Teardown)
	return c, sender
}

func TestRecoveryErrorClassification(t *testing.T) {
	for _, message := range []string{"DPI-1067: call timeout exceeded", "DPI-1080: connection was closed", "ORA-01000: maximum open cursors exceeded", "ORA-01013: user requested cancel", "ORA-03106: fatal two-task communication protocol error", "ORA-03113", "ORA-03114", "ORA-03135", "TTC error: received code 9 during response reading", "read tcp: i/o timeout", "ORA-00028", "ORA-01012", "ORA-06413", "ORA-12514", "database is closed", "bad connection", "connection was aborted"} {
		require.True(t, isConnectionError(errors.New(message)), message)
	}
	require.True(t, isConnectionError(&net.OpError{Op: "read", Err: context.DeadlineExceeded}))
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, io.EOF, io.ErrUnexpectedEOF} {
		require.True(t, isConnectionError(errors.Join(errors.New("query"), err)))
	}
	for _, err := range []error{nil, sql.ErrNoRows, errors.New("ORA-00942: table does not exist"), errors.New("ORA-00904: invalid identifier"), errors.New("ordinary SQL error")} {
		require.False(t, isConnectionError(err))
	}
}

func TestOperationDeadlinePreservesDriverError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _ := operationCheck(t)
		// Make the recovery open fail during parsing, without real network I/O in
		// the virtual-time test. Live tests cover a refused service separately.
		c.config.Protocol, c.config.Wallet = "TCPS", t.TempDir()
		driverErr := errors.New("ORA-00942: different error after cancellation")
		var closed atomic.Int32
		c.db = operationPool(t, common.GoOra, operationConn{
			query: func(ctx context.Context, _ string) (driver.Rows, error) {
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.Equal(t, time.Second, time.Until(deadline))
				<-ctx.Done()
				return nil, driverErr
			}, close: func() error { closed.Add(1); return nil },
		})
		var value int
		err := getWrapper(c, &value, "SELECT slow FROM dual")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, err, driverErr)
		require.EqualValues(t, 1, closed.Load())
		require.Nil(t, c.db, "failed reconnect must clear the affected pool")
		c.db = operationPool(t, common.GoOra, operationConn{query: func(ctx context.Context, _ string) (driver.Rows, error) {
			require.NoError(t, ctx.Err())
			return &operationRows{}, nil
		}})
		require.NoError(t, getWrapper(c, &value, "SELECT later FROM dual"))
		require.Equal(t, 42, value)
	})
}

func TestOrdinaryErrorsPreservePool(t *testing.T) {
	for _, driverName := range []string{common.GoOra, common.Godror} {
		for _, queryErr := range []error{sql.ErrNoRows, errors.New("ORA-00904: invalid identifier"), errors.New("ORA-00942: table does not exist")} {
			t.Run(driverName+"/"+queryErr.Error(), func(t *testing.T) {
				c, _ := operationCheck(t)
				db := operationPool(t, driverName, operationConn{query: func(context.Context, string) (driver.Rows, error) { return nil, queryErr }})
				c.db = db
				var value int
				err := getWrapper(c, &value, "SELECT 1 FROM dual")
				if errors.Is(queryErr, sql.ErrNoRows) {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, queryErr)
				}
				if queryErr.Error() == "ORA-00942: table does not exist" {
					require.Contains(t, err.Error(), "grant")
				}
				require.Same(t, db, c.db)
			})
		}
	}
}

func TestCustomFetchAndCloseErrorsRecover(t *testing.T) {
	for _, driverName := range []string{common.GoOra, common.Godror} {
		for _, phase := range []string{"fetch", "close", "scan and close"} {
			t.Run(driverName+"/"+phase, func(t *testing.T) {
				c, sender := operationCheck(t)
				recoveryErr := errors.New("ORA-01000: maximum open cursors exceeded")
				rows := &operationRows{}
				if phase == "fetch" {
					rows.fetchErr = recoveryErr
				} else {
					rows.closeErr = recoveryErr
				}
				columns := []config.CustomQueryColumns{{Name: "value", Type: "gauge"}}
				if phase == "scan and close" {
					columns = nil
				}
				var closed atomic.Int32
				c.dbCustomQueries = operationPool(t, driverName, operationConn{query: func(context.Context, string) (driver.Rows, error) { return rows, nil }, close: func() error { require.True(t, rows.closed); closed.Add(1); return nil }})
				c.config.InstanceConfig.CustomQueries = []config.CustomQuery{{Query: "SELECT value FROM dual", MetricPrefix: "failed", Columns: columns}}
				err := c.CustomQueries()
				require.ErrorIs(t, err, recoveryErr)
				if phase == "scan and close" {
					require.Contains(t, err.Error(), "column count mismatch")
				}
				require.Nil(t, c.dbCustomQueries)
				require.EqualValues(t, 1, closed.Load())
				sender.AssertNotCalled(t, "Gauge")
			})
		}
	}
}

func TestRefusedConnectionAndTeardownClearReferences(t *testing.T) {
	c, _ := operationCheck(t)
	db := operationPool(t, common.GoOra, operationConn{query: func(context.Context, string) (driver.Rows, error) { return &operationRows{}, nil }})
	c.db, c.dbCustomQueries = db, db
	require.Error(t, handleError(c, &c.db, errors.New("dial tcp: connect: connection refused")))
	require.Nil(t, c.db)
	c.Teardown()
	c.Teardown()
	require.Nil(t, c.db)
	require.Nil(t, c.dbCustomQueries)
}

func TestSocketTimeoutIsTwiceOperationDeadline(t *testing.T) {
	c, _ := operationCheck(t)
	for _, configured := range []int{0, -1, 1, 20, int(^uint(0) >> 1)} {
		c.config.QueryTimeout = configured
		options, err := goora.ParseConfig(buildGoOraURL(c))
		require.NoError(t, err)
		require.Equal(t, 2*c.config.QueryTimeoutDuration(), options.Timeout)
	}
}

func TestBuiltInPathsUseRecovery(t *testing.T) {
	operations := map[string]func(*Check) error{
		"select": func(c *Check) error {
			var rows []struct{ Value int }
			return selectWrapper(c, &rows, "SELECT value FROM dual")
		},
		"full SQL text": func(c *Check) error { var text string; return getFullSQLText(c, &text, "sql_id", "test") },
		"health check":  func(c *Check) error { return c.connectionTest() },
		"shared memory": func(c *Check) error { return c.SharedMemory() },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			c, _ := operationCheck(t)
			c.config.Protocol, c.config.Wallet = "TCPS", t.TempDir()
			c.dbVersion = "18.0.0.0.0"
			failure := errors.New("ORA-01013: operation canceled")
			var closed atomic.Int32
			c.db = operationPool(t, common.GoOra, operationConn{
				query: func(ctx context.Context, _ string) (driver.Rows, error) {
					_, hasDeadline := ctx.Deadline()
					require.True(t, hasDeadline)
					return nil, failure
				},
				close: func() error { closed.Add(1); return nil },
			})
			require.ErrorIs(t, operation(c), failure)
			require.Nil(t, c.db)
			require.EqualValues(t, 1, closed.Load())
		})
	}
}

func TestSetupStatementsHaveIndependentDeadlines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, _ := operationCheck(t)
		calls := 0
		failure := errors.New("driver returned another error after deadline")
		db := operationPool(t, common.GoOra, operationConn{exec: func(ctx context.Context, _ string) (driver.Result, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.Equal(t, time.Second, time.Until(deadline))
			calls++
			if calls == 1 {
				<-ctx.Done()
				return nil, failure
			}
			require.NoError(t, ctx.Err())
			return driver.RowsAffected(1), nil
		}})
		err := execWithDeadline(c, db, "first setup statement")
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorIs(t, err, failure)
		require.NoError(t, execWithDeadline(c, db, "later setup statement"))
	})
}
