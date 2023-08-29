// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	cache "github.com/patrickmn/go-cache"
	go_ora "github.com/sijms/go-ora/v2"
)

var MAX_OPEN_CONNECTIONS = 10
var DEFAULT_SQL_TRACED_RUNS = 10
var DB_TIMEOUT = "20000"

// The structure is filled by activity sampling and serves as a filter for query metrics
type StatementsFilter struct {
	SQLIDs                  map[string]int
	ForceMatchingSignatures map[string]int
}

type StatementsCacheData struct {
	statement      string
	querySignature string
	tables         []string
	commands       []string
}
type StatementsCache struct {
	SQLIDs                  map[string]StatementsCacheData
	forceMatchingSignatures map[string]StatementsCacheData
}

type Check struct {
	core.CheckBase
	config                                  *config.CheckConfig
	db                                      *sqlx.DB
	dbCustomQueries                         *sqlx.DB
	dbmEnabled                              bool
	agentVersion                            string
	checkInterval                           float64
	tags                                    []string
	tagsString                              string
	cdbName                                 string
	statementsFilter                        StatementsFilter
	statementsCache                         StatementsCache
	DDstatementsCache                       StatementsCache
	DDPrevStatementsCache                   StatementsCache
	statementMetricsMonotonicCountsPrevious map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB
	dbHostname                              string
	dbVersion                               string
	driver                                  string
	metricLastRun                           time.Time
	statementsLastRun                       time.Time
	filePath                                string
	isRDS                                   bool
	isOracleCloud                           bool
	sqlTraceRunsCount                       int
	connectedToPdb                          bool
	fqtEmitted                              *cache.Cache
	planEmitted                             *cache.Cache
	previousAllocationCount                 float64
}

func handleServiceCheck(c *Check, err error) {
	sender, errSender := c.GetSender()
	if errSender != nil {
		log.Errorf("failed to get sender for service check %s", err)
	}

	message := ""
	var status servicecheck.ServiceCheckStatus
	if err == nil {
		status = servicecheck.ServiceCheckOK
	} else {
		status = servicecheck.ServiceCheckCritical
		log.Errorf("failed to connect: %s", err)
	}
	sender.ServiceCheck("oracle.can_connect", status, "", c.tags, message)
	sender.Commit()
}

func checkIntervalExpired(lastRun *time.Time, collectionInterval int64) bool {
	start := time.Now()
	if lastRun.IsZero() || start.Sub(*lastRun).Milliseconds() >= collectionInterval*1000 {
		*lastRun = start
		return true
	}
	return false
}

// Run executes the check.
func (c *Check) Run() error {
	if c.db == nil {
		db, err := c.Connect()
		if err != nil {
			handleServiceCheck(c, err)
			c.Teardown()
			return err
		}
		if db == nil {
			c.Teardown()
			handleServiceCheck(c, fmt.Errorf("empty connection"))
			return fmt.Errorf("empty connection")
		}
		c.db = db
	}

	metricIntervalExpired := checkIntervalExpired(&c.metricLastRun, c.config.MetricCollectionInterval)

	if metricIntervalExpired {
		err := c.OS_Stats()
		if err != nil {
			db, errConnect := c.Connect()
			if errConnect != nil {
				handleServiceCheck(c, errConnect)
			} else if db == nil {
				handleServiceCheck(c, fmt.Errorf("empty connection"))
			} else {
				handleServiceCheck(c, nil)
			}
			if errClosing := CloseDatabaseConnection(db); err != nil {
				log.Errorf("Error closing connection %s", errClosing)
			}
			return fmt.Errorf("failed to collect os stats %w", err)
		} else {
			handleServiceCheck(c, nil)
		}

		if c.config.SysMetrics.Enabled {
			log.Trace("Entered sysmetrics")
			err := c.SysMetrics()
			if err != nil {
				return fmt.Errorf("failed to collect sysmetrics %w", err)
			}
		}
		if c.config.Tablespaces.Enabled {
			err := c.Tablespaces()
			if err != nil {
				return err
			}
		}
		if c.config.ProcessMemory.Enabled {
			err := c.ProcessMemory()
			if err != nil {
				return err
			}
		}
	}

	if c.dbmEnabled {
		if c.config.QuerySamples.Enabled {
			err := c.SampleSession()
			if err != nil {
				return err
			}
			if c.config.QueryMetrics.Enabled {
				_, err = c.StatementMetrics()
				if err != nil {
					return err
				}
			}
		}
		if metricIntervalExpired {
			if c.config.SharedMemory.Enabled {
				err := c.SharedMemory()
				if err != nil {
					return err
				}
			}
			if len(c.config.CustomQueries) > 0 {
				err := c.CustomQueries()
				if err != nil {
					log.Errorf("failed to execute custom queries %s", err)
				}
			}
		}
	}

	if c.config.AgentSQLTrace.Enabled {
		log.Tracef("Traced runs %d", c.sqlTraceRunsCount)
		c.sqlTraceRunsCount++
		if c.sqlTraceRunsCount >= c.config.AgentSQLTrace.TracedRuns {
			c.config.AgentSQLTrace.Enabled = false
			_, err := c.db.Exec("BEGIN dbms_monitor.session_trace_disable; END;")
			if err != nil {
				log.Errorf("failed to stop SQL trace: %v", err)
			}
			c.db.SetMaxOpenConns(MAX_OPEN_CONNECTIONS)
		}
	}
	return nil
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
			connectionOptions := map[string]string{"TIMEOUT": DB_TIMEOUT}
			if c.config.Protocol == "TCPS" {
				connectionOptions["SSL"] = "TRUE"
				if c.config.Wallet != "" {
					connectionOptions["WALLET"] = c.config.Wallet
				}
			}
			connStr = go_ora.BuildUrl(c.config.Server, c.config.Port, c.config.ServiceName, c.config.Username, c.config.Password, connectionOptions)
			// https://github.com/jmoiron/sqlx/issues/854#issuecomment-1504070464
			sqlx.BindDriver("oracle", sqlx.NAMED)
		}
	}
	c.driver = oracleDriver

	log.Infof("driver: %s", oracleDriver)

	db, err := sqlx.Open(oracleDriver, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to oracle instance: %w", err)
	}
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping oracle instance: %w", err)
	}

	db.SetMaxOpenConns(MAX_OPEN_CONNECTIONS)

	if c.cdbName == "" {
		row := db.QueryRow("SELECT /* DD */ name FROM v$database")
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

	if c.filePath == "" {
		r := db.QueryRow("SELECT SUBSTR(name, 1, 10) path FROM v$datafile WHERE rownum = 1")
		var path string
		err = r.Scan(&path)
		if err != nil {
			return nil, fmt.Errorf("failed to query path: %w", err)
		}
		if path == "/rdsdbdata" {
			c.isRDS = true
		}
	}

	r := db.QueryRow("select decode(sys_context('USERENV','CON_ID'),1,'CDB','PDB') TYPE from DUAL")
	var connectionType string
	err = r.Scan(&connectionType)
	if err != nil {
		return nil, fmt.Errorf("failed to query connection type: %w", err)
	}
	var cloudRows int
	if connectionType == "PDB" {
		c.connectedToPdb = true
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
		c.isOracleCloud = true
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

func assertBool(val bool) bool {
	return val
}

// Teardown cleans up resources used throughout the check.
func (c *Check) Teardown() {
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			log.Warnf("failed to close oracle connection | server=[%s]: %s", c.config.Server, err.Error())
		}
	}
	c.fqtEmitted = nil
	c.planEmitted = nil
}

func CloseDatabaseConnection(db *sqlx.DB) error {
	if db != nil {
		if err := db.Close(); err != nil {
			return fmt.Errorf("failed to close oracle connection: %s", err)
		}
	}
	return nil
}

// Configure configures the Oracle check.
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	var err error
	c.config, err = config.NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("failed to build check config: %w", err)
	}

	// Must be called before c.CommonConfigure because this integration supports multiple instances
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	if err := c.CommonConfigure(senderManager, integrationConfigDigest, rawInitConfig, rawInstance, source); err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	c.dbmEnabled = false
	if c.config.DBM {
		c.dbmEnabled = true
	}

	agentVersion, _ := version.Agent()
	c.agentVersion = agentVersion.GetNumberAndPre()

	c.checkInterval = float64(c.config.InitConfig.MinCollectionInterval)
	c.tags = c.config.Tags
	c.tags = append(c.tags, fmt.Sprintf("dbms:%s", common.IntegrationName), fmt.Sprintf("ddagentversion:%s", c.agentVersion))
	c.tags = append(c.tags, fmt.Sprintf("dbm:%t", c.dbmEnabled))
	if c.config.TnsAlias != "" {
		c.tags = append(c.tags, fmt.Sprintf("tns-alias:%s", c.config.TnsAlias))
	}
	if c.config.Port != 0 {
		c.tags = append(c.tags, fmt.Sprintf("port:%d", c.config.Port))
	}
	if c.config.Server != "" {
		c.tags = append(c.tags, fmt.Sprintf("server:%s", c.config.Server))
	}
	if c.config.ServiceName != "" {
		c.tags = append(c.tags, fmt.Sprintf("service:%s", c.config.ServiceName))
	}

	c.tagsString = strings.Join(c.tags, ",")

	c.fqtEmitted = cache.New(60*time.Minute, 10*time.Minute)

	var planCacheRetention = c.config.QueryMetrics.PlanCacheRetention
	if planCacheRetention == 0 {
		planCacheRetention = 1
	}
	c.planEmitted = cache.New(time.Duration(planCacheRetention)*time.Minute, 10*time.Minute)

	return nil
}

func oracleFactory() check.Check {
	return &Check{CheckBase: core.NewCheckBaseWithInterval(common.IntegrationNameScheduler, 10*time.Second)}
}

func init() {
	core.RegisterCheck(common.IntegrationNameScheduler, oracleFactory)
}

func (c *Check) GetObfuscatedStatement(o *obfuscate.Obfuscator, statement string) (common.ObfuscatedStatement, error) {
	obfuscatedStatement, err := o.ObfuscateSQLString(statement)
	if err == nil {
		return common.ObfuscatedStatement{
			Statement:      obfuscatedStatement.Query,
			QuerySignature: common.GetQuerySignature(obfuscatedStatement.Query),
			Commands:       obfuscatedStatement.Metadata.Commands,
			Tables:         strings.Split(obfuscatedStatement.Metadata.TablesCSV, ","),
			Comments:       obfuscatedStatement.Metadata.Comments,
		}, nil
	} else {
		if c.config.InstanceConfig.LogUnobfuscatedQueries {
			log.Error(fmt.Sprintf("Obfuscation error for SQL: %s", statement))
		}
		return common.ObfuscatedStatement{Statement: statement}, err
	}
}

func (c *Check) getFullPDBName(pdb string) string {
	return fmt.Sprintf("%s.%s", c.cdbName, pdb)
}

func appendPDBTag(tags []string, pdb sql.NullString) []string {
	if !pdb.Valid {
		return tags
	}
	return append(tags, "pdb:"+pdb.String)
}

func selectWrapper[T any](c *Check, s T, sql string, binds ...interface{}) error {
	err := c.db.Select(s, sql, binds...)
	if err != nil && (strings.Contains(err.Error(), "ORA-01012") || strings.Contains(err.Error(), "ORA-06413") || strings.Contains(err.Error(), "database is closed")) {
		db, err := c.Connect()
		if err != nil {
			c.Teardown()
			return err
		}
		c.db = db
	}

	return err
}
