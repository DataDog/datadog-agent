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
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	"github.com/sijms/go-ora/v2/network"
)

func selectWrapper[T any](c *Check, s T, sql string, binds ...interface{}) error {
	if c.db == nil {
		// Reconnect if the connection is lost
		// If reconnect fails, return the error
		err := reconnectOnConnectionLose(c)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
	defer cancel()
	err := c.db.SelectContext(ctx, s, sql, binds...)
	err = operationError(ctx, err)
	err = handleError(c, &c.db, err)
	if err != nil {
		err = fmt.Errorf("%w %s", err, sql)
	}
	return err
}

func getWrapper[T any](c *Check, s T, sql string, binds ...interface{}) error {
	if c.db == nil {
		err := reconnectOnConnectionLose(c)
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
	defer cancel()
	err := c.db.GetContext(ctx, s, sql, binds...)
	err = operationError(ctx, err)
	err = handleError(c, &c.db, err)
	if err != nil {
		err = fmt.Errorf("%w %s", err, sql)
	}
	return err
}

func handleError(c *Check, db **sqlx.DB, err error) error {
	if err == nil {
		return err
	}
	needsReconnect := shouldReconnect(err)
	if !needsReconnect {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		isPrivilegeError, privilegeErr := handlePrivilegeError(c, err)
		if isPrivilegeError {
			return privilegeErr
		}
	}
	isConnectionRefused, err := handleRefusedConnection(c, db, err)
	if err != nil && isConnectionRefused {
		return err
	}
	if needsReconnect {
		replacePool(c, db)
	}
	return err
}

func handlePrivilegeError(c *Check, err error) (bool, error) {
	var isPrivilegeError bool
	if err == nil {
		return isPrivilegeError, err
	}
	if !strings.Contains(err.Error(), "ORA-00942") {
		return isPrivilegeError, err
	}

	hostingType := c.hostingType
	if hostingType == "" {
		hostingType = selfManaged
	}

	var link string
	switch hostingType {
	case selfManaged:
		if c.multitenant {
			link = "https://docs.datadoghq.com/database_monitoring/setup_oracle/selfhosted/?tab=multitenant#grant-permissions"
		} else {
			link = "https://docs.datadoghq.com/database_monitoring/setup_oracle/selfhosted/?tab=noncdb#grant-permissions"
		}
	case rds:
		link = "https://docs.datadoghq.com/database_monitoring/setup_oracle/rds/#grant-permissions"
	case oci:
		link = "https://docs.datadoghq.com/database_monitoring/setup_oracle/autonomous_database/#grant-permissions"
	}

	isPrivilegeError = true
	return isPrivilegeError, fmt.Errorf("Some privileges are missing. Execute the `grant` commands from %s . Error: %w", link, err)
}

func shouldReconnect(err error) bool {
	if err == nil {
		return false
	}
	// Our deadline decides whether an operation was abandoned, even if the
	// driver returned a different error after cancellation.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, network.ErrConnReset) {
		return true
	}
	// A cleanup failure must still trigger recovery when the query also failed.
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, cause := range wrapped.Unwrap() {
			if shouldReconnect(cause) {
				return true
			}
		}
	case interface{ Unwrap() error }:
		if shouldReconnect(wrapped.Unwrap()) {
			return true
		}
	}
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	switch cause := err.(type) {
	case *net.OpError:
		return cause.Timeout() || cause.Op == "read" || cause.Op == "write"
	case net.Error:
		return cause.Timeout()
	case *network.OracleError:
		return cause.Bad() || shouldResetOracleSession(cause.ErrCode)
	case interface{ Code() int }:
		return godror.IsBadConn(err) || shouldResetOracleSession(cause.Code())
	}
	// Neither of these errors has an exported type or sentinel in the pinned APIs.
	return strings.HasPrefix(err.Error(), "TTC error:") || err.Error() == "sql: database is closed"
}

func shouldResetOracleSession(code int) bool {
	switch code {
	// 1013: go-ora/godror operation cancellation.
	// 3156: godror OCI call timeout, preserved by ODPI-C.
	// 12161: godror partial data received; ODPI-C can report this after a call timeout.
	case 1013, 3156, 12161:
		return true
	// 3106: go-ora fatal two-task communication protocol error; godror.IsBadConn also covers it.
	// 3146: go-ora/godror invalid buffer length in a TTC protocol packet.
	// 6413: go-ora/godror connection not open.
	case 3106, 3146, 6413:
		return true
	// 1000: go-ora/godror cursor exhaustion; a fresh session restores collection.
	case 1000:
		return true
	default:
		return false
	}
}

func isConnectionRefused(err error) bool {
	return strings.Contains(err.Error(), "connect: connection refused")
}

func handleRefusedConnection(c *Check, db **sqlx.DB, err error) (bool, error) {
	if err == nil {
		return false, err
	}
	if isConnectionRefused(err) {
		disposePool(c, db)
		return true, fmt.Errorf(`%w
The network connection between the Agent and the database server is disrupted. Run one of the following commands on the machine where the Agent is running to test the network connection:
nc -v dbserver port
telnet dbserver port
curl dbserver:port`,
			err)
	}
	return false, err
}

func replacePool(c *Check, db **sqlx.DB) {
	log.Debugf("%s Reconnecting", c.logPrompt)
	disposePool(c, db)
	replacement, connectErr := c.Connect()
	if connectErr != nil {
		closeDatabase(c, replacement)
		log.Errorf("%s failed to reconnect %s", c.logPrompt, connectErr)
		return
	}
	*db = replacement
}

func reconnectOnConnectionLose(c *Check) error {
	disposePool(c, &c.db)
	db, err := c.Connect()
	if err != nil {
		closeDatabase(c, db)
		return err
	}
	c.db = db
	return nil
}

func disposePool(c *Check, db **sqlx.DB) {
	old := *db
	*db = nil
	closeDatabase(c, old)
}

// Preserve a deadline even when the driver reports a transport/protocol error
// after cancellation. Recovery always creates its own fresh operation context.
func operationError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err)
	}
	return err
}

func execWithDeadline(c *Check, db *sqlx.DB, statement string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
	defer cancel()
	_, err := db.ExecContext(ctx, statement)
	return operationError(ctx, err)
}
