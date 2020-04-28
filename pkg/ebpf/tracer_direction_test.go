// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIncomingTCPDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}

	tr.portMapping.AddMapping(8000)
	connStat := CreateConnectionStat("10.2.25.1", "38.122.226.210", 8000, 80, TCP)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, INCOMING, connStat.Direction)
}

func TestOutgoingTCPDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	connStat := CreateConnectionStat("10.2.25.1", "38.122.226.210", 8000, 80, TCP)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, OUTGOING, connStat.Direction)
}

func TestIncomingUDPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		udpPortMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	tr.udpPortMapping.AddMapping(5323)
	connStat := CreateConnectionStat("10.2.25.1", "38.122.226.210", 5323, 8125, UDP)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, INCOMING, connStat.Direction)
}

func TestOutgoingUDPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		udpPortMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	connStat := CreateConnectionStat("10.2.25.1", "38.122.226.210", 5323, 8125, UDP)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, OUTGOING, connStat.Direction)
}
