// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/jmoiron/sqlx"
	go_ora "github.com/sijms/go-ora/v2"
)

func buildGoOraURL(c *Check) string {
	connectionOptions := map[string]string{"TIMEOUT": DB_TIMEOUT}
	if c.config.Protocol == "TCPS" {
		connectionOptions["SSL"] = "TRUE"
		if c.config.Wallet != "" {
			connectionOptions["WALLET"] = c.config.Wallet
		}
	}
	return go_ora.BuildUrl(c.config.Server, c.config.Port, c.config.ServiceName, c.config.Username, c.config.Password, connectionOptions)
}

// Connect establishes a connection to an Oracle instance and returns an open connection to the database.
func (c *Check) Connect() (*sqlx.DB, error) {

	var connStr string
	var oracleDriver string
	if c.config.TnsAlias != "" {
		connStr = fmt.Sprintf(`user="%s" password="%s" connectString="%s"`, c.config.Username, c.config.Password, c.config.TnsAlias)
		oracleDriver = "godror"
	} else {
		//godror ezconnect string
		if c.config.InstanceConfig.OracleClient {
			oracleDriver = "godror"
			protocolString := ""
			walletString := ""
			if c.config.Protocol == "TCPS" {
				protocolString = "tcps://"
				if c.config.Wallet != "" {
					walletString = fmt.Sprintf("?wallet_location=%s", c.config.Wallet)
				}
			}
			connStr = fmt.Sprintf(`user="%s" password="%s" connectString="%s%s:%d/%s%s"`, c.config.Username, c.config.Password, protocolString, c.config.Server, c.config.Port, c.config.ServiceName, walletString)
		} else {
			oracleDriver = "oracle"
			connStr = buildGoOraURL(c)

			// Workaround for named binds, see https://github.com/jmoiron/sqlx/issues/854#issuecomment-1504070464
			sqlx.BindDriver("oracle", sqlx.NAMED)
		}
	}
	c.driver = oracleDriver

	log.Infof("%s driver: %s", c.logPrompt, oracleDriver)

	db, err := sqlx.Open(oracleDriver, connStr)
	if err != nil {
		_, err := handleRefusedConnection(c, db, err)
		return nil, fmt.Errorf("failed to connect to oracle instance: %w", err)
	}
	err = db.Ping()
	if err != nil {
		_, err := handleRefusedConnection(c, db, err)
		return nil, fmt.Errorf("failed to ping oracle instance: %w", err)
	}

	db.SetMaxOpenConns(MAX_OPEN_CONNECTIONS)

	if c.config.AgentSQLTrace.Enabled {
		db.SetMaxOpenConns(1)
		_, err := db.Exec("ALTER SESSION SET tracefile_identifier='DDAGENT'")
		if err != nil {
			log.Warnf("%s failed to set tracefile_identifier: %v", c.logPrompt, err)
		}

		/* We are concatenating values instead of passing parameters, because there seems to be a problem
		 * in go-ora with passing bool parameters to PL/SQL. As a mitigation, we are asserting that the
		 * parameters are bool
		 */
		binds := assertBool(c.config.AgentSQLTrace.Binds)
		waits := assertBool(c.config.AgentSQLTrace.Waits)
		setEventsStatement := fmt.Sprintf("BEGIN dbms_monitor.session_trace_enable (binds => %t, waits => %t); END;", binds, waits)
		log.Trace("%s trace statement: %s", c.logPrompt, setEventsStatement)
		_, err = db.Exec(setEventsStatement)
		if err != nil {
			log.Errorf("%s failed to set SQL trace: %v", c.logPrompt, err)
		}
		if c.config.AgentSQLTrace.TracedRuns == 0 {
			c.config.AgentSQLTrace.TracedRuns = DEFAULT_SQL_TRACED_RUNS
		}
	}

	return db, nil
}

func closeDatabase(c *Check, db *sqlx.DB) {
	if db != nil {
		if err := db.Close(); err != nil {
			log.Warnf("%s failed to close oracle connection: %s", c.logPrompt, err.Error())
		}
	}
}

// Building a dedicated go-ora connection for dealing with LOBs, see https://github.com/sijms/go-ora/issues/439
func connectGoOra(c *Check) (*go_ora.Connection, error) {
	conn, err := go_ora.NewConnection(buildGoOraURL(c))
	if err != nil {
		return nil, fmt.Errorf("failed to connect with the oracle driver %w", err)
	}
	err = conn.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open connection with the oracle driver %w", err)
	}
	return conn, nil
}

func closeGoOraConnection(c *Check) {
	if c.connection == nil {
		return
	}
	err := c.connection.Close()
	if err != nil {
		log.Warnf("%s failed to close go-ora connection: %s", c.logPrompt, err.Error())
	}
}
