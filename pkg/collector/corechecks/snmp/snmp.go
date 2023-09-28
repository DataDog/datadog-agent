// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/devicecheck"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/discovery"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

var timeNow = time.Now

// Check aggregates metrics from one Check instance
type Check struct {
	core.CheckBase
	config                     *checkconfig.CheckConfig
	singleDeviceCk             *devicecheck.DeviceCheck
	discovery                  *discovery.Discovery
	sessionFactory             session.Factory
	workerRunDeviceCheckErrors *atomic.Uint64
}

// Run executes the check
func (c *Check) Run() error {
	var checkErr error
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	if c.config.IsDiscovery() {
		discoveredDevices := c.discovery.GetDiscoveredDeviceConfigs()

		jobs := make(chan *devicecheck.DeviceCheck, len(discoveredDevices))

		var wg sync.WaitGroup

		for w := 1; w <= c.config.Workers; w++ {
			wg.Add(1)
			go c.runCheckDeviceWorker(w, &wg, jobs)
		}

		for i := range discoveredDevices {
			deviceCk := discoveredDevices[i]
			hostname, err := deviceCk.GetDeviceHostname()
			if err != nil {
				log.Warnf("error getting hostname for device %s: %s", deviceCk.GetIPAddress(), err)
				continue
			}
			// `interface_configs` option not supported by SNMP corecheck autodiscovery
			deviceCk.SetSender(report.NewMetricSender(sender, hostname, nil))
			jobs <- deviceCk
		}
		close(jobs)
		wg.Wait() // wait for all workers to finish

		tags := append(c.config.GetStaticTags(), "network:"+c.config.Network)
		tags = append(tags, c.config.GetNetworkTags()...)
		sender.Gauge("snmp.discovered_devices_count", float64(len(discoveredDevices)), "", tags)
	} else {
		hostname, err := c.singleDeviceCk.GetDeviceHostname()
		if err != nil {
			return err
		}
		c.singleDeviceCk.SetSender(report.NewMetricSender(sender, hostname, c.config.InterfaceConfigs))
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
			c.workerRunDeviceCheckErrors.Inc()
			log.Errorf("worker %d : error collecting for device %s (total errors: %d): %s", workerID, job.GetIPAddress(), c.workerRunDeviceCheckErrors.Load(), err)
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
func (c *Check) Configure(senderManager sender.SenderManager, integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	var err error

	c.config, err = checkconfig.NewCheckConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("build config failed: %s", err)
	}
	log.Debugf("SNMP configuration: %s", c.config.ToString())

	if c.config.Name == "" {
		var checkName string
		// Set 'name' field of the instance if not already defined in rawInstance config.
		// The name/device_id will be used by Check.BuildID for building the check id.
		// Example of check id: `snmp:<DEVICE_ID>:a3ec59dfb03e4457`
		if c.config.IsDiscovery() {
			checkName = fmt.Sprintf("%s:%s", c.config.Namespace, c.config.Network)
		} else {
			checkName = c.config.DeviceID
		}
		setNameErr := rawInstance.SetNameForInstance(checkName)
		if setNameErr != nil {
			log.Warnf("error setting check name (checkName=%s): %s", checkName, setNameErr)
		}
	}

	// Must be called before c.CommonConfigure
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err = c.CommonConfigure(senderManager, integrationConfigDigest, rawInitConfig, rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	if c.config.IsDiscovery() {
		c.discovery = discovery.NewDiscovery(c.config, c.sessionFactory)
		c.discovery.Start()
	} else {
		c.singleDeviceCk, err = devicecheck.NewDeviceCheck(c.config, c.config.IPAddress, c.sessionFactory)
		if err != nil {
			return fmt.Errorf("failed to create device check: %s", err)
		}
	}
	return nil
}

// Cancel is called when check is unscheduled
func (c *Check) Cancel() {
	if c.discovery != nil {
		c.discovery.Stop()
		c.discovery = nil
	}
}

// Interval returns the scheduling time for the check
func (c *Check) Interval() time.Duration {
	return c.config.MinCollectionInterval
}

// GetDiagnoses collects diagnoses for diagnose CLI
func (c *Check) GetDiagnoses() ([]diagnosis.Diagnosis, error) {
	if c.config.IsDiscovery() {
		devices := c.discovery.GetDiscoveredDeviceConfigs()
		var diagnosis []diagnosis.Diagnosis

		for _, deviceCheck := range devices {
			diagnosis = append(diagnosis, deviceCheck.GetDiagnoses()...)
		}

		return diagnosis, nil
	}

	return c.singleDeviceCk.GetDiagnoses(), nil
}

func snmpFactory() check.Check {
	return &Check{
		CheckBase:                  core.NewCheckBase(common.SnmpIntegrationName),
		sessionFactory:             session.NewGosnmpSession,
		workerRunDeviceCheckErrors: atomic.NewUint64(0),
	}
}

func init() {
	core.RegisterCheck(common.SnmpIntegrationName, snmpFactory)
}
