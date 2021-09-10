package devicecheck

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/fetch"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/valuestore"
)

const (
	snmpLoaderTag    = "loader:core"
	serviceCheckName = "snmp.can_check"
)

// DeviceCheck hold info necessary to collect info for a single device
type DeviceCheck struct {
	config  *checkconfig.CheckConfig
	sender  *report.MetricSender
	session session.Session
}

// NewDeviceCheck returns a new DeviceCheck
func NewDeviceCheck(config *checkconfig.CheckConfig, ipAddress string) (*DeviceCheck, error) {
	newConfig := config.CopyWithNewIP(ipAddress)

	sess, err := session.NewSession(newConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to configure session: %s", err)
	}

	return &DeviceCheck{
		config:  newConfig,
		session: sess,
	}, nil
}

// SetSender sets the current sender
func (d *DeviceCheck) SetSender(sender *report.MetricSender) {
	d.sender = sender
}

// GetIPAddress returns device IP
func (d *DeviceCheck) GetIPAddress() string {
	return d.config.IPAddress
}

// GetIDTags returns device IDTags
func (d *DeviceCheck) GetIDTags() []string {
	return d.config.DeviceIDTags
}

// Run executes the check
func (d *DeviceCheck) Run(collectionTime time.Time) error {
	startTime := time.Now()
	staticTags := append(d.config.GetStaticTags(), d.config.GetNetworkTags()...)

	// Fetch and report metrics
	var checkErr error
	var deviceStatus metadata.DeviceStatus
	tags, values, checkErr := d.getValuesAndTags(staticTags)
	if checkErr != nil {
		d.sender.ServiceCheck(serviceCheckName, metrics.ServiceCheckCritical, "", tags, checkErr.Error())
	} else {
		d.sender.ServiceCheck(serviceCheckName, metrics.ServiceCheckOK, "", tags, "")
	}
	if values != nil {
		d.sender.ReportMetrics(d.config.Metrics, values, tags)
	}

	if d.config.CollectDeviceMetadata {
		if values != nil {
			deviceStatus = metadata.DeviceStatusReachable
		} else {
			deviceStatus = metadata.DeviceStatusUnreachable
		}

		// We include instance tags to `deviceMetadataTags` since device metadata tags are not enriched with `checkSender.checkTags`.
		// `checkSender.checkTags` are added for metrics, service checks, events only.
		// Note that we don't add some extra tags like `service` tag that might be present in `checkSender.checkTags`.
		deviceMetadataTags := append(common.CopyStrings(tags), d.config.InstanceTags...)

		d.sender.ReportNetworkDeviceMetadata(d.config, values, deviceMetadataTags, collectionTime, deviceStatus)
	}

	d.submitTelemetryMetrics(startTime, tags)
	return checkErr
}

func (d *DeviceCheck) getValuesAndTags(staticTags []string) ([]string, *valuestore.ResultValueStore, error) {
	var checkErrors []string
	tags := common.CopyStrings(staticTags)

	// Create connection
	connErr := d.session.Connect()
	if connErr != nil {
		return tags, nil, fmt.Errorf("snmp connection error: %s", connErr)
	}
	defer func() {
		err := d.session.Close()
		if err != nil {
			log.Warnf("failed to close session: %v", err)
		}
	}()

	err := d.doAutodetectProfile(d.session)
	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to autodetect profile: %s", err))
	}

	tags = append(tags, d.config.ProfileTags...)

	valuesStore, err := fetch.Fetch(d.session, d.config)
	log.Debugf("fetched values: %v", valuesStore)

	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to fetch values: %s", err))
	} else {
		tags = append(tags, d.sender.GetCheckInstanceMetricTags(d.config.MetricTags, valuesStore)...)
	}

	var joinedError error
	if len(checkErrors) > 0 {
		joinedError = errors.New(strings.Join(checkErrors, "; "))
	}
	return tags, valuesStore, joinedError
}

func (d *DeviceCheck) doAutodetectProfile(sess session.Session) error {
	// Try to detect profile using device sysobjectid
	if d.config.AutodetectProfile {
		sysObjectID, err := session.FetchSysObjectID(sess)
		if err != nil {
			return fmt.Errorf("failed to fetch sysobjectid: %s", err)
		}
		d.config.AutodetectProfile = false // do not try to auto detect profile next time

		profile, err := checkconfig.GetProfileForSysObjectID(d.config.Profiles, sysObjectID)
		if err != nil {
			return fmt.Errorf("failed to get profile sys object id for `%s`: %s", sysObjectID, err)
		}
		err = d.config.RefreshWithProfile(profile)
		if err != nil {
			// Should not happen since the profile is one of those we matched in GetProfileForSysObjectID
			return fmt.Errorf("failed to refresh with profile `%s` detected using sysObjectID `%s`: %s", profile, sysObjectID, err)
		}
	}
	return nil
}

func (d *DeviceCheck) submitTelemetryMetrics(startTime time.Time, tags []string) {
	newTags := append(common.CopyStrings(tags), snmpLoaderTag)

	d.sender.Gauge("snmp.devices_monitored", float64(1), "", newTags)

	// SNMP Performance metrics
	d.sender.MonotonicCount("datadog.snmp.check_interval", time.Duration(startTime.UnixNano()).Seconds(), "", newTags)
	d.sender.Gauge("datadog.snmp.check_duration", time.Since(startTime).Seconds(), "", newTags)
	d.sender.Gauge("datadog.snmp.submitted_metrics", float64(d.sender.GetSubmittedMetrics()), "", newTags)
}
