package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/snmp/traps"
)

// Start starts the SNMP listeners
func Start() {
	traps.StartTrapsListeners()
}
