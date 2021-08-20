package snmp

// deviceCheck hold info necessary to collect info for a single device
type deviceCheck struct {
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
	deviceID     string   // TODO: move to deviceCheck
	deviceIDTags []string // TODO: move to deviceCheck

	// state (can be changed with profile refresh)
	autodetectProfile bool
	oidConfig         oidConfig
	metrics           []metricsConfig
	metricTags        []metricTagConfig
	profileTags       []string
	profile           string
	profileDef        *profileDefinition
}

func newDeviceCheck(config snmpConfig, ipAddress string) *deviceCheck {
	return &deviceCheck{}
}
