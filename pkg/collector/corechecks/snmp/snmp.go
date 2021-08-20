package snmp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
)

const (
	snmpCheckName    = "snmp"
	snmpLoaderTag    = "loader:core"
	serviceCheckName = "snmp.can_check"
)

var timeNow = time.Now

//var sessionFactory = createSession
//
//func createSession(config snmpConfig) (sessionAPI, error) {
//	var s sessionAPI
//	s = &snmpSession{}
//	err := s.Configure(snmpConfig{})
//	if err != nil {
//		return nil, err
//	}
//	return s, nil
//}

// Check aggregates metrics from one Check instance
type Check struct {
	core.CheckBase
	config    *snmpConfig
	discovery snmpDiscovery
}

// Run executes the check
func (c *Check) Run() error {
	var checkErr error
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	if c.config.network != "" {
		var discoveredDevices []*snmpConfig
		if c.config.testInstances == 0 {
			discoveredDevices = c.discovery.getDiscoveredDeviceConfigs(sender)
		} else {
			discoveredDevices = c.discovery.getDiscoveredDeviceConfigsTestInstances(c.config.testInstances, sender)
		}

		jobs := make(chan *snmpConfig, len(discoveredDevices))

		for w := 1; w <= c.config.workers; w++ {
			go c.runCheckDeviceWorker(w, jobs)
		}

		for i := range discoveredDevices {
			config := discoveredDevices[i]
			log.Warnf("[DEV] schedule device collection: %s, tags: %v", config.ipAddress, config.getDeviceIDTags())
			//checkErr = c.runCheckDevice(config)
			jobs <- config
		}
		close(jobs)

	} else {
		// TODO: sender submittedMetrics state, so need to be per config/device level
		c.config.sender = metricSender{sender: sender}
		checkErr = c.runCheckDevice(c.config)
	}

	// Commit
	sender.Commit()
	return checkErr
}

func (c *Check) runCheckDeviceWorker(workerID int, jobs <-chan *snmpConfig) {
	for job := range jobs {
		log.Warnf("[DEV] worker %d starting collect device %s, tags %s, session %p", workerID, job.ipAddress, job.getDeviceIDTags(), job.session)
		err := c.runCheckDevice(job)
		if err != nil {
			log.Warnf("[DEV] worker %d error collect device %s: %s", workerID, job.ipAddress, err)
			continue
		}
		log.Warnf("[DEV] worker %d done collect device %s ", workerID, job.ipAddress)
	}
}

func (c *Check) runCheckDevice(config *snmpConfig) error {
	log.Warnf("[DEV] collect for device: %s (tags: %v)", config.ipAddress, config.getDeviceIDTags())
	startTime := time.Now()
	staticTags := config.getStaticTags()

	// Fetch and report metrics
	var checkErr error
	var deviceStatus metadata.DeviceStatus
	collectionTime := timeNow()
	tags, values, checkErr := c.getValuesAndTags(config, staticTags)
	if checkErr != nil {
		config.sender.serviceCheck(serviceCheckName, metrics.ServiceCheckCritical, "", tags, checkErr.Error())
	} else {
		config.sender.serviceCheck(serviceCheckName, metrics.ServiceCheckOK, "", tags, "")
	}
	if values != nil {
		config.sender.reportMetrics(config.metrics, values, tags)
	}

	if config.collectDeviceMetadata {
		if values != nil {
			deviceStatus = metadata.DeviceStatusReachable
		} else {
			deviceStatus = metadata.DeviceStatusUnreachable
		}

		// We include instance tags to `deviceMetadataTags` since device metadata tags are not enriched with `checkSender.checkTags`.
		// `checkSender.checkTags` are added for metrics, service checks, events only.
		// Note that we don't add some extra tags like `service` tag that might be present in `checkSender.checkTags`.
		deviceMetadataTags := append(copyStrings(tags), config.instanceTags...)
		config.sender.reportNetworkDeviceMetadata(config, values, deviceMetadataTags, collectionTime, deviceStatus)
	}

	c.submitTelemetryMetrics(config, startTime, tags)
	return checkErr
}

func (c *Check) getValuesAndTags(config *snmpConfig, staticTags []string) ([]string, *resultValueStore, error) {
	var checkErrors []string
	tags := copyStrings(staticTags)

	// Create connection
	connErr := config.session.Connect()
	if connErr != nil {
		return tags, nil, fmt.Errorf("snmp connection error: %s", connErr)
	}
	defer func() {
		err := config.session.Close()
		if err != nil {
			log.Warnf("failed to close session: %v", err)
		}
	}()

	err := c.autodetectProfile(config)
	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to autodetect profile: %s", err))
	}

	tags = append(tags, config.profileTags...)

	valuesStore, err := fetchValues(config.session, config)
	log.Debugf("fetched values: %v", valuesStore)

	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to fetch values: %s", err))
	} else {
		tags = append(tags, c.config.sender.getCheckInstanceMetricTags(config.metricTags, valuesStore)...)
	}

	var joinedError error
	if len(checkErrors) > 0 {
		joinedError = errors.New(strings.Join(checkErrors, "; "))
	}
	return tags, valuesStore, joinedError
}

func (c *Check) autodetectProfile(config *snmpConfig) error {
	// Try to detect profile using device sysobjectid
	// TODO: use per device config?
	if config.autodetectProfile {
		sysObjectID, err := fetchSysObjectID(config.session)
		if err != nil {
			return fmt.Errorf("failed to fetch sysobjectid: %s", err)
		}
		config.autodetectProfile = false // do not try to auto detect profile next time

		profile, err := getProfileForSysObjectID(config.profiles, sysObjectID)
		if err != nil {
			return fmt.Errorf("failed to get profile sys object id for `%s`: %s", sysObjectID, err)
		}
		err = config.refreshWithProfile(profile)
		if err != nil {
			// Should not happen since the profile is one of those we matched in getProfileForSysObjectID
			return fmt.Errorf("failed to refresh with profile `%s` detected using sysObjectID `%s`: %s", profile, sysObjectID, err)
		}
	}
	return nil
}

func (c *Check) submitTelemetryMetrics(config *snmpConfig, startTime time.Time, tags []string) {
	newTags := append(copyStrings(tags), snmpLoaderTag)

	config.sender.gauge("snmp.devices_monitored", float64(1), "", newTags)

	// SNMP Performance metrics
	config.sender.monotonicCount("datadog.snmp.check_interval", time.Duration(startTime.UnixNano()).Seconds(), "", newTags)
	config.sender.gauge("datadog.snmp.check_duration", time.Since(startTime).Seconds(), "", newTags)
	config.sender.gauge("datadog.snmp.submitted_metrics", float64(config.sender.submittedMetrics), "", newTags)
}

// Configure configures the snmp checks
func (c *Check) Configure(rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	// Must be called before c.CommonConfigure
	c.BuildID(rawInstance, rawInitConfig)

	err := c.CommonConfigure(rawInstance, source)
	if err != nil {
		return fmt.Errorf("common configure failed: %s", err)
	}

	config, err := buildConfig(rawInstance, rawInitConfig)
	if err != nil {
		return fmt.Errorf("build config failed: %s", err)
	}

	log.Debugf("SNMP configuration: %s", config.toString())

	c.config = config
	err = c.config.session.Configure(c.config)
	if err != nil {
		return fmt.Errorf("session configure failed: %s", err)
	}

	if c.config.network != "" {
		log.Warnf("[DEV] Network: %s", c.config.network)
		c.discovery = newSnmpDiscovery(c.config)
		c.discovery.Start()
	}

	return nil
}

// Interval returns the scheduling time for the check
func (c *Check) Interval() time.Duration {
	return c.config.minCollectionInterval
}

// Cancel is called when check is unscheduled
func (c *Check) Cancel() {
	log.Warnf("[DEV] Cancel called for check %s", c.ID())
	c.discovery.stop <- true
}

func snmpFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(snmpCheckName),
	}
}

func init() {
	core.RegisterCheck(snmpCheckName, snmpFactory)
}
