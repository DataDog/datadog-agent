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
	conn, err := d.Dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	a := ctx.Value(connectAttemptKey{}).(*connectAttempt)
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
	attempt := &connectAttempt{}
	stop := context.AfterFunc(ctx, attempt.close)
	defer stop()
	conn, err := c.Connector.Connect(context.WithValue(ctx, connectAttemptKey{}, attempt))
	if ctxErr := ctx.Err(); ctxErr != nil {
		err = ctxErr
	}
	if err != nil {
		attempt.close()
		if conn != nil {
			_ = conn.Close()
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

// go-ora only watches ctx until QueryContext returns, not while Next fetches
// subsequent batches. A canceled check must also interrupt those reads. Close
// the socket rather than calling driver.Conn.Close concurrently with Next;
// database/sql and the run owner perform the driver/pool cleanup afterwards.
func (c *cancelableGoOraConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Connection.QueryContext(ctx, query, args)
	if err != nil {
		return rows, err
	}
	dataSet, ok := rows.(*go_ora.DataSet)
	if !ok {
		return rows, nil
	}
	stop := context.AfterFunc(ctx, func() {
		for _, socket := range c.sockets {
			_ = socket.Close()
		}
	})
	return &cancelableGoOraRows{DataSet: dataSet, stop: stop}, nil
}

// Embedding DataSet preserves the driver's column metadata and result-set APIs.
type cancelableGoOraRows struct {
	*go_ora.DataSet
	stop func() bool
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
