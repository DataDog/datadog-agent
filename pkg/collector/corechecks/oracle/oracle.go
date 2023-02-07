// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oracle

import (
	"fmt"

	_ "github.com/godror/godror"
	"github.com/jmoiron/sqlx"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Check represents one Oracle instance check.
type Check struct {
	core.CheckBase
	config *config.CheckConfig
	db     *sqlx.DB
}

// Run executes the check.
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	sender.Gauge("oracle.helloworld", 10, "", nil)
	sender.Commit()
	if c.db == nil {
		db, err := c.Connect()
		if err != nil {
			c.Teardown()
			return err
		}
		c.db = db
	}
	return nil
}

// Connect establishes a connection to an Oracle instance and returns an open connection to the database.
func (c *Check) Connect() (*sqlx.DB, error) {
	connStr := fmt.Sprintf("%s/%s@%s/%s", c.config.Username, c.config.Password, c.config.Server, c.config.ServiceName)
	db, err := sqlx.Open("godror", connStr)
	if err != nil {
		log.Errorf("Failed to connect to Oracle instance | err=[%s]", err)
		return nil, err
	}
	return db, nil
}

// Teardown cleans up resources used throughout the check.
func (c *Check) Teardown() {
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			log.Warnf("Failed to close Oracle connection | server=[%s] err=[%s]", c.config.Server, err)
		}
	}
}

// Configure configures the Oracle check.
func (c *Check) Configure(integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	var err error
	c.config, err = config.NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("failed to build check config: %s", err)
	}

	// Must be called before c.CommonConfigure because this integration supports multiple instances
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	if err := c.CommonConfigure(integrationConfigDigest, rawInitConfig, rawInstance, source); err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}
	return nil
}

func oracleFactory() check.Check {
	return &Check{CheckBase: core.NewCheckBase(common.IntegrationName)}
}

func init() {
	core.RegisterCheck(common.IntegrationName, oracleFactory)
}
