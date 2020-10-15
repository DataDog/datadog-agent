// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

const testNs uint64 = 1234

func TestIncomingTCPDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: network.NewPortMapping(NewDefaultConfig().ProcRoot, true, true),
	}

	tr.portMapping.AddMapping(1234, 8000)
	tr.portMapping.AddMapping(1234, 8080)
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 8000, 80, network.TCP, 1234)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.INCOMING, connStat.Direction)

	connStat = createConnectionStatWithNAT("10.2.25.1", "38.122.226.210", 8000, 80, network.TCP, 1234, 8080)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.INCOMING, connStat.Direction)
}

func TestOutgoingTCPDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: network.NewPortMapping(NewDefaultConfig().ProcRoot, true, true),
	}
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 8000, 80, network.TCP, 1234)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.OUTGOING, connStat.Direction)

	connStat = createConnectionStatWithNAT("10.2.25.1", "38.122.226.210", 8000, 80, network.TCP, 1234, 8080)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.OUTGOING, connStat.Direction)
}

func TestIncomingUDPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		udpPortMapping: network.NewPortMapping(NewDefaultConfig().ProcRoot, true, true),
	}
	tr.udpPortMapping.AddMapping(0, 5323)
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 5323, 8125, network.UDP, 1234)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.INCOMING, connStat.Direction)

	tr.udpPortMapping.AddMapping(0, 8080)
	connStat = createConnectionStatWithNAT("10.2.25.1", "38.122.226.210", 5323, 8125, network.UDP, 1234, 8080)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.INCOMING, connStat.Direction)
}

func TestOutgoingUDPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		udpPortMapping: network.NewPortMapping(NewDefaultConfig().ProcRoot, true, true),
	}
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 5323, 8125, network.UDP, 1234)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.OUTGOING, connStat.Direction)

	connStat = createConnectionStatWithNAT("10.2.25.1", "38.122.226.210", 5323, 8125, network.UDP, 1234, 8080)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, network.OUTGOING, connStat.Direction)
}

func createConnectionStat(source string, dest string, SPort uint16, DPort uint16, connType network.ConnectionType, netNs uint64) network.ConnectionStats {
	return createConnectionStatWithNAT(source, dest, SPort, DPort, connType, netNs, 0)
}

func createConnectionStatWithNAT(source string, dest string, SPort uint16, DPort uint16, connType network.ConnectionType, netNs uint64, natPort uint16) network.ConnectionStats {
	var iptrans *network.IPTranslation
	if natPort != 0 {
		iptrans = &network.IPTranslation{
			ReplSrcIP:   util.AddressFromString(dest),
			ReplDstIP:   util.AddressFromString(source),
			ReplSrcPort: DPort,
			ReplDstPort: natPort,
		}
	}
	return network.ConnectionStats{
		Source:        util.AddressFromString(source),
		Dest:          util.AddressFromString(dest),
		SPort:         SPort,
		DPort:         DPort,
		Type:          connType,
		NetNS:         uint32(netNs),
		IPTranslation: iptrans,
	}
}
