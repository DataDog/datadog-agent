// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"
	go_ora "github.com/sijms/go-ora/v2"
)

// Check represents one Oracle instance check.
type Check struct {
	core.CheckBase
	config        *config.CheckConfig
	db            *sqlx.DB
	hostname      string
	dbmEnabled    bool
	agentVersion  string
	checkInterval float64
	tags          []string
}

// Run executes the check.
func (c *Check) Run() error {
	/*
		sender, err := c.GetSender()
		if err != nil {
			log.Error("Get sender Run")
			return err
		}
		sender.Gauge("oracle.helloworld", 10, "", nil)
		sender.Commit()
	*/
	if c.db == nil {
		db, err := c.Connect()
		if err != nil {
			c.Teardown()
			return err
		}
		c.db = db
	}

	if c.hostname == "" {
		if c.config.InstanceConfig.ReportedHostname != "" {
			c.hostname = c.config.InstanceConfig.ReportedHostname
		} else {
			hostname, err := hostname.Get(context.TODO())
			if err != nil {
				return fmt.Errorf("failed to get hostname: %w", err)
			}
			c.hostname = hostname
		}
	}

	if c.dbmEnabled {
		err := c.SampleSession()
		if err != nil {
			return log.Errorf("Sampling session: %v", err)
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
		if c.config.InstanceConfig.UseGodrorWithEZConnect {
			oracleDriver = "godror"
			connStr = fmt.Sprintf("%s/%s@%s/%s", c.config.Username, c.config.Password, c.config.Server, c.config.ServiceName)
		} else {
			oracleDriver = "oracle"
			connStr = go_ora.BuildUrl(c.config.Server, c.config.Port, c.config.ServiceName, c.config.Username, c.config.Password, map[string]string{})
		}
	}

	log.Infof("Connect string: %s", connStr)

	db, err := sqlx.Open(oracleDriver, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to oracle instance: %w", err)
	}
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("failed to ping oracle instance: %w", err)
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

	return nil
}

func oracleFactory() check.Check {
	return &Check{CheckBase: core.NewCheckBase(common.IntegrationName)}
}

func init() {
	core.RegisterCheck(common.IntegrationName, oracleFactory)
}
