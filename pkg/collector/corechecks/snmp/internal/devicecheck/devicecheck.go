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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/externalhost"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	coresnmp "github.com/DataDog/datadog-agent/pkg/snmp"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/fetch"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/report"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

const (
	snmpLoaderTag        = "loader:core"
	serviceCheckName     = "snmp.can_check"
	deviceHostnamePrefix = "device:"
)

// DeviceCheck hold info necessary to collect info for a single device
type DeviceCheck struct {
	config  *checkconfig.CheckConfig
	sender  *report.MetricSender
	session session.Session
}

// NewDeviceCheck returns a new DeviceCheck
func NewDeviceCheck(config *checkconfig.CheckConfig, ipAddress string, sessionFactory session.Factory) (*DeviceCheck, error) {
	newConfig := config.CopyWithNewIP(ipAddress)

	sess, err := sessionFactory(newConfig)
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

// GetDeviceHostname returns DeviceID as hostname if UseDeviceIDAsHostname is true
func (d *DeviceCheck) GetDeviceHostname() (string, error) {
	if d.config.UseDeviceIDAsHostname {
		hostname := deviceHostnamePrefix + d.config.DeviceID
		normalizedHostname, err := util.NormalizeHost(hostname)
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
	deviceReachable, tags, values, checkErr := d.getValuesAndTags(staticTags)
	if checkErr != nil {
		d.sender.ServiceCheck(serviceCheckName, metrics.ServiceCheckCritical, tags, checkErr.Error())
	} else {
		d.sender.ServiceCheck(serviceCheckName, metrics.ServiceCheckOK, tags, "")
	}
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
	agentTags := config.GetConfiguredTags(false)
	log.Debugf("Set external tags for device host, host=`%s`, agentTags=`%v`", deviceHostname, agentTags)
	externalhost.SetExternalTags(deviceHostname, common.SnmpExternalTagsSourceType, agentTags)
}

func (d *DeviceCheck) getValuesAndTags(staticTags []string) (bool, []string, *valuestore.ResultValueStore, error) {
	var deviceReachable bool
	var checkErrors []string
	tags := common.CopyStrings(staticTags)

	// Create connection
	connErr := d.session.Connect()
	if connErr != nil {
		return false, tags, nil, fmt.Errorf("snmp connection error: %s", connErr)
	}
	defer func() {
		err := d.session.Close()
		if err != nil {
			log.Warnf("failed to close session: %v", err)
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

	err = d.doAutodetectProfile(d.session)
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
	newTags := append(common.CopyStrings(tags), snmpLoaderTag, common.GetAgentVersionTag())

	d.sender.Gauge("snmp.devices_monitored", float64(1), newTags)

	// SNMP Performance metrics
	d.sender.MonotonicCount("datadog.snmp.check_interval", time.Duration(startTime.UnixNano()).Seconds(), newTags)
	d.sender.Gauge("datadog.snmp.check_duration", time.Since(startTime).Seconds(), newTags)
	d.sender.Gauge("datadog.snmp.submitted_metrics", float64(d.sender.GetSubmittedMetrics()), newTags)
}
