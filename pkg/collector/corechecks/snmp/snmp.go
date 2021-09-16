package snmp

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/discovery"
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
	discovery      discovery.Discovery
}

// Run executes the check
func (c *Check) Run() error {
	var checkErr error
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	if c.config.IsDiscovery() {
		var discoveredDevices []*devicecheck.DeviceCheck
		discoveredDevices = c.discovery.GetDiscoveredDeviceConfigs()

		jobs := make(chan *devicecheck.DeviceCheck, len(discoveredDevices))

		var wg sync.WaitGroup

		for w := 1; w <= c.config.Workers; w++ {
			wg.Add(1)
			go c.runCheckDeviceWorker(w, &wg, jobs)
		}

		for i := range discoveredDevices {
			deviceCk := discoveredDevices[i]
			deviceCk.SetSender(report.NewMetricSender(sender))
			jobs <- deviceCk
		}
		close(jobs)
		wg.Wait() // wait for all workers to finish

		tags := append(c.config.GetStaticTags(), "network:"+c.config.Network)
		tags = append(tags, c.config.GetNetworkTags()...)
		sender.Gauge("snmp.discovered_devices_count", float64(len(discoveredDevices)), "", tags)
	} else {
		c.singleDeviceCk.SetSender(report.NewMetricSender(sender))
		checkErr = c.runCheckDevice(c.singleDeviceCk)
	}

	// Commit
	sender.Commit()
	return checkErr
}

func (c *Check) runCheckDeviceWorker(workerID int, wg *sync.WaitGroup, jobs <-chan *devicecheck.DeviceCheck) {
	defer wg.Done()
	for job := range jobs {
		err := c.runCheckDevice(job)
		if err != nil {
			log.Errorf("worker %d : error collecting for device %s: %s", workerID, job.GetIPAddress(), err)
		}
	}
}

func (c *Check) runCheckDevice(deviceCk *devicecheck.DeviceCheck) error {
	collectionTime := timeNow()

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

	if c.config.IsDiscovery() {
		c.discovery = discovery.NewDiscovery(c.config)
		c.discovery.Start()
	} else {
		c.singleDeviceCk, err = devicecheck.NewDeviceCheck(c.config, c.config.IPAddress)
		if err != nil {
			return fmt.Errorf("failed to create device check: %s", err)
		}
	}
	return nil
}

// Cancel is called when check is unscheduled
func (c *Check) Cancel() {
	c.discovery.Stop()
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
