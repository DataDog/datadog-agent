package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/devicecheck"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
)

const (
	snmpCheckName = "snmp"
)

var timeNow = time.Now

// Check aggregates metrics from one Check instance
type Check struct {
	core.CheckBase
	config  checkconfig.CheckConfig // TODO: use ref instead of struct ?
	session session.Session
	//sender   *report.MetricSender
	deviceCk *devicecheck.DeviceCheck
}

// Run executes the check
func (c *Check) Run() error {

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	c.deviceCk.SetSender(report.NewMetricSender(sender))
	c.deviceCk.SetSession(c.session)

	collectionTime := timeNow()

	err = c.deviceCk.Run(collectionTime)
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

	config, err := checkconfig.BuildConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("build config failed: %s", err)
	}

	log.Debugf("SNMP configuration: %s", config.ToString())

	c.config = config
	err = c.session.Configure(c.config)
	if err != nil {
		return fmt.Errorf("session configure failed: %s", err)
	}

	c.deviceCk = devicecheck.NewDeviceCheck(&c.config, c.config.IPAddress)

	return nil
}

// Interval returns the scheduling time for the check
func (c *Check) Interval() time.Duration {
	return c.config.MinCollectionInterval
}

func snmpFactory() check.Check {
	return &Check{
		session:   &session.GosnmpSession{},
		CheckBase: core.NewCheckBase(snmpCheckName),
	}
}

func init() {
	core.RegisterCheck(snmpCheckName, snmpFactory)
}
