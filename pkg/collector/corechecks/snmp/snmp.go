package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/report"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/devicecheck"
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
	discovery      snmpDiscovery
}

// Run executes the check
func (c *Check) Run() error {
	var checkErr error
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	if c.config.Network != "" {
		var discoveredDevices []*devicecheck.DeviceCheck
		if c.config.TestInstances == 0 {
			discoveredDevices = c.discovery.getDiscoveredDeviceConfigs(sender)
		} else {
			discoveredDevices = c.discovery.getDiscoveredDeviceConfigsTestInstances(c.config.TestInstances, sender)
		}

		jobs := make(chan *devicecheck.DeviceCheck, len(discoveredDevices))

		var wg sync.WaitGroup

		for w := 1; w <= c.config.Workers; w++ {
			wg.Add(1)
			go c.runCheckDeviceWorker(w, &wg, jobs)
		}

		for i := range discoveredDevices {
			devivceCk := discoveredDevices[i]
			devivceCk.SetSender(report.NewMetricSender(sender))
			log.Warnf("[DEV] schedule device collection: %s, tags: %v", devivceCk.GetIPAddress(), devivceCk.GetIDTags())
			//checkErr = c.runCheckDevice(devivceCk)
			jobs <- devivceCk
		}
		close(jobs)
		wg.Wait() // wait for all workers to finish

	} else {
		// TODO: sender submittedMetrics state, so need to be per config/device level
		c.singleDeviceCk.SetSender(report.NewMetricSender(sender))
		checkErr = c.runCheckDevice(c.singleDeviceCk)
	}

	// Commit
	sender.Commit()
	return checkErr

	return nil
}

func (c *Check) runCheckDeviceWorker(workerID int, wg *sync.WaitGroup, jobs <-chan *devicecheck.DeviceCheck) {
	defer wg.Done()
	for job := range jobs {
		log.Warnf("[DEV] worker %d starting collect device %s, tags %s", workerID, job.GetIPAddress(), job.GetIDTags())
		err := c.runCheckDevice(job)
		if err != nil {
			log.Warnf("[DEV] worker %d error collect device %s: %s", workerID, job.GetIPAddress(), err)
			continue
		}
		log.Warnf("[DEV] worker %d done collect device %s ", workerID, job.GetIPAddress())
	}
}

func (c *Check) runCheckDevice(deviceCk *devicecheck.DeviceCheck) error {
	collectionTime := timeNow()
	time.Sleep(1 * time.Second) // TODO: Remove me, for testing

	err := deviceCk.Run(collectionTime)
	if err != nil {
		return err
	}

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

	if c.config.Network != "" {
		log.Warnf("[DEV] Network: %s", c.config.Network)
		c.discovery = newSnmpDiscovery(c.config)
		c.discovery.Start()
	}
	return nil
}

// Cancel is called when check is unscheduled
func (c *Check) Cancel() {
	log.Warnf("[DEV] Cancel called for check %s", c.ID())
	c.discovery.stop <- true
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
