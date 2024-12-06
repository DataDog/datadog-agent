// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/jmoiron/sqlx"
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
	err := c.db.Select(s, sql, binds...)
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
	err := c.db.Get(s, sql, binds...)
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
	if strings.Contains(err.Error(), "no rows in result") {
		return nil
	}
	isPrivilegeError, err := handlePrivilegeError(c, err)
	if err != nil && isPrivilegeError {
		return err
	}
	isConnectionRefused, err := handleRefusedConnection(c, *db, err)
	if err != nil && isConnectionRefused {
		return err
	}
	reconnectOnConnectionError(c, db, err)
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

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	connectionErrors := []string{"ORA-00028", "ORA-01012", "ORA-06413", "ORA-12514", "database is closed", "bad connection", "connection was aborted"}
	for _, e := range connectionErrors {
		if strings.Contains(err.Error(), e) {
			return true
		}
	}
	return false
}

func isConnectionRefused(err error) bool {
	return strings.Contains(err.Error(), "connect: connection refused")
}

func handleRefusedConnection(c *Check, db *sqlx.DB, err error) (bool, error) {
	if err == nil {
		return false, err
	}
	if isConnectionRefused(err) {
		closeDatabase(c, db)
		return true, fmt.Errorf(`%w
The network connection between the Agent and the database server is disrupted. Run one of the following commands on the machine where the Agent is running to test the network connection:
nc -v dbserver port
telnet dbserver port
curl dbserver:port`,
			err)
	}
	return false, err
}

func reconnectOnConnectionError(c *Check, db **sqlx.DB, err error) {
	if !isConnectionError(err) {
		return
	}
	log.Debugf("%s Reconnecting", c.logPrompt)
	if *db != nil {
		closeDatabase(c, *db)
	}
	*db, err = c.Connect()
	if err != nil {
		log.Errorf("%s failed to reconnect %s", c.logPrompt, err)
		closeDatabase(c, *db)
	}
}

func reconnectOnConnectionLose(c *Check) error {
	db, err := c.Connect()
	if err != nil {
		log.Errorf("%s failed to reconnect %s", c.logPrompt, err)
		closeDatabase(c, c.db)
	}
	c.db = db
	return err
}
