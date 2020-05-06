package traps

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StartTrapsListeners starts the SNMP traps listeners.
func StartTrapsListeners() {
	log.Info("SNMP traps listeners started")
}
