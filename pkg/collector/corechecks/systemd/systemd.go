// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package systemd

import (
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-systemd/dbus"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/inventories"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	systemdCheckName = "systemd"

	unitActiveState = "active"
	unitLoadedState = "loaded"

	typeUnit    = "unit"
	typeService = "service"
	typeSocket  = "socket"

	canConnectServiceCheck   = "systemd.can_connect"
	systemStateServiceCheck  = "systemd.system.state"
	unitStateServiceCheck    = "systemd.unit.state"
	unitSubStateServiceCheck = "systemd.unit.substate"
)

var dbusTypeMap = map[string]string{
	typeUnit:    "Unit",
	typeService: "Service",
	typeSocket:  "Socket",
}

// metricConfigItem map a metric to a systemd unit property.
type metricConfigItem struct {
	metricName         string
	propertyName       string
	accountingProperty string
	optional           bool // if optional log as debug when there is an issue getting the property, otherwise log as error
}

// metricConfigs contains metricConfigItem(s) grouped by unit type.
// TODO: Instead of using `optional`, use SystemD version to decide if a attribute/metric should be processed or not.
var metricConfigs = map[string][]metricConfigItem{
	typeService: {
		{
			// only present from systemd v220
			// https://github.com/systemd/systemd/blob/dd0395b5654c52e982adf6d354db9c7fdcf4b6c7/NEWS#L5571-L5576
			metricName:         "systemd.service.cpu_time_consumed",
			propertyName:       "CPUUsageNSec",
			accountingProperty: "CPUAccounting",
			optional:           true,
		},
		{
			metricName:         "systemd.service.memory_usage",
			propertyName:       "MemoryCurrent",
			accountingProperty: "MemoryAccounting",
		},
		{
			// only present from systemd v227
			// https://github.com/systemd/systemd/blob/dd0395b5654c52e982adf6d354db9c7fdcf4b6c7/NEWS#L4980-L4984
			// https://montecristosoftware.eu/matteo/systemd/commit/03a7b521e3ffb7f5d153d90480ba5d4bc29d1e8f#6e0729ff5b041f3624fb339e9484dbfad911e297_799_823
			metricName:         "systemd.service.task_count",
			propertyName:       "TasksCurrent",
			accountingProperty: "TasksAccounting",
			optional:           true,
		},
		{
			// only present from systemd v235
			// https://github.com/systemd/systemd/blob/dd0395b5654c52e982adf6d354db9c7fdcf4b6c7/NEWS#L3027-L3029
			metricName:   "systemd.service.restart_count",
			propertyName: "NRestarts",
			optional:     true,
		},
	},
	typeSocket: {
		{
			metricName:   "systemd.socket.connection_accepted_count",
			propertyName: "NAccepted",
		},
		{
			metricName:   "systemd.socket.connection_count",
			propertyName: "NConnections",
		},
		{
			// only present from systemd v239
			// https://github.com/systemd/systemd/blob/dd0395b5654c52e982adf6d354db9c7fdcf4b6c7/NEWS#L2256-L2258
			metricName:   "systemd.socket.connection_refused_count",
			propertyName: "NRefused",
			optional:     true,
		},
	},
}

var unitActiveStates = []string{"active", "activating", "inactive", "deactivating", "failed"}

var validServiceCheckStatus = []string{
	"ok",
	"warning",
	"critical",
	"unknown",
}

var serviceCheckStateMapping = map[string]string{
	"active":       "ok",
	"inactive":     "critical",
	"failed":       "critical",
	"activating":   "unknown",
	"deactivating": "unknown",
}

var systemdStatusMapping = map[string]metrics.ServiceCheckStatus{
	"initializing": metrics.ServiceCheckUnknown,
	"starting":     metrics.ServiceCheckUnknown,
	"running":      metrics.ServiceCheckOK,
	"degraded":     metrics.ServiceCheckCritical,
	"maintenance":  metrics.ServiceCheckCritical,
	"stopping":     metrics.ServiceCheckCritical,
}

// SystemdCheck aggregates metrics from one SystemdCheck instance
type SystemdCheck struct {
	core.CheckBase
	stats  systemdStats
	config systemdConfig
}
type unitSubstateMapping = map[string]string

type systemdInstanceConfig struct {
	PrivateSocket         string                         `yaml:"private_socket"`
	UnitNames             []string                       `yaml:"unit_names"`
	SubstateStatusMapping map[string]unitSubstateMapping `yaml:"substate_status_mapping"`
}

type systemdInitConfig struct{}

type systemdConfig struct {
	instance systemdInstanceConfig
	initConf systemdInitConfig
}

type systemdStats interface {
	// Dbus Connection
	PrivateSocketConnection(privateSocket string) (*dbus.Conn, error)
	SystemBusSocketConnection() (*dbus.Conn, error)
	CloseConn(c *dbus.Conn)

	// System Data
	SystemState(c *dbus.Conn) (*dbus.Property, error)
	ListUnits(c *dbus.Conn) ([]dbus.UnitStatus, error)
	GetUnitTypeProperties(c *dbus.Conn, unitName string, unitType string) (map[string]interface{}, error)
	GetVersion(c *dbus.Conn) (string, error)

	// Misc
	UnixNow() int64
}

type defaultSystemdStats struct{}

func (s *defaultSystemdStats) PrivateSocketConnection(privateSocket string) (*dbus.Conn, error) {
	return NewSystemdConnection(privateSocket)
}

func (s *defaultSystemdStats) SystemBusSocketConnection() (*dbus.Conn, error) {
	return dbus.NewSystemConnection()
}

func (s *defaultSystemdStats) CloseConn(c *dbus.Conn) {
	c.Close()
}

func (s *defaultSystemdStats) SystemState(c *dbus.Conn) (*dbus.Property, error) {
	return c.SystemState()
}

func (s *defaultSystemdStats) ListUnits(conn *dbus.Conn) ([]dbus.UnitStatus, error) {
	return conn.ListUnits()
}

func (s *defaultSystemdStats) GetUnitTypeProperties(c *dbus.Conn, unitName string, unitType string) (map[string]interface{}, error) {
	return c.GetUnitTypeProperties(unitName, unitType)
}

func (s *defaultSystemdStats) GetVersion(c *dbus.Conn) (string, error) {
	return c.GetManagerProperty("Version")
}

func (s *defaultSystemdStats) UnixNow() int64 {
	return time.Now().Unix()
}

// Run executes the check
func (c *SystemdCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	conn, err := c.connect(sender)
	if err != nil {
		return err
	}
	defer c.stats.CloseConn(conn)

	c.submitVersion(conn)
	c.submitSystemdState(sender, conn)

	err = c.submitMetrics(sender, conn)
	if err != nil {
		return err
	}
	sender.Commit()

	return nil
}

func (c *SystemdCheck) connect(sender aggregator.Sender) (*dbus.Conn, error) {
	conn, err := c.getDbusConnection()
	if err != nil {
		newErr := fmt.Errorf("cannot create a connection: %v", err)
		sender.ServiceCheck(canConnectServiceCheck, metrics.ServiceCheckCritical, "", nil, newErr.Error())
		return nil, newErr
	}
	sender.ServiceCheck(canConnectServiceCheck, metrics.ServiceCheckOK, "", nil, "")
	return conn, nil
}

func (c *SystemdCheck) submitSystemdState(sender aggregator.Sender, conn *dbus.Conn) {
	systemStateProp, err := c.stats.SystemState(conn)
	// Expected to fail for Systemd version <212
	// Source: https://github.com/systemd/systemd/blob/d4ffda38716d33dbc17faaa12034ccb77d0ed68b/NEWS#L7292-L7300
	// TODO: Instead of logging debug, replace with condition based on Systemd version.
	if err != nil {
		log.Debugf("err calling SystemState: %v", err)
	} else {
		serviceCheckStatus := metrics.ServiceCheckUnknown
		systemState, ok := systemStateProp.Value.Value().(string)
		if ok {
			status, ok := systemdStatusMapping[systemState]
			if ok {
				serviceCheckStatus = status
			}
		}
		sender.ServiceCheck(systemStateServiceCheck, serviceCheckStatus, "", nil, fmt.Sprintf("Systemd status is %v", systemStateProp.Value))
	}
}

func (c *SystemdCheck) getDbusConnection() (*dbus.Conn, error) {
	var err error
	var conn *dbus.Conn
	if c.config.instance.PrivateSocket != "" {
		conn, err = c.getPrivateSocketConnection(c.config.instance.PrivateSocket)
	} else {
		defaultPrivateSocket := "/run/systemd/private"
		if config.IsContainerized() {
			conn, err = c.getPrivateSocketConnection("/host" + defaultPrivateSocket)
		} else {
			conn, err = c.getSystemBusSocketConnection()
			if err != nil {
				conn, err = c.getPrivateSocketConnection(defaultPrivateSocket)
			}
		}
	}
	return conn, err
}

func (c *SystemdCheck) getPrivateSocketConnection(privateSocket string) (*dbus.Conn, error) {
	conn, err := c.stats.PrivateSocketConnection(privateSocket)
	if err != nil {
		log.Debugf("Error getting new connection using private socket %s: %v", privateSocket, err)
	}
	return conn, err
}

func (c *SystemdCheck) getSystemBusSocketConnection() (*dbus.Conn, error) {
	conn, err := c.stats.SystemBusSocketConnection()
	if err != nil {
		log.Debugf("Error getting new connection using system bus socket: %v", err)
	}
	return conn, err
}

func (c *SystemdCheck) submitVersion(conn *dbus.Conn) {
	version, err := c.stats.GetVersion(conn)
	if err != nil {
		log.Debugf("Error collecting version from the systemd: %v", err)
		return
	}
	checkID := string(c.ID())
	log.Debugf("Submit version %v for checkID %v", version, checkID)
	inventories.SetCheckMetadata(checkID, "version.raw", version)
}

func (c *SystemdCheck) submitMetrics(sender aggregator.Sender, conn *dbus.Conn) error {
	units, err := c.stats.ListUnits(conn)
	if err != nil {
		return fmt.Errorf("error getting list of units: %v", err)
	}

	c.submitCountMetrics(sender, units)

	loadedCount := 0
	monitoredCount := 0
	for _, unit := range units {
		if unit.LoadState == unitLoadedState {
			loadedCount++
		}
		if !c.isMonitored(unit.Name) {
			continue
		}
		monitoredCount++
		tags := []string{"unit:" + unit.Name}

		sender.ServiceCheck(unitStateServiceCheck, getServiceCheckStatus(unit.ActiveState, serviceCheckStateMapping), "", tags, "")

		if subStateMapping, found := c.config.instance.SubstateStatusMapping[unit.Name]; found {
			// User provided a custom mapping for this unit. Submit the systemd.unit.substate service check based on that
			if _, ok := subStateMapping[unit.SubState]; !ok {
				log.Debugf("The systemd unit %s has a substate value of %s that is not defined in the mapping set in the conf.yaml file. The service check will report 'UNKNOWN'", unit.Name, unit.SubState)
			}
			sender.ServiceCheck(unitSubStateServiceCheck, getServiceCheckStatus(unit.SubState, subStateMapping), "", tags, "")
		}

		c.submitBasicUnitMetrics(sender, conn, unit, tags)
		c.submitPropertyMetricsAsGauge(sender, conn, unit, tags)
	}

	sender.Gauge("systemd.units_total", float64(len(units)), "", nil)
	sender.Gauge("systemd.units_loaded_count", float64(loadedCount), "", nil)
	sender.Gauge("systemd.units_monitored_count", float64(monitoredCount), "", nil)
	return nil
}

func (c *SystemdCheck) submitBasicUnitMetrics(sender aggregator.Sender, conn *dbus.Conn, unit dbus.UnitStatus, tags []string) {
	active := 0
	if unit.ActiveState == unitActiveState {
		active = 1
	}
	loaded := 0
	if unit.LoadState == unitLoadedState {
		loaded = 1
	}
	sender.Gauge("systemd.unit.monitored", float64(1), "", tags)
	sender.Gauge("systemd.unit.active", float64(active), "", tags)
	sender.Gauge("systemd.unit.loaded", float64(loaded), "", tags)

	unitProperties, err := c.stats.GetUnitTypeProperties(conn, unit.Name, dbusTypeMap[typeUnit])
	if err != nil {
		log.Warnf("Error getting unit unitProperties: %s: %v", unit.Name, err)
		return
	}
	activeEnterTimestamp, err := getPropertyUint64(unitProperties, "ActiveEnterTimestamp")
	if err != nil {
		log.Warnf("Error getting property ActiveEnterTimestamp: %v", err)
		return
	}
	sender.Gauge("systemd.unit.uptime", float64(computeUptime(unit.ActiveState, activeEnterTimestamp, c.stats.UnixNow())), "", tags)
}

func (c *SystemdCheck) submitCountMetrics(sender aggregator.Sender, units []dbus.UnitStatus) {
	counts := map[string]int{}

	for _, activeState := range unitActiveStates {
		counts[activeState] = 0
	}

	for _, unit := range units {
		counts[unit.ActiveState]++
	}

	for _, activeState := range unitActiveStates {
		count := counts[activeState]
		sender.Gauge("systemd.units_by_state", float64(count), "", []string{"state:" + activeState})
	}
}

func (c *SystemdCheck) submitPropertyMetricsAsGauge(sender aggregator.Sender, conn *dbus.Conn, unit dbus.UnitStatus, tags []string) {
	for unitType := range metricConfigs {
		if !strings.HasSuffix(unit.Name, "."+unitType) {
			continue
		}
		serviceProperties, err := c.stats.GetUnitTypeProperties(conn, unit.Name, dbusTypeMap[unitType])
		if err != nil {
			log.Warnf("Error getting detailed properties for unit %s", unit.Name)
			return
		}
		for _, service := range metricConfigs[unitType] {
			err := sendServicePropertyAsGauge(sender, serviceProperties, service, tags)
			if err != nil {
				msg := fmt.Sprintf("Cannot send property '%s' for unit '%s': %v", service.propertyName, unit.Name, err)
				if service.optional {
					log.Debugf(msg)
				} else {
					log.Warnf(msg)
				}
			}
		}
	}
}

func sendServicePropertyAsGauge(sender aggregator.Sender, properties map[string]interface{}, service metricConfigItem, tags []string) error {
	if service.accountingProperty != "" {
		accounting, err := getPropertyBool(properties, service.accountingProperty)
		if err != nil {
			return err
		}
		if !accounting {
			log.Debugf("Skip sending metric due to disabled accounting. PropertyName=%s, AccountingProperty=%s, tags: %v", service.propertyName, service.accountingProperty, tags)
			return nil
		}
	}
	value, err := getPropertyUint64(properties, service.propertyName)
	if err != nil {
		return fmt.Errorf("error getting property %s: %v", service.propertyName, err)
	}
	sender.Gauge(service.metricName, float64(value), "", tags)
	return nil
}

// computeUptime returns uptime in microseconds
func computeUptime(activeState string, activeEnterTimestampMicroSec uint64, unitNow int64) int64 {
	if activeState != unitActiveState {
		return 0
	}
	uptime := unitNow - int64(activeEnterTimestampMicroSec)/1000000
	if uptime < 0 {
		return 0
	}
	return uptime
}

func getPropertyUint64(properties map[string]interface{}, propertyName string) (uint64, error) {
	prop, ok := properties[propertyName]
	if !ok {
		return 0, fmt.Errorf("property %s not found", propertyName)
	}
	switch typedProp := prop.(type) {
	case uint:
		return uint64(typedProp), nil
	case uint32:
		return uint64(typedProp), nil
	case uint64:
		return typedProp, nil
	}
	return 0, fmt.Errorf("property %s (%T) cannot be converted to uint64", propertyName, prop)
}

func getPropertyString(properties map[string]interface{}, propertyName string) (string, error) {
	prop, ok := properties[propertyName]
	if !ok {
		return "", fmt.Errorf("property %s not found", propertyName)
	}
	propValue, ok := prop.(string)
	if !ok {
		return "", fmt.Errorf("property %s (%T) cannot be converted to string", propertyName, prop)
	}
	return propValue, nil
}

func getPropertyBool(properties map[string]interface{}, propertyName string) (bool, error) {
	prop, ok := properties[propertyName]
	if !ok {
		return false, fmt.Errorf("property %s not found", propertyName)
	}
	propValue, ok := prop.(bool)
	if !ok {
		return false, fmt.Errorf("property %s (%T) cannot be converted to bool", propertyName, prop)
	}
	return propValue, nil
}

// getServiceCheckStatus returns a service check status for a given unit state (or substate) and a provided mapping
func getServiceCheckStatus(state string, mapping map[string]string) metrics.ServiceCheckStatus {
	switch mapping[state] {
	case "ok":
		return metrics.ServiceCheckOK
	case "warning":
		return metrics.ServiceCheckWarning
	case "critical":
		return metrics.ServiceCheckCritical
	}
	return metrics.ServiceCheckUnknown
}

// isMonitored verifies if a unit should be monitored.
func (c *SystemdCheck) isMonitored(unitName string) bool {
	for _, name := range c.config.instance.UnitNames {
		if name == unitName {
			return true
		}
	}
	return false
}

func isValidServiceCheckStatus(serviceCheckStatus string) bool {
	for _, validStatus := range validServiceCheckStatus {
		if serviceCheckStatus == validStatus {
			return true
		}
	}
	return false
}

// Configure configures the systemd checks
func (c *SystemdCheck) Configure(integrationConfigDigest uint64, rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	// Make sure check id is different for each different config
	// Must be called before CommonConfigure that uses checkID
	c.BuildID(integrationConfigDigest, rawInstance, rawInitConfig)

	err := c.CommonConfigure(integrationConfigDigest, rawInitConfig, rawInstance, source)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInitConfig, &c.config.initConf)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(rawInstance, &c.config.instance)
	if err != nil {
		return err
	}

	if len(c.config.instance.UnitNames) == 0 {
		return fmt.Errorf("instance config `unit_names` must not be empty")
	}

	for unitNameInMapping := range c.config.instance.SubstateStatusMapping {
		if !c.isMonitored(unitNameInMapping) {
			return fmt.Errorf("instance config specifies a custom substate mapping for unit '%s' but this unit is not monitored. Please add '%s' to 'unit_names'", unitNameInMapping, unitNameInMapping)
		}
	}

	for unitName, unitMapping := range c.config.instance.SubstateStatusMapping {
		for _, serviceCheckStatus := range unitMapping {
			if !isValidServiceCheckStatus(serviceCheckStatus) {
				return fmt.Errorf("Status '%s' for unit '%s' in 'substate_status_mapping' is invalid. It should be one of '%s'", serviceCheckStatus, unitName, strings.Join(validServiceCheckStatus, ", "))
			}
		}
	}

	return nil
}

func systemdFactory() check.Check {
	return &SystemdCheck{
		stats:     &defaultSystemdStats{},
		CheckBase: core.NewCheckBase(systemdCheckName),
	}
}

func init() {
	core.RegisterCheck(systemdCheckName, systemdFactory)
}
