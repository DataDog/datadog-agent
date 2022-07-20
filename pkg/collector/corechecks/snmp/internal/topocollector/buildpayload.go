package topocollector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/topopayload"
)

func buildPayload(remoteConns []common.LldpRemote, hostname string, address string, name string) topopayload.TopologyPayload {
	p := topopayload.TopologyPayload{
		Host: hostname,
		Device: topopayload.Device{
			IP:   address, // TODO: Update with real data
			Name: name,    // TODO: Update with real data
		},
	}
	for _, remoteCon := range remoteConns {
		var remManAddr string
		if remoteCon.RemoteManagement != nil {
			remManAddr = remoteCon.RemoteManagement.ManAddr
		}
		p.Connections = append(p.Connections, topopayload.Connection{
			Remote: topopayload.Endpoint{
				Device: topopayload.Device{
					IP:                    remManAddr,
					Name:                  remoteCon.SysName,
					Description:           remoteCon.SysDesc,
					ChassisID:             remoteCon.ChassisID,
					ChassisIDType:         common.ChassisIDSubtypeMap[remoteCon.ChassisIDSubtype],
					CapabilitiesSupported: remoteCon.SysCapSupported,
					CapabilitiesEnabled:   remoteCon.SysCapEnabled,
				},
				Interface: topopayload.Interface{
					// TODO: Check if type if valid/present
					IDType:      common.PortIDSubTypeMap[remoteCon.PortIDSubType],
					ID:          remoteCon.PortID,
					Description: remoteCon.PortDesc,
				},
			},
			Local: topopayload.Endpoint{
				// TODO: is it ok to have device field, but never filled for local endpoint?
				Interface: topopayload.Interface{
					IDType:      common.PortIDSubTypeMap[remoteCon.LocalPort.PortIDSubType],
					ID:          remoteCon.LocalPort.PortID,
					Description: remoteCon.LocalPort.PortDesc,
				},
			},
		})
	}

	return p
}
