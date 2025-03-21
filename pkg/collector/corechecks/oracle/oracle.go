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
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"

	//nolint:revive // TODO(DBM) Fix revive linter
	_ "github.com/godror/godror"
	go_version "github.com/hashicorp/go-version"
	"github.com/jmoiron/sqlx"
	cache "github.com/patrickmn/go-cache"
	go_ora "github.com/sijms/go-ora/v2"
)

//nolint:revive // TODO(DBM) Fix revive linter
var MAX_OPEN_CONNECTIONS = 10

//nolint:revive // TODO(DBM) Fix revive linter
var DEFAULT_SQL_TRACED_RUNS = 10

//nolint:revive // TODO(DBM) Fix revive linter
var DB_TIMEOUT = "20000"

const (
	// MaxSQLFullTextVSQL is SQL_FULLTEXT size in V$SQL
	MaxSQLFullTextVSQL = 4000

	// MaxSQLFullTextVSQLStats is SQL_FULLTEXT size in V$SQLSTATS. The column is defined as VARCHAR2(4000)
	// but due to the Oracle bug "27760729 : V$SQLSTAT.SQL_FULLTEXT DOES NOT SHOW COMPLETE SQL STMT";
	// it contains only the first 1000 characters
	MaxSQLFullTextVSQLStats = 1000
)

const serviceCheckName = "oracle.can_query"

type hostingCode string

const (
	// CheckName is the name of the check
	CheckName = common.IntegrationNameScheduler
	// OracleDbmCheckName is the name of the check that was renamed to `oracle`.
	OracleDbmCheckName             = "oracle-dbm"
	selfManaged        hostingCode = "self-managed"
	rds                hostingCode = "RDS"
	oci                hostingCode = "OCI"
)

type pgaOverAllocationCount struct {
	value float64
	valid bool
}

//nolint:revive // TODO(DBM) Fix revive linter
type Check struct {
	core.CheckBase
	config                                  *config.CheckConfig
	db                                      *sqlx.DB
	dbCustomQueries                         *sqlx.DB
	connection                              *go_ora.Connection
	dbmEnabled                              bool
	agentVersion                            string
	agentHostname                           string
	checkInterval                           float64
	tags                                    []string
	tagsWithoutDbRole                       []string
	configTags                              []string
	tagsString                              string
	cdbName                                 string
	statementMetricsMonotonicCountsPrevious map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB
	dbHostname                              string
	dbVersion                               string
	driver                                  string
	metricLastRun                           time.Time
	statementsLastRun                       time.Time
	dbInstanceLastRun                       time.Time
	filePath                                string
	sqlTraceRunsCount                       int
	connectedToPdb                          bool
	fqtEmitted                              *cache.Cache
	planEmitted                             *cache.Cache
	previousPGAOverAllocationCount          pgaOverAllocationCount
	hostingType                             hostingCode
	logPrompt                               string
	initialized                             bool
	multitenant                             bool
	lastOracleRows                          []OracleRow // added for tests
	databaseRole                            string
	openMode                                string
	legacyIntegrationCompatibilityMode      bool
}

type vDatabase struct {
	Name         string `db:"NAME"`
	Cdb          string `db:"CDB"`
	DatabaseRole string `db:"DATABASE_ROLE"`
	OpenMode     string `db:"OPEN_MODE"`
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
	var allErrors error
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

	metricIntervalExpired := checkIntervalExpired(&c.metricLastRun, c.config.MetricCollectionInterval)

	if !c.initialized {
		err := c.init()
		if err != nil {
			return fmt.Errorf("%s failed to initialize: %w", c.logPrompt, err)
		}
	}

	if c.driver == "oracle" && c.connection == nil {
		conn, err := connectGoOra(c)
		if err != nil {
			return fmt.Errorf("%s failed to connect with go-ora %w", c.logPrompt, err)
		}
		c.connection = conn
	}

	dbInstanceIntervalExpired := checkIntervalExpired(&c.dbInstanceLastRun, 1800)

	if dbInstanceIntervalExpired && !c.legacyIntegrationCompatibilityMode && !c.config.OnlyCustomQueries {
		err := sendDbInstanceMetadata(c)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to send db instance metadata %w", c.logPrompt, err))
		}
	}

	if metricIntervalExpired {
		if c.dbmEnabled {
			err := c.dataGuard()
			if err != nil {
				allErrors = errors.Join(allErrors, err)
			}
		}
		fixTags(c)

		err := c.connectionTest()
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
			allErrors = errors.Join(allErrors, fmt.Errorf("%s connection error %w", c.logPrompt, err))
		} else {
			handleServiceCheck(c, nil)
		}

		if c.config.OnlyCustomQueries {
			if metricIntervalExpired && (len(c.config.InstanceConfig.CustomQueries) > 0 || len(c.config.InitConfig.CustomQueries) > 0) {
				err = c.CustomQueries()
				var message string
				var status servicecheck.ServiceCheckStatus
				if allErrors == nil {
					status = servicecheck.ServiceCheckOK
				} else {
					status = servicecheck.ServiceCheckCritical
					message = allErrors.Error()
				}
				sendServiceCheck(c, serviceCheckName, status, message)
				commit(c)
				return err
			}
		}

		if !c.legacyIntegrationCompatibilityMode {
			err := c.OS_Stats()
			if err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect os stats %w", c.logPrompt, err))
			}
		}

		if c.config.SysMetrics.Enabled {
			log.Debugf("%s Entered sysmetrics", c.logPrompt)
			_, err := c.sysMetrics()
			if err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect sysmetrics %w", c.logPrompt, err))
			}
		}
		if c.config.Tablespaces.Enabled {
			err := c.Tablespaces()
			if err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect tablespaces %w", c.logPrompt, err))
			}
		}
		if c.config.ProcessMemory.Enabled || c.config.InactiveSessions.Enabled {
			err := c.ProcessMemory()
			if err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect process memory %w", c.logPrompt, err))
			}
		}
		if metricIntervalExpired && (len(c.config.InstanceConfig.CustomQueries) > 0 || len(c.config.InitConfig.CustomQueries) > 0) {
			err := c.CustomQueries()
			if err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to execute custom queries %w", c.logPrompt, err))
			}
		}
	}

	if c.dbmEnabled {
		if c.config.QuerySamples.Enabled {
			err := c.SampleSession()
			if err != nil {
				allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect session samples %w", c.logPrompt, err))
			}
			if c.config.QueryMetrics.Enabled {
				_, err = c.StatementMetrics()
				if err != nil {
					allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect statement metrics %w", c.logPrompt, err))
				}
			}
		}
		if metricIntervalExpired {
			if c.config.SharedMemory.Enabled {
				err := c.SharedMemory()
				if err != nil {
					allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect shared memory %w", c.logPrompt, err))
				}
			}
		}

		if metricIntervalExpired {
			if c.config.Asm.Enabled {
				err := c.asmDiskgroups()
				if err != nil {
					allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect asm diskgroups %w", c.logPrompt, err))
				}
			}
		}

		if metricIntervalExpired {
			if c.config.ResourceManager.Enabled {
				err := c.resourceManager()
				if err != nil {
					allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect resource manager %w", c.logPrompt, err))
				}
			}
			if c.config.Locks.Enabled {
				err := c.locks()
				if err != nil {
					allErrors = errors.Join(allErrors, fmt.Errorf("%s failed to collect locks %w", c.logPrompt, err))
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

	var message string
	var status servicecheck.ServiceCheckStatus
	if allErrors == nil {
		status = servicecheck.ServiceCheckOK
	} else {
		status = servicecheck.ServiceCheckCritical
		message = allErrors.Error()
	}
	sendServiceCheck(c, serviceCheckName, status, message)
	commit(c)
	if c.legacyIntegrationCompatibilityMode {
		log.Warnf("%s missing privileges detected, running in deprecated integration compatibility mode", c.logPrompt)
	}
	return allErrors
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

	tags := make([]string, len(c.config.Tags))
	copy(tags, c.config.Tags)

	tags = append(tags, fmt.Sprintf("dbms:%s", common.IntegrationName), fmt.Sprintf("ddagentversion:%s", c.agentVersion))
	tags = append(tags, fmt.Sprintf("dbm:%t", c.dbmEnabled))
	if c.config.TnsAlias != "" {
		tags = append(tags, fmt.Sprintf("tns-alias:%s", c.config.TnsAlias))
	}
	if c.config.Port != 0 {
		tags = append(tags, fmt.Sprintf("port:%d", c.config.Port))
	}
	if c.config.Server != "" {
		tags = append(tags, fmt.Sprintf("server:%s", c.config.Server))
	}
	if c.config.ServiceName != "" {
		tags = append(tags, fmt.Sprintf("service:%s", c.config.ServiceName))
	}

	c.logPrompt = config.GetLogPrompt(c.config.InstanceConfig)

	agentHostname, err := hostname.Get(context.Background())
	if err == nil {
		c.agentHostname = agentHostname
	} else {
		log.Errorf("%s failed to retrieve agent hostname: %s", c.logPrompt, err)
	}
	tags = append(tags, fmt.Sprintf("ddagenthostname:%s", c.agentHostname))

	c.configTags = make([]string, len(tags))
	copy(c.configTags, tags)
	c.tags = make([]string, len(tags))
	copy(c.tags, tags)

	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{CheckBase: core.NewCheckBaseWithInterval(common.IntegrationNameScheduler, 10*time.Second)}
}

//nolint:revive // TODO(DBM) Fix revive linter
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

func isDbVersionLessThan(c *Check, v string) bool {
	dbVersion := c.dbVersion
	vParsed, err := go_version.NewVersion(v)
	if err != nil {
		log.Errorf("%s Can't parse %s version string", c.logPrompt, v)
		return false
	}
	parsedDbVersion, err := go_version.NewVersion(dbVersion)
	if err != nil {
		log.Errorf("%s Can't parse db version string %s", c.logPrompt, dbVersion)
		return false
	}
	if parsedDbVersion.LessThan(vParsed) {
		return true
	}
	return false
}

func isDbVersionGreaterOrEqualThan(c *Check, v string) bool {
	return !isDbVersionLessThan(c, v)
}

func fixTags(c *Check) {
	c.tags = make([]string, len(c.tagsWithoutDbRole))
	copy(c.tags, c.tagsWithoutDbRole)
	if c.databaseRole != "" {
		roleTag := strings.ToLower(strings.ReplaceAll(string(c.databaseRole), " ", "_"))
		c.tags = append(c.tags, "database_role:"+roleTag)
	}
	c.tagsString = strings.Join(c.tags, ",")
}
