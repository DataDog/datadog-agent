package snmp

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/report"
)

const (
	snmpCheckName = "snmp"
)

var timeNow = time.Now

// Check aggregates metrics from one Check instance
type Check struct {
	core.CheckBase
	config         *checkconfig.CheckConfig
	singleDeviceCk *devicecheck.DeviceCheck
}

// Run executes the check
func (c *Check) Run() error {

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	c.singleDeviceCk.SetSender(report.NewMetricSender(sender))

	collectionTime := timeNow()

	err = c.singleDeviceCk.Run(collectionTime)
	if err != nil {
		return err
	}

	// Commit
	sender.Commit()
	return nil
}

// Configure configures the snmp checks
func (c *Check) Configure(rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	// Must be called before c.CommonConfigure
	c.BuildID(rawInstance, rawInitConfig)

	err := c.CommonConfigure(rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	c.config, err = checkconfig.NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("build config failed: %s", err)
	}
	log.Debugf("SNMP configuration: %s", c.config.ToString())

	c.singleDeviceCk, err = devicecheck.NewDeviceCheck(c.config, c.config.IPAddress)
	if err != nil {
		return fmt.Errorf("failed to create device check: %s", err)
	}

	return nil
}

// Interval returns the scheduling time for the check
func (c *Check) Interval() time.Duration {
	return c.config.MinCollectionInterval
}

func snmpFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(snmpCheckName),
	}
}

func init() {
	core.RegisterCheck(snmpCheckName, snmpFactory)
}
