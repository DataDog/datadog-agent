package v5

import (
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/gohai"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/resources"
)

// GetPayload returns the complete metadata payload as seen in Agent v5
func GetPayload(hostname string) *Payload {
	cp := common.GetPayload()
	hp := host.GetPayload(hostname)
	rp := resources.GetPayload(hostname)
	gp := gohai.GetPayload()
	return &Payload{
		CommonPayload:    CommonPayload{*cp},
		HostPayload:      HostPayload{*hp},
		ResourcesPayload: ResourcesPayload{*rp},
		GohaiPayload:     GohaiPayload{*gp},
	}
}
