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

const (
	// MaxSQLFullTextVSQL is SQL_FULLTEXT size in V$SQL
	MaxSQLFullTextVSQL = 4000

	// MaxSQLFullTextVSQLStats is SQL_FULLTEXT size in V$SQLSTATS. The column is defined as VARCHAR2(4000)
	// but due to the Oracle bug "27760729 : V$SQLSTAT.SQL_FULLTEXT DOES NOT SHOW COMPLETE SQL STMT";
	// it contains only the first 1000 characters
	MaxSQLFullTextVSQLStats = 1000
)

type hostingCode string

const (
	selfManaged hostingCode = "self-managed"
	rds         hostingCode = "RDS"
	oci         hostingCode = "OCI"
)

type hostingType struct {
	value hostingCode
	valid bool
}

type pgaOverAllocationCount struct {
	value float64
	valid bool
}

type Check struct {
	core.CheckBase
	config                                  *config.CheckConfig
	db                                      *sqlx.DB
	dbCustomQueries                         *sqlx.DB
	connection                              *go_ora.Connection
	dbmEnabled                              bool
	agentVersion                            string
	checkInterval                           float64
	tags                                    []string
	tagsString                              string
	cdbName                                 string
	statementMetricsMonotonicCountsPrevious map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB
	dbHostname                              string
	dbVersion                               string
	driver                                  string
	metricLastRun                           time.Time
	statementsLastRun                       time.Time
	filePath                                string
	sqlTraceRunsCount                       int
	connectedToPdb                          bool
	fqtEmitted                              *cache.Cache
	planEmitted                             *cache.Cache
	previousPGAOverAllocationCount          pgaOverAllocationCount
	hostingType
	logPrompt string
}

func handleServiceCheck(c *Check, err error) {
	sender, errSender := c.GetSender()
	if errSender != nil {
		log.Errorf("%s failed to get sender for service check %s", c.logPrompt, err)
	}

	message := ""
	var status servicecheck.ServiceCheckStatus
	if err == nil {
		status = servicecheck.ServiceCheckOK
	} else {
		status = servicecheck.ServiceCheckCritical
		log.Errorf("%s failed to connect: %s", c.logPrompt, err)
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
			return fmt.Errorf("%s empty connection", c.logPrompt)
		}
		c.db = db
	}

	if c.driver == "oracle" && c.connection == nil {
		conn, err := connectGoOra(c)
		if err != nil {
			return fmt.Errorf("%s failed to connect with go-ora %w", c.logPrompt, err)
		}
		c.connection = conn
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
			closeDatabase(c, db)
			return fmt.Errorf("%s failed to collect os stats %w", c.logPrompt, err)
		} else {
			handleServiceCheck(c, nil)
		}

		if c.config.SysMetrics.Enabled {
			log.Debugf("%s Entered sysmetrics", c.logPrompt)
			err := c.SysMetrics()
			if err != nil {
				return fmt.Errorf("%s failed to collect sysmetrics %w", c.logPrompt, err)
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
		if len(c.config.CustomQueries) > 0 {
			err := c.CustomQueries()
			if err != nil {
				log.Errorf("%s failed to execute custom queries %s", c.logPrompt, err)
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
		}
	}

	if c.config.AgentSQLTrace.Enabled {
		log.Debugf("%s Traced runs %d", c.logPrompt, c.sqlTraceRunsCount)
		c.sqlTraceRunsCount++
		if c.sqlTraceRunsCount >= c.config.AgentSQLTrace.TracedRuns {
			c.config.AgentSQLTrace.Enabled = false
			_, err := c.db.Exec("BEGIN dbms_monitor.session_trace_disable; END;")
			if err != nil {
				log.Errorf("%s failed to stop SQL trace: %v", c.logPrompt, err)
			}
			c.db.SetMaxOpenConns(MAX_OPEN_CONNECTIONS)
		}
	}
	return nil
}

func assertBool(val bool) bool {
	return val
}

// Teardown cleans up resources used throughout the check.
func (c *Check) Teardown() {
	log.Infof("%s Teardown", c.logPrompt)
	closeDatabase(c, c.db)
	closeDatabase(c, c.dbCustomQueries)
	closeGoOraConnection(c)
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
	c.tags = make([]string, len(c.config.Tags))
	copy(c.tags, c.config.Tags)
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

	c.fqtEmitted = getFqtEmittedCache()
	c.planEmitted = getPlanEmittedCache(c)

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
			log.Errorf("%s Obfuscation error for SQL: %s", c.logPrompt, statement)
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
	return append(tags, "pdb:"+strings.ToLower(pdb.String))
}
