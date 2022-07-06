package discoverycollector

import (
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/common"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/payload"
)

func buildPayload(remoteConns []common.LldpRemote, hostname string) payload.TopologyPayload {
	p := payload.TopologyPayload{
		Host: hostname,
	}
	for _, remoteCon := range remoteConns {
		var remManAddr string
		if remoteCon.RemoteManagement != nil {
			remManAddr = remoteCon.RemoteManagement.ManAddr
		}
		p.Connections = append(p.Connections, payload.Connection{
			Remote: payload.Endpoint{
				Device: payload.Device{
					IP:                    remManAddr,
					Name:                  remoteCon.SysName,
					Description:           remoteCon.SysDesc,
					ChassisId:             remoteCon.ChassisId,
					ChassisIdType:         common.ChassisIdSubtypeMap[remoteCon.ChassisIdSubtype],
					CapabilitiesSupported: remoteCon.SysCapSupported,
					CapabilitiesEnabled:   remoteCon.SysCapEnabled,
				},
				Interface: payload.Interface{
					// TODO: Check if type if valid/present
					IdType:      common.PortIdSubTypeMap[remoteCon.PortIdSubType],
					Id:          remoteCon.PortId,
					Description: remoteCon.PortDesc,
				},
			},
			Local: payload.Endpoint{
				// TODO: is it ok to have device field, but never filled for local endpoint?
				Interface: payload.Interface{
					IdType:      common.PortIdSubTypeMap[remoteCon.LocalPort.PortIdSubType],
					Id:          remoteCon.LocalPort.PortId,
					Description: remoteCon.LocalPort.PortDesc,
				},
			},
		})
	}

	return p
}
