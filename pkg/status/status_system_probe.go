package status

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/process/net"
)

func getSystemProbeStats() map[string]interface{} {

	// TODO: Pull system-probe path from system-probe.yaml
	net.SetSystemProbePath("/opt/datadog-agent/run/sysprobe.sock")
	probeUtil, err := net.GetRemoteSystemProbeUtil()

	if err != nil {
		return map[string]interface{}{
			"Errors": fmt.Sprintf("%v", err),
		}
	}

	systemProbeDetails, err := probeUtil.GetStats()
	if err != nil {
		return map[string]interface{}{
			"Errors": fmt.Sprintf("issue querying stats from system probe: %v", err),
		}
	}

	return systemProbeDetails
}
