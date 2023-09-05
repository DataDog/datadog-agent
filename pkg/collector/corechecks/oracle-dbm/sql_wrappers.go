// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	"strings"
)

func selectWrapper[T any](c *Check, s T, sql string, binds ...interface{}) error {
	err := c.db.Select(s, sql, binds...)
	reconnectOnConnectionError(c, &c.db, err)
	return err
}

func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	connectionErrors := []string{"ORA-00028", "ORA-01012", "ORA-06413", "database is closed"}
	for _, e := range connectionErrors {
		if strings.Contains(err.Error(), e) {
			return true
		}
	}
	return false
}

func reconnectOnConnectionError(c *Check, db **sqlx.DB, err error) {
	if !isConnectionError(err) {
		return
	}
	log.Debugf("Reconnecting")
	if *db != nil {
		closeDatabase(c, *db)
	}
	*db, err = c.Connect()
	if err != nil {
		log.Errorf("failed to reconnect %s", err)
		closeDatabase(c, *db)
	}
}
