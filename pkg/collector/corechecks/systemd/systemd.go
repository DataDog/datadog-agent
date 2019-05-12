// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build systemd

package systemd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/coreos/go-systemd/dbus"
	"gopkg.in/yaml.v2"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	systemdCheckName = "systemd"

	unitTag            = "unit"
	unitActiveStateTag = "active_state"

	unitActiveState = "active"
	unitLoadedState = "loaded"

	typeUnit    = "unit"
	typeService = "service"
	typeSocket  = "socket"

	canConnectServiceCheck  = "systemd.can_connect"
	systemStateServiceCheck = "systemd.system.state"
	unitStateServiceCheck   = "systemd.unit.state"
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
var metricConfigs = map[string][]metricConfigItem{
	typeService: {
		{
			metricName:         "systemd.service.cpu_usage_n_sec",
			propertyName:       "CPUUsageNSec",
			accountingProperty: "CPUAccounting",
		},
		{
			metricName:         "systemd.service.memory_current",
			propertyName:       "MemoryCurrent",
			accountingProperty: "MemoryAccounting",
		},
		{
			metricName:         "systemd.service.tasks_current",
			propertyName:       "TasksCurrent",
			accountingProperty: "TasksAccounting",
		},
		{
			// only present from systemd v235
			metricName:   "systemd.service.n_restarts",
			propertyName: "NRestarts",
			optional:     true,
		},
	},
	typeSocket: {
		{
			metricName:   "systemd.socket.n_accepted",
			propertyName: "NAccepted",
		},
		{
			metricName:   "systemd.socket.n_connections",
			propertyName: "NConnections",
		},
		{
			// only present from systemd v239
			metricName:   "systemd.socket.n_refused",
			propertyName: "NRefused",
			optional:     true,
		},
	},
}

var unitActiveStates = []string{"active", "activating", "inactive", "deactivating", "failed"}

var systemdStatusMapping = map[string]metrics.ServiceCheckStatus{
	"initializing": metrics.ServiceCheckUnknown,
	"starting":     metrics.ServiceCheckUnknown,
	"running":      metrics.ServiceCheckOK,
	"degraded":     metrics.ServiceCheckCritical,
	"maintenance":  metrics.ServiceCheckCritical,
	"stopping":     metrics.ServiceCheckCritical,
}

// Check aggregates metrics from one Check instance
type Check struct {
	core.CheckBase
	stats  systemdStats
	config systemdConfig
}

type systemdInstanceConfig struct {
	UnitNames         []string `yaml:"unit_names"`
	UnitRegexStrings  []string `yaml:"unit_regex"`
	UnitRegexPatterns []*regexp.Regexp
}

type systemdInitConfig struct{}

type systemdConfig struct {
	instance systemdInstanceConfig
	initConf systemdInitConfig
}

type systemdStats interface {
	// Dbus Connection
	NewConn() (*dbus.Conn, error)
	CloseConn(c *dbus.Conn)

	// System Data
	SystemState(c *dbus.Conn) (*dbus.Property, error)
	ListUnits(c *dbus.Conn) ([]dbus.UnitStatus, error)
	GetUnitTypeProperties(c *dbus.Conn, unitName string, unitType string) (map[string]interface{}, error)

	// Misc
	TimeNanoNow() int64
}

type defaultSystemdStats struct{}

func (s *defaultSystemdStats) NewConn() (*dbus.Conn, error) {
	return dbus.New()
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

func (s *defaultSystemdStats) TimeNanoNow() int64 {
	return time.Now().UnixNano()
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	conn, err := c.getDbusConn(sender)
	if err != nil {
		return err
	}
	defer c.stats.CloseConn(conn)

	err = c.submitMetrics(sender, conn)
	if err != nil {
		return err
	}
	sender.Commit()
	return nil
}

func (c *Check) getDbusConn(sender aggregator.Sender) (*dbus.Conn, error) {
	conn, err := c.stats.NewConn()
	if err != nil {
		newErr := fmt.Errorf("Cannot create a connection: %v", err)
		sender.ServiceCheck(canConnectServiceCheck, metrics.ServiceCheckCritical, "", nil, newErr.Error())
		return nil, newErr
	}

	systemStateProp, err := c.stats.SystemState(conn)
	if err != nil {
		newErr := fmt.Errorf("Err calling SystemState: %v", err)
		sender.ServiceCheck(canConnectServiceCheck, metrics.ServiceCheckCritical, "", nil, newErr.Error())
		return nil, newErr
	}
	sender.ServiceCheck(canConnectServiceCheck, metrics.ServiceCheckOK, "", nil, "")

	serviceCheckStatus := metrics.ServiceCheckUnknown
	systemState, ok := systemStateProp.Value.Value().(string)
	if ok {
		status, ok := systemdStatusMapping[systemState]
		if ok {
			serviceCheckStatus = status
		}
	}
	sender.ServiceCheck(systemStateServiceCheck, serviceCheckStatus, "", nil, fmt.Sprintf("Systemd status is %v", systemStateProp.Value))
	return conn, nil
}

func (c *Check) submitMetrics(sender aggregator.Sender, conn *dbus.Conn) error {
	units, err := c.stats.ListUnits(conn)
	if err != nil {
		return fmt.Errorf("Error getting list of units: %v", err)
	}

	c.submitCountMetrics(sender, units)

	loadedCount := 0
	for _, unit := range units {
		if unit.LoadState == unitLoadedState {
			loadedCount++
		}
		if !c.isMonitored(unit.Name) {
			continue
		}
		tags := []string{unitTag + ":" + unit.Name}
		sender.ServiceCheck(unitStateServiceCheck, getServiceCheckStatus(unit.ActiveState), "", tags, "")

		c.submitBasicUnitMetrics(sender, conn, unit, tags)
		c.submitPropertyMetricsAsGauge(sender, conn, unit, tags)
	}

	sender.Gauge("systemd.unit.loaded.count", float64(loadedCount), "", nil)
	return nil
}

func (c *Check) submitBasicUnitMetrics(sender aggregator.Sender, conn *dbus.Conn, unit dbus.UnitStatus, tags []string) {
	unitProperties, err := c.stats.GetUnitTypeProperties(conn, unit.Name, dbusTypeMap[typeUnit])
	if err != nil {
		log.Errorf("Error getting unit unitProperties: %s", unit.Name)
		return
	}

	ActiveEnterTimestamp, err := getPropertyUint64(unitProperties, "ActiveEnterTimestamp")
	if err != nil {
		log.Errorf("Error getting property ActiveEnterTimestamp: %v", err)
		return
	}
	active := 0
	if unit.ActiveState == unitActiveState {
		active = 1
	}
	loaded := 0
	if unit.LoadState == unitLoadedState {
		loaded = 1
	}
	sender.Gauge("systemd.unit.active", float64(active), "", tags)
	sender.Gauge("systemd.unit.loaded", float64(loaded), "", tags)
	sender.Gauge("systemd.unit.uptime", float64(computeUptime(unit.ActiveState, ActiveEnterTimestamp, c.stats.TimeNanoNow())), "", tags)
}

func (c *Check) submitCountMetrics(sender aggregator.Sender, units []dbus.UnitStatus) {
	counts := map[string]int{}

	for _, activeState := range unitActiveStates {
		counts[activeState] = 0
	}

	for _, unit := range units {
		counts[unit.ActiveState]++
	}

	for _, activeState := range unitActiveStates {
		count := counts[activeState]
		sender.Gauge("systemd.unit.count", float64(count), "", []string{unitActiveStateTag + ":" + activeState})
	}
}

func (c *Check) submitPropertyMetricsAsGauge(sender aggregator.Sender, conn *dbus.Conn, unit dbus.UnitStatus, tags []string) {
	for unitType := range metricConfigs {
		if !strings.HasSuffix(unit.Name, "."+unitType) {
			continue
		}
		serviceProperties, err := c.stats.GetUnitTypeProperties(conn, unit.Name, dbusTypeMap[unitType])
		if err != nil {
			log.Errorf("Error getting serviceProperties for service: %s", unit.Name)
			return
		}
		for _, service := range metricConfigs[unitType] {
			err := sendServicePropertyAsGauge(sender, serviceProperties, service, tags)
			if err != nil {
				msg := fmt.Sprintf("Cannot send property '%s' for unit '%s': %v", service.propertyName, unit.Name, err)
				if service.optional {
					log.Debugf(msg)
				} else {
					log.Errorf(msg)
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
		return fmt.Errorf("Error getting property %s: %v", service.propertyName, err)
	}
	sender.Gauge(service.metricName, float64(value), "", tags)
	return nil
}

// computeUptime returns uptime in microseconds
func computeUptime(activeState string, activeEnterTimestampMicroSec uint64, nanoNow int64) int64 {
	if activeState != unitActiveState {
		return 0
	}
	uptime := nanoNow/1000 - int64(activeEnterTimestampMicroSec)
	if uptime < 0 {
		return 0
	}
	return uptime
}

func getPropertyUint64(properties map[string]interface{}, propertyName string) (uint64, error) {
	prop, ok := properties[propertyName]
	if !ok {
		return 0, fmt.Errorf("Property %s not found", propertyName)
	}
	switch typedProp := prop.(type) {
	case uint:
		return uint64(typedProp), nil
	case uint32:
		return uint64(typedProp), nil
	case uint64:
		return typedProp, nil
	}
	return 0, fmt.Errorf("Property %s (%T) cannot be converted to uint64", propertyName, prop)
}

func getPropertyString(properties map[string]interface{}, propertyName string) (string, error) {
	prop, ok := properties[propertyName]
	if !ok {
		return "", fmt.Errorf("Property %s not found", propertyName)
	}
	propValue, ok := prop.(string)
	if !ok {
		return "", fmt.Errorf("Property %s (%T) cannot be converted to string", propertyName, prop)
	}
	return propValue, nil
}

func getPropertyBool(properties map[string]interface{}, propertyName string) (bool, error) {
	prop, ok := properties[propertyName]
	if !ok {
		return false, fmt.Errorf("Property %s not found", propertyName)
	}
	propValue, ok := prop.(bool)
	if !ok {
		return false, fmt.Errorf("Property %s (%T) cannot be converted to bool", propertyName, prop)
	}
	return propValue, nil
}

func getServiceCheckStatus(activeState string) metrics.ServiceCheckStatus {
	switch activeState {
	case "active":
		return metrics.ServiceCheckOK
	case "inactive", "failed":
		return metrics.ServiceCheckCritical
	case "activating", "deactivating":
		return metrics.ServiceCheckUnknown
	}
	return metrics.ServiceCheckUnknown
}

// isMonitored verifies if a unit should be monitored.
func (c *Check) isMonitored(unitName string) bool {
	if len(c.config.instance.UnitNames) == 0 && len(c.config.instance.UnitRegexPatterns) == 0 {
		return true
	}
	for _, name := range c.config.instance.UnitNames {
		if name == unitName {
			return true
		}
	}
	for _, pattern := range c.config.instance.UnitRegexPatterns {
		if pattern.MatchString(unitName) {
			return true
		}
	}
	return false
}

// Configure configures the systemd checks
func (c *Check) Configure(rawInstance integration.Data, rawInitConfig integration.Data) error {
	err := c.CommonConfigure(rawInstance)
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

	for _, regexString := range c.config.instance.UnitRegexStrings {
		pattern, err := regexp.Compile(regexString)
		if err != nil {
			log.Errorf("Failed to parse systemd check option unit_regex: %s", err)
			continue
		}
		c.config.instance.UnitRegexPatterns = append(c.config.instance.UnitRegexPatterns, pattern)
	}
	return nil
}

func systemdFactory() check.Check {
	return &Check{
		stats:     &defaultSystemdStats{},
		CheckBase: core.NewCheckBase(systemdCheckName),
	}
}

func init() {
	core.RegisterCheck(systemdCheckName, systemdFactory)
}
