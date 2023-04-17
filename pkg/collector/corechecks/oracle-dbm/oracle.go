// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle-dbm/config"
	"github.com/DataDog/datadog-agent/pkg/obfuscate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	go_ora "github.com/sijms/go-ora/v2"
)

// Check represents one Oracle instance check.
type StatementsFilter struct {
	SQLIDs map[string]int
	//ForceMatchingSignatures map[uint64]int
	ForceMatchingSignatures map[string]int
}

type Check struct {
	core.CheckBase
	config                                  *config.CheckConfig
	db                                      *sqlx.DB
	dbmEnabled                              bool
	agentVersion                            string
	checkInterval                           float64
	tags                                    []string
	cdbName                                 string
	statementsFilter                        StatementsFilter
	statementMetricsMonotonicCountsPrevious map[StatementMetricsKeyDB]StatementMetricsMonotonicCountDB
	dbHostname                              string
	dbVersion                               string
	driver                                  string
}

// Run executes the check.
func (c *Check) Run() error {
	if c.db == nil {
		db, err := c.Connect()
		if err != nil {
			c.Teardown()
			return err
		}
		c.db = db
	}

	if c.dbmEnabled {
		err := c.SampleSession()
		if err != nil {
			return err
		}

		_, err = c.StatementMetrics()
		if err != nil {
			return err
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
			connStr = fmt.Sprintf("%s/%s@%s/%s", c.config.Username, c.config.Password, c.config.Server, c.config.ServiceName)
		} else {
			oracleDriver = "oracle"
			connStr = go_ora.BuildUrl(c.config.Server, c.config.Port, c.config.ServiceName, c.config.Username, c.config.Password, map[string]string{})
			// https://github.com/jmoiron/sqlx/issues/854#issuecomment-1504070464
			sqlx.BindDriver("oracle", sqlx.NAMED)
		}
	}
	c.driver = oracleDriver

	log.Infof("driver: %s, Connect string: %s", oracleDriver, connStr)

	db, err := sqlx.Open(oracleDriver, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to oracle instance: %w", err)
	}
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping oracle instance: %w", err)
	}

	db.SetMaxOpenConns(10)

	if c.cdbName == "" {
		row := db.QueryRow("SELECT name FROM v$database")
		err = row.Scan(&c.cdbName)
		if err != nil {
			return nil, fmt.Errorf("failed to query db name: %w", err)
		}
		c.tags = append(c.tags, fmt.Sprintf("cdb:%s", c.cdbName))
	}

	if c.dbHostname == "" || c.dbVersion == "" {
		row := db.QueryRow("SELECT host_name, version FROM v$instance")
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

	return db, nil
}

// Teardown cleans up resources used throughout the check.
func (c *Check) Teardown() {
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			log.Warnf("failed to close oracle connection | server=[%s]: %s", c.config.Server, err.Error())
		}
	}
}

// Configure configures the Oracle check.
func (c *Check) Configure(integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	var err error
	c.config, err = config.NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("failed to build check config: %w", err)
	}

	// Must be called before c.CommonConfigure because this integration supports multiple instances
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	if err := c.CommonConfigure(integrationConfigDigest, rawInitConfig, rawInstance, source); err != nil {
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

	return nil
}

func oracleFactory() check.Check {
	return &Check{CheckBase: core.NewCheckBase(common.IntegrationNameScheduler)}
}

func init() {
	core.RegisterCheck(common.IntegrationNameScheduler, oracleFactory)
}

// func (c *Check) GetObfuscatedStatement(o *obfuscate.Obfuscator, statement string, forceMatchingSignature string, SQLID string) (common.ObfuscatedStatement, error) {
func (c *Check) GetObfuscatedStatement(o *obfuscate.Obfuscator, statement string) (common.ObfuscatedStatement, error) {
	obfuscatedStatement, err := o.ObfuscateSQLString(statement)
	if err == nil {
		return common.ObfuscatedStatement{
			Statement:      obfuscatedStatement.Query,
			QuerySignature: common.GetQuerySignature(statement),
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
