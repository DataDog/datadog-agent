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
	"net"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/jmoiron/sqlx"
	go_ora "github.com/sijms/go-ora/v2"
)

// go-ora's connector can return an error without closing the partially opened
// connection. Keep ownership of its sockets until Connect succeeds.
type cleanupConnector struct {
	driver.Connector
}

type connectAttemptKey struct{}

type connectAttempt struct {
	mu       sync.Mutex
	ctx      context.Context
	finished bool
	conns    []net.Conn
}

func (a *connectAttempt) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.finished {
		return
	}
	a.finished = true
	for _, conn := range a.conns {
		_ = conn.Close()
	}
	a.conns = nil
}

func (a *connectAttempt) transfer() []net.Conn {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.finished {
		return nil
	}
	a.finished = true
	conns := a.conns
	a.conns = nil
	return conns
}

type cleanupDialer struct {
	net.Dialer
}

func (d *cleanupDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	a := ctx.Value(connectAttemptKey{}).(*connectAttempt)
	conn, err := d.Dialer.DialContext(a.ctx, network, address)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.finished {
		_ = conn.Close()
		return nil, context.Canceled
	}
	a.conns = append(a.conns, conn)
	return conn, nil
}

func (c *cleanupConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	attempt := &connectAttempt{ctx: ctx}
	stop := context.AfterFunc(ctx, attempt.close)
	defer stop()
	conn, err := c.Connector.Connect(context.WithValue(context.WithoutCancel(ctx), connectAttemptKey{}, attempt))
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = errors.Join(ctxErr, err)
	}
	if err != nil {
		attempt.close()
		if conn != nil {
			err = errors.Join(err, conn.Close())
		}
		return nil, err
	}
	sockets := attempt.transfer()
	if oracleConn, ok := conn.(*go_ora.Connection); ok {
		return &cancelableGoOraConn{Connection: oracleConn, sockets: sockets}, nil
	}
	return conn, nil
}

type cancelableGoOraConn struct {
	*go_ora.Connection
	sockets []net.Conn
}

// go-ora v2.9.0 stops watching QueryContext before later Next calls. Own
// cancellation through Rows.Close, interrupting the socket instead of invoking
// driver.Close concurrently with a read. Do not run its asynchronous break
// watcher as well: it is not joined by EndContext and can outlive the operation.
// Closing the transport interrupts client I/O; it does not establish when Oracle
// finishes the abandoned operation. Recovery disposes of the affected pool.
func (c *cancelableGoOraConn) watch(ctx context.Context) func() {
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		for _, socket := range c.sockets {
			_ = socket.Close()
		}
	})
	return func() {
		if !stop() {
			<-done
		}
	}
}

func (c *cancelableGoOraConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	stop := c.watch(ctx)
	rows, err := c.Connection.QueryContext(context.WithoutCancel(ctx), query, args)
	if err != nil {
		// go-ora already closes the statement on this path and can return a
		// typed-nil *DataSet as driver.Rows. Do not call Close on that result.
		stop()
		return nil, err
	}
	dataSet, ok := rows.(*go_ora.DataSet)
	if !ok {
		stop()
		return rows, nil
	}
	return &cancelableGoOraRows{DataSet: dataSet, stop: stop}, nil
}

func (c *cancelableGoOraConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	stop := c.watch(ctx)
	defer stop()
	return c.Connection.ExecContext(context.WithoutCancel(ctx), query, args)
}

func (c *cancelableGoOraConn) Ping(ctx context.Context) error {
	stop := c.watch(ctx)
	defer stop()
	return c.Connection.Ping(context.WithoutCancel(ctx))
}

// Embedding DataSet preserves the driver's column metadata and result-set APIs.
type cancelableGoOraRows struct {
	*go_ora.DataSet
	stop func()
}

func (r *cancelableGoOraRows) Close() error {
	defer r.stop()
	return r.DataSet.Close()
}

func openGoOraPool(connString string) (*sqlx.DB, error) {
	options, err := go_ora.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	connector := go_ora.NewConnector(connString).(*go_ora.OracleConnector)
	connector.Dialer(&cleanupDialer{Dialer: net.Dialer{Timeout: options.ConnectTimeout}})
	return sqlx.NewDb(sql.OpenDB(&cleanupConnector{Connector: connector}), common.GoOra), nil
}
