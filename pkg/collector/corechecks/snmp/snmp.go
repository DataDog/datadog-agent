package snmp

import (
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/fetch"
	session "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
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
	config  checkconfig.CheckConfig
	session session.Session
	sender  MetricSender
}

// Run executes the check
func (c *Check) Run() error {
	startTime := time.Now()

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	c.sender = MetricSender{Sender: sender}

	staticTags := c.config.GetStaticTags()

	// Fetch and report metrics
	var checkErr error
	var deviceStatus metadata.DeviceStatus
	collectionTime := timeNow()
	tags, values, checkErr := c.getValuesAndTags(staticTags)
	if checkErr != nil {
		c.sender.ServiceCheck(serviceCheckName, metrics.ServiceCheckCritical, "", tags, checkErr.Error())
	} else {
		c.sender.ServiceCheck(serviceCheckName, metrics.ServiceCheckOK, "", tags, "")
	}
	if values != nil {
		c.sender.ReportMetrics(c.config.Metrics, values, tags)
	}

	if c.config.CollectDeviceMetadata {
		if values != nil {
			deviceStatus = metadata.DeviceStatusReachable
		} else {
			deviceStatus = metadata.DeviceStatusUnreachable
		}

		// We include instance tags to `deviceMetadataTags` since device metadata tags are not enriched with `checkSender.checkTags`.
		// `checkSender.checkTags` are added for metrics, service checks, events only.
		// Note that we don't add some extra tags like `service` tag that might be present in `checkSender.checkTags`.
		deviceMetadataTags := append(common.CopyStrings(tags), c.config.InstanceTags...)
		c.sender.reportNetworkDeviceMetadata(c.config, values, deviceMetadataTags, collectionTime, deviceStatus)
	}

	c.submitTelemetryMetrics(startTime, tags)

	// Commit
	sender.Commit()
	return checkErr
}

func (c *Check) getValuesAndTags(staticTags []string) ([]string, *valuestore.ResultValueStore, error) {
	var checkErrors []string
	tags := common.CopyStrings(staticTags)

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

	err := c.autodetectProfile(c.session)
	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to autodetect profile: %s", err))
	}

	tags = append(tags, c.config.ProfileTags...)

	valuesStore, err := fetch.Fetch(c.session, c.config)
	log.Debugf("fetched values: %v", valuesStore)

	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to fetch values: %s", err))
	} else {
		tags = append(tags, c.sender.getCheckInstanceMetricTags(c.config.MetricTags, valuesStore)...)
	}

	var joinedError error
	if len(checkErrors) > 0 {
		joinedError = errors.New(strings.Join(checkErrors, "; "))
	}
	return tags, valuesStore, joinedError
}

func (c *Check) autodetectProfile(sess session.Session) error {
	// Try to detect profile using device sysobjectid
	if c.config.AutodetectProfile {
		sysObjectID, err := session.FetchSysObjectID(sess)
		if err != nil {
			return fmt.Errorf("failed to fetch sysobjectid: %s", err)
		}
		c.config.AutodetectProfile = false // do not try to auto detect profile next time

		profile, err := checkconfig.GetProfileForSysObjectID(c.config.Profiles, sysObjectID)
		if err != nil {
			return fmt.Errorf("failed to get profile sys object id for `%s`: %s", sysObjectID, err)
		}
		err = c.config.RefreshWithProfile(profile)
		if err != nil {
			// Should not happen since the profile is one of those we matched in GetProfileForSysObjectID
			return fmt.Errorf("failed to refresh with profile `%s` detected using sysObjectID `%s`: %s", profile, sysObjectID, err)
		}
	}
	return nil
}

func (c *Check) submitTelemetryMetrics(startTime time.Time, tags []string) {
	newTags := append(common.CopyStrings(tags), snmpLoaderTag)

	c.sender.Gauge("snmp.devices_monitored", float64(1), "", newTags)

	// SNMP Performance metrics
	c.sender.MonotonicCount("datadog.snmp.check_interval", time.Duration(startTime.UnixNano()).Seconds(), "", newTags)
	c.sender.Gauge("datadog.snmp.check_duration", time.Since(startTime).Seconds(), "", newTags)
	c.sender.Gauge("datadog.snmp.submitted_metrics", float64(c.sender.SubmittedMetrics), "", newTags)
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
