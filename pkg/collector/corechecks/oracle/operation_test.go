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
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/jmoiron/sqlx"
	goora "github.com/sijms/go-ora/v2"
	"github.com/sijms/go-ora/v2/network"
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

type codedOracleError int

func (e codedOracleError) Code() int     { return int(e) }
func (e codedOracleError) Error() string { return fmt.Sprintf("native Oracle error %d", int(e)) }

func TestRecoveryErrorClassification(t *testing.T) {
	for _, code := range []int{28, 1012, 3113, 3114, 3135, 1000, 1013, 3106, 3146, 3156, 6413, 12161, 1033, 1089} {
		for _, err := range []error{network.NewOracleError(code), codedOracleError(code)} {
			require.True(t, shouldReconnect(err), "%T code %d", err, code)
			require.True(t, shouldReconnect(fmt.Errorf("query: %w", err)), "%T code %d", err, code)
		}
	}
	for _, err := range []error{
		driver.ErrBadConn, context.DeadlineExceeded, context.Canceled, io.EOF, io.ErrUnexpectedEOF,
		net.ErrClosed, network.ErrConnReset,
		&net.DNSError{IsTimeout: true},
		&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNABORTED},
		&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
		&net.OpError{Op: "write", Net: "tcp", Err: syscall.EPIPE},
		&net.OpError{Op: "read", Net: "tcp", Err: syscall.Errno(10053)},
		&net.OpError{Op: "write", Net: "tcp", Err: errors.New("localized socket failure")},
		errors.New("TTC error: received code 9 during response reading"),
		errors.New("sql: database is closed"),
	} {
		require.True(t, shouldReconnect(err), "%v", err)
		require.True(t, shouldReconnect(fmt.Errorf("query: %w", err)), "%v", err)
	}
}

func TestOrdinaryErrorTextDoesNotTriggerRecovery(t *testing.T) {
	for _, err := range []error{
		nil, sql.ErrNoRows,
		network.NewOracleError(904), network.NewOracleError(942), codedOracleError(904), codedOracleError(942),
		&network.OracleError{ErrCode: 904, ErrMsg: "TTC error: ORA-01000 bad connection in invalid SQL"},
		&net.OpError{Op: "dial", Err: &net.DNSError{Err: "no such host"}},
		errors.New("SQL contains ORA-01000, bad connection, i/o timeout and connection was aborted"),
		errors.New("query references TTC error: and sql: database is closed"),
	} {
		require.False(t, shouldReconnect(err), "%v", err)
	}
}

func TestJoinedCleanupErrorsTriggerRecovery(t *testing.T) {
	for _, ordinary := range []error{network.NewOracleError(942), codedOracleError(942)} {
		for _, cleanup := range []error{network.NewOracleError(6413), codedOracleError(3113), io.EOF} {
			require.True(t, shouldReconnect(errors.Join(ordinary, fmt.Errorf("close: %w", cleanup))))
			require.True(t, shouldReconnect(errors.Join(cleanup, ordinary)))
		}
	}
	require.False(t, shouldReconnect(errors.Join(codedOracleError(942), network.NewOracleError(904))))
}

func TestRecoveryDoesNotReplayFailedQuery(t *testing.T) {
	for _, driverName := range []string{common.GoOra, common.Godror} {
		t.Run(driverName, func(t *testing.T) {
			c, _ := operationCheck(t)
			c.config.Protocol, c.config.Wallet = "TCPS", t.TempDir()
			var failure error = network.NewOracleError(1000)
			if driverName == common.Godror {
				failure = codedOracleError(1000)
			}
			calls := 0
			c.db = operationPool(t, driverName, operationConn{query: func(context.Context, string) (driver.Rows, error) { calls++; return nil, failure }})
			var value int
			err := getWrapper(c, &value, "SELECT value FROM dual")
			require.ErrorIs(t, err, failure)
			require.NotErrorIs(t, err, driver.ErrBadConn)
			require.Equal(t, 1, calls)
			require.Nil(t, c.db)
		})
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
		for _, queryErr := range []error{sql.ErrNoRows, errors.New("ORA-00904: invalid identifier"), errors.New("ORA-00942: table does not exist"), network.NewOracleError(12514), codedOracleError(12514)} {
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
				var recoveryErr error = network.NewOracleError(1000)
				if driverName == common.Godror {
					recoveryErr = codedOracleError(1000)
				}
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
			failure := network.NewOracleError(1013)
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
