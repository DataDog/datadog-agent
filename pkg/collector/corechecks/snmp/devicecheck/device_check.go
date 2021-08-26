package devicecheck

import "github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/checkconfig"

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
}
