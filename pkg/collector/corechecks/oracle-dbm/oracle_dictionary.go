// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	go_ora "github.com/sijms/go-ora/v2"
)

func getFullSQLText(c *Check, SQLStatement *string, key string, value string) error {
	var err error
	var sql string
	switch c.driver {
	case common.Godror:
		sql = fmt.Sprintf("SELECT /* DD */ sql_fulltext FROM v$sql WHERE %s = :v AND rownum = 1", key)
		err = c.db.Get(SQLStatement, sql, value)
		reconnectOnConnectionError(c, &c.db, err)
	case common.GoOra:
		var sqlFullText go_ora.Clob
		sql = fmt.Sprintf("BEGIN SELECT /* DD */ sql_fulltext INTO :sql_fulltext FROM v$sql WHERE %s = :v AND rownum = 1; END;", key)
		_, err = c.connection.Exec(sql, go_ora.Out{Dest: &sqlFullText, Size: 8000}, value)
		if err == nil && sqlFullText.Valid && sqlFullText.String != "" {
			*SQLStatement = sqlFullText.String
		} else if err != nil {
			if !isConnectionError(err) {
				return err
			}
			log.Debugf("Reconnecting")
			if c.connection != nil {
				closeGoOraConnection(c)
			}
			conn, errConnect := connectGoOra(c)
			if errConnect != nil {
				log.Errorf("failed to reconnect %s", err)
				closeGoOraConnection(c)
			} else {
				c.connection = conn
			}
			return fmt.Errorf("failed to query sql full text for %s = %s %s", key, value, err)
		} else if sqlFullText.String == "" {
			return fmt.Errorf("no rows for %s = %s", key, value)
		}
	}
	return err
}
