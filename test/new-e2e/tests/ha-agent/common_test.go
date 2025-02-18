
package haagent

import (
	_ "embed"
)

//go:embed fixtures/snmp.yaml
var snmpIntegration []byte
