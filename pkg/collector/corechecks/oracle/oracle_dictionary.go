// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getFullSQLText(c *Check, SQLStatement *string, key string, value string) error {
	/*
	 * Due to the Oracle bug "V$SQLSTATS.SQL_FULLTEXT does not show full of sql statements (Doc ID 2398100.1)
	 * we must retrieve `sql_fulltext` from `v$sql`.
	 */
	if c.db == nil {
		if err := reconnectOnConnectionLose(c); err != nil {
			return err
		}
	}
	query := fmt.Sprintf("SELECT /* DD */ sql_fulltext FROM v$sql WHERE %s = :v AND rownum = 1", key)
	ctx, cancel := context.WithTimeout(context.Background(), c.config.QueryTimeoutDuration())
	defer cancel()
	err := c.db.GetContext(ctx, SQLStatement, query, value)
	err = operationError(ctx, err)
	if errors.Is(err, sql.ErrNoRows) && !isConnectionError(err) {
		log.Warnf("%s The SQL text for the statement %s = %s couldn't be fetched because the SQL was evicted from shared pool", c.logPrompt, key, value)
		return nil
	}
	return handleError(c, &c.db, err)
}
