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
)

func getFullSQLText(c *Check, SQLStatement *string, key string, value string) error {
	/*
	 * Due to the Oracle bug "V$SQLSTATS.SQL_FULLTEXT does not show full of sql statements (Doc ID 2398100.1)
	 * we must retrieve `sql_fulltext` from `v$sql`.
	 *
	 * We use DBMS_LOB.SUBSTR to convert the CLOB to VARCHAR2 at the SQL level,
	 * which avoids the need for a dedicated go-ora LOB connection (see go-ora#439).
	 */
	sql := fmt.Sprintf("SELECT /* DD */ dbms_lob.substr(sql_fulltext, %d, 1) sql_fulltext FROM v$sql WHERE %s = :v AND rownum = 1", MaxSQLFullTextVSQL, key)
	err := c.db.Get(SQLStatement, sql, value)
	reconnectOnConnectionError(c, &c.db, err)
	if err != nil && strings.Contains(err.Error(), "no rows") {
		log.Warnf("%s The SQL text for the statement %s = %s couldn't be fetched because the SQL was evicted from shared pool", c.logPrompt, key, value)
		err = nil
	}
	return err
}
