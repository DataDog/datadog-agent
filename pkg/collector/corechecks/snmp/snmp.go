package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	snmpCheckName    = "snmp"
	snmpLoaderTag    = "loader:core"
	serviceCheckName = "snmp.can_check"
)

var timeNow = time.Now

// Check aggregates metrics from one Check instance
type Check struct {
	core.CheckBase
	config  snmpConfig
	session sessionAPI
	sender  metricSender
}

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	c.sender = metricSender{sender: sender}

	staticTags := c.config.getStaticTags()

	// Fetch and report metrics
	var checkErr error
	var deviceStatus metadata.DeviceStatus
	collectionTime := timeNow()
	tags, values, checkErr := c.getValuesAndTags(staticTags)
	if checkErr != nil {
		deviceStatus = metadata.DeviceStatusUnreachable
		c.sender.serviceCheck(serviceCheckName, metrics.ServiceCheckCritical, "", tags, checkErr.Error())
	} else {
		deviceStatus = metadata.DeviceStatusReachable
		c.sender.serviceCheck(serviceCheckName, metrics.ServiceCheckOK, "", tags, "")
		c.sender.reportMetrics(c.config.metrics, values, tags)
	}

	if c.config.collectDeviceMetadata {
		// We include instance tags to `deviceMetadataTags` since device metadata tags are not enriched with `checkSender.checkTags`.
		// `checkSender.checkTags` are added for metrics, service checks, events only.
		// Note that we don't add some extra tags like `service` tag that might be present in `checkSender.checkTags`.
		deviceMetadataTags := append(copyStrings(tags), c.config.instanceTags...)
		c.sender.reportNetworkDeviceMetadata(c.config, values, deviceMetadataTags, collectionTime, deviceStatus)
	}

	c.submitTelemetryMetrics(startTime, tags)

	// Commit
	sender.Commit()
	return checkErr
}

func (c *Check) getValuesAndTags(staticTags []string) ([]string, *resultValueStore, error) {
	tags := copyStrings(staticTags)

	// Create connection
	connErr := c.session.Connect()
	if connErr != nil {
		return tags, nil, fmt.Errorf("snmp connection error: %s", connErr)
	}
	defer func() {
		err := c.session.Close()
		if err != nil {
			log.Warnf("failed to close session: %v", err)
		}
	}()

	// Try to detect profile using device sysobjectid
	if c.config.autodetectProfile {
		sysObjectID, err := fetchSysObjectID(c.session)
		if err != nil {
			return tags, nil, fmt.Errorf("failed to fetching sysobjectid: %s", err)
		}
		c.config.autodetectProfile = false // do not try to auto detect profile next time

		profile, err := getProfileForSysObjectID(c.config.profiles, sysObjectID)
		if err != nil {
			return tags, nil, fmt.Errorf("failed to get profile sys object id for `%s`: %s", sysObjectID, err)
		}
		err = c.config.refreshWithProfile(profile)
		if err != nil {
			// Should not happen since the profile is one of those we matched in getProfileForSysObjectID
			return tags, nil, fmt.Errorf("failed to refresh with profile `%s` detected using sysObjectID `%s`: %s", profile, sysObjectID, err)
		}
	}
	tags = append(tags, c.config.profileTags...)

	valuesStore, err := fetchValues(c.session, c.config)
	log.Debugf("fetched values: %v", valuesStore)

	if err != nil {
		return tags, nil, fmt.Errorf("failed to fetch values: %s", err)
	}
	tags = append(tags, c.sender.getCheckInstanceMetricTags(c.config.metricTags, valuesStore)...)
	return tags, valuesStore, nil
}

func (c *Check) submitTelemetryMetrics(startTime time.Time, tags []string) {
	newTags := append(copyStrings(tags), snmpLoaderTag)

	c.sender.gauge("snmp.devices_monitored", float64(1), "", newTags)

	// SNMP Performance metrics
	c.sender.monotonicCount("datadog.snmp.check_interval", time.Duration(startTime.UnixNano()).Seconds(), "", newTags)
	c.sender.gauge("datadog.snmp.check_duration", time.Since(startTime).Seconds(), "", newTags)
	c.sender.gauge("datadog.snmp.submitted_metrics", float64(c.sender.submittedMetrics), "", newTags)
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
	err = c.session.Configure(c.config)
	if err != nil {
		return fmt.Errorf("session configure failed: %s", err)
	}

	return nil
}

func snmpFactory() check.Check {
	return &Check{
		session:   &snmpSession{},
		CheckBase: core.NewCheckBase(snmpCheckName),
	}
}

func init() {
	core.RegisterCheck(snmpCheckName, snmpFactory)
}
