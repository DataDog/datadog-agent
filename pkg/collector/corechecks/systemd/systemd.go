// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build systemd

package systemd

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/coreos/go-systemd/dbus"
	"gopkg.in/yaml.v2"

	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

const (
	systemdCheckName = "systemd"

	unitActiveState = "active"
	unitLoadedState = "loaded"

	typeUnit    = "unit"
	typeService = "service"
	typeSocket  = "socket"

	canConnectServiceCheck  = "systemd.can_connect"
	systemStateServiceCheck = "systemd.system.state"
	unitStateServiceCheck   = "systemd.unit.state"

	defaultMaxUnits = 50
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
			// only present from systemd v220
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
			metricName:         "systemd.service.task_count",
			propertyName:       "TasksCurrent",
			accountingProperty: "TasksAccounting",
		},
		{
			// only present from systemd v235
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
			metricName:   "systemd.socket.connection_refused_count",
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

// SystemdCheck aggregates metrics from one SystemdCheck instance
type SystemdCheck struct {
	core.CheckBase
	stats  systemdStats
	config systemdConfig
}

type systemdInstanceConfig struct {
	PrivateSocket     string   `yaml:"private_socket"`
	SystemBusSocket   string   `yaml:"system_bus_socket"`
	UnitNames         []string `yaml:"unit_names"`
	UnitRegexStrings  []string `yaml:"unit_regexes"`
	MaxUnits          int      `yaml:"max_units"`
	UnitRegexPatterns []*regexp.Regexp
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

func (s *defaultSystemdStats) UnixNow() int64 {
	return time.Now().Unix()
}

// Run executes the check
func (c *SystemdCheck) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	conn, err := c.connect(sender)
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

func (c *SystemdCheck) connect(sender aggregator.Sender) (*dbus.Conn, error) {
	conn, err := c.getDbusConnection()

	if err != nil {
		newErr := fmt.Errorf("cannot create a connection: %v", err)
		sender.ServiceCheck(canConnectServiceCheck, metrics.ServiceCheckCritical, "", nil, newErr.Error())
		return nil, newErr
	}

	systemStateProp, err := c.stats.SystemState(conn)
	if err != nil {
		newErr := fmt.Errorf("err calling SystemState: %v", err)
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

func (c *SystemdCheck) getDbusConnection() (*dbus.Conn, error) {
	var err error
	var conn *dbus.Conn
	if c.config.instance.PrivateSocket != "" {
		conn, err = c.getPrivateSocketConnection(c.config.instance.PrivateSocket)
	} else if c.config.instance.SystemBusSocket != "" {
		conn, err = c.getSystemBusSocketConnection(c.config.instance.SystemBusSocket)
	} else {
		defaultPrivateSocket := "/run/systemd/private"
		defaultSystemBusSocket := "/var/run/dbus/system_bus_socket"
		if config.IsContainerized() {
			defaultPrivateSocket = "/host" + defaultPrivateSocket
			defaultSystemBusSocket = "/host" + defaultSystemBusSocket
		}
		conn, err = c.getPrivateSocketConnection(defaultPrivateSocket)
		if err != nil {
			conn, err = c.getSystemBusSocketConnection(defaultSystemBusSocket)
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

func (c *SystemdCheck) getSystemBusSocketConnection(systemBusSocket string) (*dbus.Conn, error) {
	err := os.Setenv("DBUS_SYSTEM_BUS_ADDRESS", systemBusSocket)
	if err != nil {
		return nil, fmt.Errorf("error setting env: %v", err)
	}
	conn, err := c.stats.SystemBusSocketConnection()
	if err != nil {
		log.Debugf("Error getting new connection using system bus socket %s: %v", systemBusSocket, err)
	}
	return conn, err
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
		if monitoredCount > c.config.instance.MaxUnits {
			log.Warnf("Reporting more metrics than the allowed maximum. " +
				"Please contact support@datadoghq.com for more information.")
			continue
		}
		tags := []string{"unit:" + unit.Name}
		sender.ServiceCheck(unitStateServiceCheck, getServiceCheckStatus(unit.ActiveState), "", tags, "")

		c.submitBasicUnitMetrics(sender, conn, unit, tags)
		c.submitPropertyMetricsAsGauge(sender, conn, unit, tags)
	}

	sender.Gauge("systemd.unit.loaded.count", float64(loadedCount), "", nil)
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
	sender.Gauge("systemd.unit.active", float64(active), "", tags)
	sender.Gauge("systemd.unit.loaded", float64(loaded), "", tags)

	unitProperties, err := c.stats.GetUnitTypeProperties(conn, unit.Name, dbusTypeMap[typeUnit])
	if err != nil {
		log.Warnf("Error getting unit unitProperties: %s", unit.Name)
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
		sender.Gauge("systemd.unit.count", float64(count), "", []string{"active_state:" + activeState})
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
func (c *SystemdCheck) isMonitored(unitName string) bool {
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
func (c *SystemdCheck) Configure(rawInstance integration.Data, rawInitConfig integration.Data, source string) error {
	err := c.CommonConfigure(rawInstance, source)
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

	if c.config.instance.MaxUnits == 0 {
		c.config.instance.MaxUnits = defaultMaxUnits
	}

	for _, regexString := range c.config.instance.UnitRegexStrings {
		pattern, err := regexp.Compile(regexString)
		if err != nil {
			log.Warnf("Failed to parse systemd check option unit_regexes: %s", err)
			continue
		}
		c.config.instance.UnitRegexPatterns = append(c.config.instance.UnitRegexPatterns, pattern)
	}

	if len(c.config.instance.UnitNames) == 0 && len(c.config.instance.UnitRegexPatterns) == 0 {
		return fmt.Errorf("`unit_names` and `unit_regexes` must not be both empty")
	}

	if c.config.instance.PrivateSocket != "" && c.config.instance.SystemBusSocket != "" {
		return fmt.Errorf("`private_socket` and `system_bus_socket` should not be both provided")
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
