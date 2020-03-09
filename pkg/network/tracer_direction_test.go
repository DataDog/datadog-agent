// +build linux_bpf

package network

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

func TestUDPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	connStat := CreateConnectionStat("10.2.25.1", "38.122.226.210", 5323, 8125, UDP)
	connStat.Direction = tr.determineConnectionDirection(&connStat)
	assert.Equal(t, NONE, connStat.Direction)
}
