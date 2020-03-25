package status

import (
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
)

func getSystemProbeStatus() (map[string]interface{}) {

	processnet.GetRemoteSystemProbeUtil()
	systemProbeDetails := make(map[string]interface{})

	return systemProbeDetails
}


