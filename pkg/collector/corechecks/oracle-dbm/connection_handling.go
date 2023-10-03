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
		if c.config.InstanceConfig.InstantClient {
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

	log.Infof("driver: %s", oracleDriver)

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

	if c.cdbName == "" {
		row := db.QueryRow("SELECT /* DD */ lower(name) FROM v$database")
		err = row.Scan(&c.cdbName)
		if err != nil {
			return nil, fmt.Errorf("failed to query db name: %w", err)
		}
		c.tags = append(c.tags, fmt.Sprintf("cdb:%s", c.cdbName))
	}

	if c.dbHostname == "" || c.dbVersion == "" {
		// host_name is null on Oracle Autonomous Database
		row := db.QueryRow("SELECT /* DD */ nvl(host_name, instance_name), version_full FROM v$instance")
		var dbHostname string
		err = row.Scan(&dbHostname, &c.dbVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to query hostname and version: %w", err)
		}
		if c.config.ReportedHostname != "" {
			c.dbHostname = c.config.ReportedHostname
		} else {
			c.dbHostname = dbHostname
		}
		c.tags = append(c.tags, fmt.Sprintf("host:%s", c.dbHostname), fmt.Sprintf("oracle_version:%s", c.dbVersion))
	}

	if !c.hostingType.valid {
		ht := hostingType{value: selfManaged, valid: false}

		// Is RDS?
		if c.filePath == "" {
			r := db.QueryRow("SELECT SUBSTR(name, 1, 10) path FROM v$datafile WHERE rownum = 1")
			var path string
			err = r.Scan(&path)
			if err != nil {
				return nil, fmt.Errorf("failed to query path: %w", err)
			}
			if path == "/rdsdbdata" {
				ht.value = rds
			}
		}

		// Check if PDB
		r := db.QueryRow("select decode(sys_context('USERENV','CON_ID'),1,'CDB','PDB') TYPE from DUAL")
		var connectionType string
		err = r.Scan(&connectionType)
		if err != nil {
			return nil, fmt.Errorf("failed to query connection type: %w", err)
		}
		if connectionType == "PDB" {
			c.connectedToPdb = true
		}

		if ht.value == selfManaged {
			var cloudRows int
			if c.connectedToPdb {
				r := db.QueryRow("select 1 from v$pdbs where cloud_identity like '%oraclecloud%' and rownum = 1")
				err := r.Scan(&cloudRows)
				if err != nil {
					log.Errorf("failed to query v$pdbs: %s", err)
				}
				if cloudRows == 1 {
					r := db.QueryRow("select 1 from cdb_services where name like '%oraclecloud%' and rownum = 1")
					err := r.Scan(&cloudRows)
					if err != nil {
						log.Errorf("failed to query cdb_services: %s", err)
					}
				}
			}
			if cloudRows == 1 {
				ht.value = oci
			}
		}
		c.tags = append(c.tags, fmt.Sprintf("hosting_type:%s", ht.value))
		ht.valid = true
		c.hostingType = ht
	}

	if c.config.AgentSQLTrace.Enabled {
		db.SetMaxOpenConns(1)
		_, err := db.Exec("ALTER SESSION SET tracefile_identifier='DDAGENT'")
		if err != nil {
			log.Warnf("failed to set tracefile_identifier: %v", err)
		}

		/* We are concatenating values instead of passing parameters, because there seems to be a problem
		 * in go-ora with passing bool parameters to PL/SQL. As a mitigation, we are asserting that the
		 * parameters are bool
		 */
		binds := assertBool(c.config.AgentSQLTrace.Binds)
		waits := assertBool(c.config.AgentSQLTrace.Waits)
		setEventsStatement := fmt.Sprintf("BEGIN dbms_monitor.session_trace_enable (binds => %t, waits => %t); END;", binds, waits)
		log.Trace("trace statement: %s", setEventsStatement)
		_, err = db.Exec(setEventsStatement)
		if err != nil {
			log.Errorf("failed to set SQL trace: %v", err)
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
			log.Warnf("failed to close oracle connection | server=[%s]: %s", c.config.Server, err.Error())
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
		log.Warnf("failed to close go-ora connection | server=[%s]: %s", c.config.Server, err.Error())
	}
}
