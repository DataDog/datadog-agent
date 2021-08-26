package devicecheck

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/report"
)

// DeviceCheck hold info necessary to collect info for a single device
type DeviceCheck struct {
	// static configs
	ipAddress       string
	port            uint16
	communityString string
	snmpVersion     string
	timeout         int
	retries         int
	user            string
	authProtocol    string
	authKey         string
	privProtocol    string
	privKey         string
	contextName     string

	// initialized once
	deviceID     string
	deviceIDTags []string

	// state (can be changed with profile refresh)
	autodetectProfile bool
	oidConfig         checkconfig.OidConfig
	metrics           []checkconfig.MetricsConfig
	metricTags        []checkconfig.MetricTagConfig
	profileTags       []string
	profile           string

	sender *report.MetricSender
}

// SetSender sets the current sender
func (d *DeviceCheck) SetSender(sender *report.MetricSender) {
	d.sender = sender
}

// NewDeviceCheck returns a new DeviceCheck
func NewDeviceCheck(config checkconfig.CheckConfig, ipAddress string) *DeviceCheck {
	return &DeviceCheck{
		ipAddress:       ipAddress,
		port:            config.Port,
		communityString: config.CommunityString,
		snmpVersion:     config.SnmpVersion,
		timeout:         config.Timeout,
		retries:         config.Retries,
		user:            config.User,
		authProtocol:    config.AuthProtocol,
		authKey:         config.AuthKey,
		privProtocol:    config.PrivProtocol,
		privKey:         config.PrivKey,
		contextName:     config.ContextName,
		//deviceID: config.DeviceID,
		//deviceIDTags: config.DeviceIDTags,
		//autodetectProfile: config.,
		//oidConfig: config.,
		//metrics: config.,
		//metricTags: config.,
		//profileTags: config.,
		//profile: config.,
	}
}
