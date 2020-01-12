package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

func TestIncomingTCPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}

	tr.portMapping.AddMapping(8000)
	connStats := ConnectionStats{
		Source: util.AddressFromString("10.2.25"),
		Dest:   util.AddressFromString("38.122.226.210"),
		SPort:  8000,
		DPort:  5893,
		Type:   TCP,
	}

	conn := []ConnectionStats{connStats}
	tr.setConnectionDirections(conn)
	assert.Equal(t, INCOMING, conn[0].Direction)
}

func TestOutgoingTCPConnectionDirection(t *testing.T) {
	tr, err := NewTracer(NewDefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	connStats := ConnectionStats{
		Source: util.AddressFromString("10.2.25"),
		Dest:   util.AddressFromString("38.122.226.210"),
		SPort:  8000,
		DPort:  80,
		Type:   TCP,
	}

	conn := []ConnectionStats{connStats}
	tr.setConnectionDirections(conn)
	assert.Equal(t, OUTGOING, conn[0].Direction)
}

func TestRemoteUDPConnectionDirection(t *testing.T) {
	tr, err := NewTracer(NewDefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	connStats := ConnectionStats{
		Source: util.AddressFromString("10.2.25"),
		Dest:   util.AddressFromString("38.122.226.210"),
		SPort:  5323,
		DPort:  8125,
		Type:   UDP,
	}

	conn := []ConnectionStats{connStats}
	tr.setConnectionDirections(conn)
	assert.Equal(t, NONE, conn[0].Direction)
}

func TestLocalNATConnectionDirection(t *testing.T) {
	tr, err := NewTracer(NewDefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Stop()

	connStatNAT := ConnectionStats{
		Source: util.AddressFromString("10.2.25"),
		Dest:   util.AddressFromString("2.2.2.2"),
		SPort:  59782,
		DPort:  8000,
		Type:   TCP,
		IPTranslation: &netlink.IPTranslation{
			ReplSrcIP:   util.AddressFromString("1.1.1.1"),
			ReplDstIP:   util.AddressFromString("10.0.2.25"),
			ReplSrcPort: 8000,
			ReplDstPort: 59782,
		},
	}

	connStat := ConnectionStats{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("10.0.2.25"),
		SPort:  8000,
		DPort:  59782,
		Type:   TCP,
	}

	conns := []ConnectionStats{connStatNAT, connStat}
	tr.setConnectionDirections(conns)
	assert.Equal(t, LOCAL, conns[0].Direction)
	assert.Equal(t, LOCAL, conns[1].Direction)
}
