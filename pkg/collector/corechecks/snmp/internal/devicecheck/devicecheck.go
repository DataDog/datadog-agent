// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package devicecheck

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cihub/seelog"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	coresnmp "github.com/DataDog/datadog-agent/pkg/snmp"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/fetch"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const (
	snmpLoaderTag           = "loader:core"
	serviceCheckName        = "snmp.can_check"
	deviceReachableMetric   = "snmp.device.reachable"
	deviceUnreachableMetric = "snmp.device.unreachable"
	deviceHostnamePrefix    = "device:"
)

// define timeNow as variable to make it possible to mock it during test
var timeNow = time.Now

// DeviceCheck hold info necessary to collect info for a single device
type DeviceCheck struct {
	config                 *checkconfig.CheckConfig
	sender                 *report.MetricSender
	session                session.Session
	sessionCloseErrorCount *atomic.Uint64
	savedDynamicTags       []string
	nextAutodetectMetrics  time.Time
}

// NewDeviceCheck returns a new DeviceCheck
func NewDeviceCheck(config *checkconfig.CheckConfig, ipAddress string, sessionFactory session.Factory) (*DeviceCheck, error) {
	newConfig := config.CopyWithNewIP(ipAddress)

	sess, err := sessionFactory(newConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to configure session: %s", err)
	}

	return &DeviceCheck{
		config:                 newConfig,
		session:                sess,
		sessionCloseErrorCount: atomic.NewUint64(0),
		nextAutodetectMetrics:  timeNow(),
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

// GetDeviceHostname returns DeviceID as hostname if UseDeviceIDAsHostname is true
func (d *DeviceCheck) GetDeviceHostname() (string, error) {
	if d.config.UseDeviceIDAsHostname {
		hostname := deviceHostnamePrefix + d.config.DeviceID
		normalizedHostname, err := validate.NormalizeHost(hostname)
		if err != nil {
			return "", err
		}
		return normalizedHostname, nil
	}
	return "", nil
}

// Run executes the check
func (d *DeviceCheck) Run(collectionTime time.Time) error {
	startTime := time.Now()
	staticTags := append(d.config.GetStaticTags(), d.config.GetNetworkTags()...)

	// Fetch and report metrics
	var checkErr error
	var deviceStatus metadata.DeviceStatus

	deviceReachable, dynamicTags, values, checkErr := d.getValuesAndTags()
	tags := common.CopyStrings(staticTags)
	if checkErr != nil {
		tags = append(tags, d.savedDynamicTags...)
		d.sender.ServiceCheck(serviceCheckName, servicecheck.ServiceCheckCritical, tags, checkErr.Error())
	} else {
		d.savedDynamicTags = dynamicTags
		tags = append(tags, dynamicTags...)
		d.sender.ServiceCheck(serviceCheckName, servicecheck.ServiceCheckOK, tags, "")
	}
	d.sender.Gauge(deviceReachableMetric, common.BoolToFloat64(deviceReachable), tags)
	d.sender.Gauge(deviceUnreachableMetric, common.BoolToFloat64(!deviceReachable), tags)

	if values != nil {
		d.sender.ReportMetrics(d.config.Metrics, values, tags)
	}

	if d.config.CollectDeviceMetadata {
		if deviceReachable {
			deviceStatus = metadata.DeviceStatusReachable
		} else {
			deviceStatus = metadata.DeviceStatusUnreachable
		}

		// We include instance tags to `deviceMetadataTags` since device metadata tags are not enriched with `checkSender.checkTags`.
		// `checkSender.checkTags` are added for metrics, service checks, events only.
		// Note that we don't add some extra tags like `service` tag that might be present in `checkSender.checkTags`.
		deviceMetadataTags := append(common.CopyStrings(tags), d.config.InstanceTags...)
		deviceMetadataTags = append(deviceMetadataTags, common.GetAgentVersionTag())

		d.sender.ReportNetworkDeviceMetadata(d.config, values, deviceMetadataTags, collectionTime, deviceStatus)
	}

	d.submitTelemetryMetrics(startTime, tags)
	d.setDeviceHostExternalTags()
	return checkErr
}

func (d *DeviceCheck) setDeviceHostExternalTags() {
	deviceHostname, err := d.GetDeviceHostname()
	if deviceHostname == "" || err != nil {
		return
	}
	agentTags := configUtils.GetConfiguredTags(config.Datadog, false)
	log.Debugf("Set external tags for device host, host=`%s`, agentTags=`%v`", deviceHostname, agentTags)
	externalhost.SetExternalTags(deviceHostname, common.SnmpExternalTagsSourceType, agentTags)
}

func (d *DeviceCheck) getValuesAndTags() (bool, []string, *valuestore.ResultValueStore, error) {
	var deviceReachable bool
	var checkErrors []string
	var tags []string

	// Create connection
	connErr := d.session.Connect()
	if connErr != nil {
		return false, tags, nil, fmt.Errorf("snmp connection error: %s", connErr)
	}
	defer func() {
		err := d.session.Close()
		if err != nil {
			d.sessionCloseErrorCount.Inc()
			log.Warnf("failed to close session (count: %d): %v", d.sessionCloseErrorCount.Load(), err)
		}
	}()

	// Check if the device is reachable
	getNextValue, err := d.session.GetNext([]string{coresnmp.DeviceReachableGetNextOid})
	if err != nil {
		deviceReachable = false
		checkErrors = append(checkErrors, fmt.Sprintf("check device reachable: failed: %s", err))
	} else {
		deviceReachable = true
		if log.ShouldLog(seelog.DebugLvl) {
			log.Debugf("check device reachable: success: %v", gosnmplib.PacketAsString(getNextValue))
		}
	}

	err = d.detectMetricsToMonitor(d.session)
	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to autodetect profile: %s", err))
	}

	tags = append(tags, d.config.ProfileTags...)

	valuesStore, err := fetch.Fetch(d.session, d.config)
	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("fetched values: %v", valuestore.ResultValueStoreAsString(valuesStore))
	}

	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to fetch values: %s", err))
	} else {
		tags = append(tags, d.sender.GetCheckInstanceMetricTags(d.config.MetricTags, valuesStore)...)
	}

	var joinedError error
	if len(checkErrors) > 0 {
		joinedError = errors.New(strings.Join(checkErrors, "; "))
	}
	return deviceReachable, tags, valuesStore, joinedError
}

func (d *DeviceCheck) detectMetricsToMonitor(sess session.Session) error {
	if d.config.DetectMetricsEnabled {
		if d.nextAutodetectMetrics.After(timeNow()) {
			return nil
		}
		d.nextAutodetectMetrics = d.nextAutodetectMetrics.Add(time.Duration(d.config.DetectMetricsRefreshInterval) * time.Second)

		detectedMetrics, metricTagConfigs := d.detectAvailableMetrics()
		log.Debugf("detected metrics: %v", detectedMetrics)
		d.config.SetAutodetectProfile(detectedMetrics, metricTagConfigs)
	} else if d.config.AutodetectProfile {
		// detect using sysObjectID
		sysObjectID, err := session.FetchSysObjectID(sess)
		if err != nil {
			return fmt.Errorf("failed to fetch sysobjectid: %s", err)
		}
		profile, err := checkconfig.GetProfileForSysObjectID(d.config.Profiles, sysObjectID)
		if err != nil {
			return fmt.Errorf("failed to get profile sys object id for `%s`: %s", sysObjectID, err)
		}
		if profile != d.config.Profile {
			log.Debugf("detected profile change: %s -> %s", d.config.Profile, profile)
			err = d.config.SetProfile(profile)
			if err != nil {
				// Should not happen since the profile is one of those we matched in GetProfileForSysObjectID
				return fmt.Errorf("failed to refresh with profile `%s` detected using sysObjectID `%s`: %s", profile, sysObjectID, err)
			}
		}
	}
	return nil
}

func (d *DeviceCheck) detectAvailableMetrics() ([]profiledefinition.MetricsConfig, []profiledefinition.MetricTagConfig) {
	fetchedOIDs := session.FetchAllOIDsUsingGetNext(d.session)
	log.Debugf("fetched OIDs: %v", fetchedOIDs)

	root := common.BuildOidTrie(fetchedOIDs)
	if log.ShouldLog(seelog.DebugLvl) {
		root.DebugPrint()
	}

	var metricConfigs []profiledefinition.MetricsConfig
	var metricTagConfigs []profiledefinition.MetricTagConfig

	// If a metric name has already been encountered, we won't try to add it again.
	alreadySeenMetrics := make(map[string]bool)
	// If a global tag has already been encountered, we won't try to add it again.
	alreadyGlobalTags := make(map[string]bool)
	for _, profileConfig := range d.config.Profiles {
		for _, metricConfig := range profileConfig.Definition.Metrics {
			newMetricConfig := metricConfig
			if metricConfig.IsScalar() {
				metricName := metricConfig.Symbol.Name
				if metricConfig.Options.MetricSuffix != "" {
					metricName = metricName + "." + metricConfig.Options.MetricSuffix
				}
				if !alreadySeenMetrics[metricName] && root.LeafExist(metricConfig.Symbol.OID) {
					alreadySeenMetrics[metricName] = true
					metricConfigs = append(metricConfigs, newMetricConfig)
				}
			} else if metricConfig.IsColumn() {
				newMetricConfig.Symbols = []profiledefinition.SymbolConfig{}
				for _, symbol := range metricConfig.Symbols {
					if !alreadySeenMetrics[symbol.Name] && root.NonLeafNodeExist(symbol.OID) {
						alreadySeenMetrics[symbol.Name] = true
						newMetricConfig.Symbols = append(newMetricConfig.Symbols, symbol)
					}
				}
				if len(newMetricConfig.Symbols) > 0 {
					metricConfigs = append(metricConfigs, newMetricConfig)
				}
			}
		}
		for _, metricTag := range profileConfig.Definition.MetricTags {
			if root.LeafExist(metricTag.OID) || root.LeafExist(metricTag.Column.OID) {
				if metricTag.Tag != "" {
					if alreadyGlobalTags[metricTag.Tag] {
						continue
					}
					alreadyGlobalTags[metricTag.Tag] = true
				} else {
					// We don't add `metricTag` if any of the `metricTag.Tags` has already been encountered.
					alreadyPresent := false
					for tagKey := range metricTag.Tags {
						if alreadyGlobalTags[tagKey] {
							alreadyPresent = true
							break
						}
					}
					if alreadyPresent {
						continue
					}
					for tagKey := range metricTag.Tags {
						alreadyGlobalTags[tagKey] = true
					}
				}
				metricTagConfigs = append(metricTagConfigs, metricTag)
			}
		}
	}
	return metricConfigs, metricTagConfigs
}

func (d *DeviceCheck) submitTelemetryMetrics(startTime time.Time, tags []string) {
	newTags := append(common.CopyStrings(tags), snmpLoaderTag, common.GetAgentVersionTag())

	d.sender.Gauge("snmp.devices_monitored", float64(1), newTags)

	// SNMP Performance metrics
	d.sender.MonotonicCount("datadog.snmp.check_interval", time.Duration(startTime.UnixNano()).Seconds(), newTags)
	d.sender.Gauge("datadog.snmp.check_duration", time.Since(startTime).Seconds(), newTags)
	d.sender.Gauge("datadog.snmp.submitted_metrics", float64(d.sender.GetSubmittedMetrics()), newTags)
}
