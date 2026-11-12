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
	"sync"
	"sync/atomic"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type lifecycleConnector struct {
	connect func(context.Context) (driver.Conn, error)
}

func (d lifecycleConnector) Connect(ctx context.Context) (driver.Conn, error) { return d.connect(ctx) }
func (lifecycleConnector) Driver() driver.Driver                              { return nil }

type lifecycleConn struct {
	query func(context.Context, string) (driver.Rows, error)
	close func() error
}

func (c lifecycleConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c lifecycleConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c lifecycleConn) Close() error              { return c.close() }
func (c lifecycleConn) QueryContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query)
}

type lifecycleRows struct{ read bool }

func (*lifecycleRows) Columns() []string { return []string{"VALUE"} }
func (*lifecycleRows) Close() error      { return nil }
func (r *lifecycleRows) Next(values []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	values[0] = int64(1)
	return nil
}

func lifecyclePool(t *testing.T, conn lifecycleConn) *sqlx.DB {
	t.Helper()
	db := sqlx.NewDb(sql.OpenDB(lifecycleConnector{connect: func(context.Context) (driver.Conn, error) { return conn, nil }}), common.GoOra)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func lifecycleCheck(t *testing.T) *Check {
	t.Helper()
	c := newCheck().(*Check)
	manager := mocksender.CreateDefaultDemultiplexer(t)
	require.NoError(t, c.Configure(manager, integration.FakeConfigHash, []byte(`server: 127.0.0.1
port: 1
service_name: unused
username: unused
password: unused
only_custom_queries: true
propagate_agent_tags: false
`), []byte("{}"), "test", ""))
	mocksender.NewMockSenderWithSenderManager(c.ID(), manager).SetupAcceptAll()
	c.initialized, c.connectedToPdb = true, true
	t.Cleanup(c.Teardown)
	return c
}

func healthyLifecycleConn(closed *atomic.Int32) lifecycleConn {
	return lifecycleConn{
		query: func(context.Context, string) (driver.Rows, error) { return &lifecycleRows{}, nil },
		close: func() error { closed.Add(1); return nil },
	}
}

func TestCancelIdleClosesPoolsAndRetiresCheck(t *testing.T) {
	c := lifecycleCheck(t)
	var closed atomic.Int32
	c.db = lifecyclePool(t, healthyLifecycleConn(&closed))
	c.dbCustomQueries = lifecyclePool(t, healthyLifecycleConn(&closed))
	require.NoError(t, c.db.Ping())
	require.NoError(t, c.dbCustomQueries.Ping())
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() { c.Stop(); c.Cancel() })
	}
	wg.Wait()
	<-c.lifecycle.done
	require.EqualValues(t, 2, closed.Load())
	require.Nil(t, c.db)
	require.Nil(t, c.dbCustomQueries)
	require.ErrorIs(t, c.Run(), context.Canceled)
	require.EqualValues(t, 2, closed.Load())
}

func TestCancelDoesNotWaitForDatabaseClose(t *testing.T) {
	c := lifecycleCheck(t)
	closeStarted, finishClose := make(chan struct{}), make(chan struct{})
	conn := healthyLifecycleConn(&atomic.Int32{})
	conn.close = func() error { close(closeStarted); <-finishClose; return nil }
	c.db = lifecyclePool(t, conn)
	require.NoError(t, c.db.Ping())
	c.Cancel()
	<-closeStarted
	select {
	case <-c.lifecycle.done:
		t.Fatal("cleanup completed before driver.Close returned")
	default:
	}
	close(finishClose)
	<-c.lifecycle.done
}

func TestStopIdlePreservesHealthyPool(t *testing.T) {
	c := lifecycleCheck(t)
	var closed atomic.Int32
	c.db = lifecyclePool(t, healthyLifecycleConn(&closed))
	require.NoError(t, c.Run())
	pool := c.db
	c.Stop()
	require.NoError(t, c.Run())
	require.Same(t, pool, c.db)
	require.Zero(t, closed.Load())
}

func TestHooksInterruptRunWithoutReconnecting(t *testing.T) {
	for _, retire := range []bool{false, true} {
		name := "Stop"
		if retire {
			name = "Cancel"
		}
		t.Run(name, func(t *testing.T) {
			c := lifecycleCheck(t)
			var closed, later atomic.Int32
			c.db = lifecyclePool(t, healthyLifecycleConn(&closed))
			started := make(chan struct{})
			conn := healthyLifecycleConn(&closed)
			conn.query = func(ctx context.Context, query string) (driver.Rows, error) {
				if query == "blocked" {
					close(started)
					<-ctx.Done()
					// A recognized connection error must not cause a replacement
					// once the cycle was stopped, even if it masks context.Canceled.
					return nil, errors.New("ORA-00028: your session has been killed")
				}
				later.Add(1)
				return &lifecycleRows{}, nil
			}
			c.dbCustomQueries = lifecyclePool(t, conn)
			c.config.InstanceConfig.CustomQueries = []config.CustomQuery{{Query: "blocked"}, {Query: "later"}}
			done := make(chan error, 1)
			go func() { done <- c.Run() }()
			<-started
			if retire {
				c.Cancel()
			} else {
				c.Stop()
			}
			require.ErrorIs(t, <-done, context.Canceled)
			if retire {
				<-c.lifecycle.done
			}
			require.Zero(t, later.Load())
			require.EqualValues(t, 2, closed.Load())
			require.Nil(t, c.db)
			require.Nil(t, c.dbCustomQueries)
			if retire {
				require.ErrorIs(t, c.Run(), context.Canceled)
			} else {
				// Stop ends this cycle; it must not permanently retire the check.
				c.db = lifecyclePool(t, healthyLifecycleConn(&closed))
				c.config.InstanceConfig.CustomQueries = nil
				require.NoError(t, c.Run())
			}
		})
	}
}

func TestCancelWhileOpeningConnection(t *testing.T) {
	c := lifecycleCheck(t)
	started := make(chan struct{})
	c.db = sqlx.NewDb(sql.OpenDB(lifecycleConnector{connect: func(ctx context.Context) (driver.Conn, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}), common.GoOra)
	done := make(chan error, 1)
	go func() { done <- c.Run() }()
	<-started
	c.Cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	<-c.lifecycle.done
	require.Nil(t, c.db)
}

func TestRetiredCheckDoesNotStartSQLOrRecovery(t *testing.T) {
	c := newCheck().(*Check)
	c.Cancel()
	<-c.lifecycle.done
	var value int
	var values []int
	var text string
	require.ErrorIs(t, getWrapper(c, &value, "SELECT 1 FROM dual"), context.Canceled)
	require.ErrorIs(t, selectWrapper(c, &values, "SELECT 1 FROM dual"), context.Canceled)
	require.ErrorIs(t, getFullSQLText(c, &text, "sql_id", "test"), context.Canceled)
	require.ErrorIs(t, c.CustomQueries(), context.Canceled)
	require.ErrorIs(t, c.connectionTest(), context.Canceled)
	_, err := c.Connect()
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, reconnectOnConnectionLose(c), context.Canceled)
	reconnectOnConnectionError(c, &c.db, errors.New("ORA-00028"))
	require.Nil(t, c.db)
}
