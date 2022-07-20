package topocollector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/topopayload"
)

func buildPayload(remoteConns []common.LldpRemote, hostname string) topopayload.TopologyPayload {
	p := topopayload.TopologyPayload{
		Host: hostname,
		Device: topopayload.Device{
			IP:   "127.0.0.1", // TODO: Update with real data
			Name: "my-device", // TODO: Update with real data
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
					ChassisId:             remoteCon.ChassisId,
					ChassisIdType:         common.ChassisIdSubtypeMap[remoteCon.ChassisIdSubtype],
					CapabilitiesSupported: remoteCon.SysCapSupported,
					CapabilitiesEnabled:   remoteCon.SysCapEnabled,
				},
				Interface: topopayload.Interface{
					// TODO: Check if type if valid/present
					IdType:      common.PortIdSubTypeMap[remoteCon.PortIdSubType],
					Id:          remoteCon.PortId,
					Description: remoteCon.PortDesc,
				},
			},
			Local: topopayload.Endpoint{
				// TODO: is it ok to have device field, but never filled for local endpoint?
				Interface: topopayload.Interface{
					IdType:      common.PortIdSubTypeMap[remoteCon.LocalPort.PortIdSubType],
					Id:          remoteCon.LocalPort.PortId,
					Description: remoteCon.LocalPort.PortDesc,
				},
			},
		})
	}

	return p
}
