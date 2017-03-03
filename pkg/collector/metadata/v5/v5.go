package v5

import (
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/host"
)

// GetPayload returns the complete metadata payload as seen in Agent v5
func GetPayload(hostname string) *Payload {
	cp := common.GetPayload()
	hp := host.GetPayload(hostname)
	return &Payload{
		CommonPayload: CommonPayload{*cp},
		HostPayload:   HostPayload{*hp},
	}
}
