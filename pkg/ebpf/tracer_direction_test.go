package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

/**
var tests = []struct {
	name         string
	tr           *Tracer
	conns 		 []ConnectionStats
	portMappings []uint16
	expected     []ConnectionDirection
}{
	{
		"TestIncomingTCPConnectionDirection",
		&Tracer{portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig())},
		[]ConnectionStats{
			ConnectionStats{
				Source: util.AddressFromString("10.2.25.1"),
				Dest:   util.AddressFromString("38.122.226.210"),
				SPort:  8000,
				DPort:  5893,
				Type:   TCP,
			},
		},
		[]uint16{8000},
		[]ConnectionDirection{OUTGOING},
	},
	{
		"TestOutgoingTCPConnectionDirection",
		&Tracer{portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig())},
		[]ConnectionStats{
			ConnectionStats{
				Source: util.AddressFromString("10.2.25.1"),
				Dest:   util.AddressFromString("38.122.226.210"),
				SPort:  8000,
				DPort:  80,
				Type:   TCP,
			},
		},
		nil,
		[]ConnectionDirection{OUTGOING},
	},
	{
		"TestRemoteUDPConnectionDirection",
		&Tracer{portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig())},
		[]ConnectionStats{
			ConnectionStats{
				Source: util.AddressFromString("10.2.25.1"),
				Dest:   util.AddressFromString("38.122.226.210"),
				SPort:  5323,
				DPort:  8125,
				Type:   UDP,
			},
		},
		nil,
		[]ConnectionDirection{NONE},
	},
	{
		"TestLocalDNATConnectionDirection",
		&Tracer{portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig())},
		[]ConnectionStats{
			ConnectionStats{
				Source: util.AddressFromString("10.2.25.1"),
				Dest:   util.AddressFromString("2.2.2.2"),
				SPort:  59782,
				DPort:  8000,
				Type:   TCP,
				IPTranslation: &netlink.IPTranslation{
					ReplSrcIP:   util.AddressFromString("1.1.1.1"),
					ReplDstIP:   util.AddressFromString("10.2.25.1"),
					ReplSrcPort: 8000,
					ReplDstPort: 59782,
				},
			},
			{
				Source: util.AddressFromString("1.1.1.1"),
				Dest:   util.AddressFromString("10.0.2.25"),
				SPort:  8000,
				DPort:  59782,
				Type:   TCP,
			},
		},
		nil,
		[]ConnectionDirection{LOCAL,LOCAL},
	},
}

func TestDirections (t *testing.T) {
	for _, test := range tests{
		if test.portMappings != nil {
			for _, portMapping := range test.portMappings {
				test.tr.portMapping.AddMapping(portMapping)
			}
		}
		test.tr.setConnectionDirections(test.conns)
		for i , _ := range test.conns {
			t.Errorf("%s Expected %d got %d", test.name, test.expected, test.conns[i].Direction)
			// assert.Equal(t, test.expected[index], conn.Direction)
		}
	}
}
**/

func TestIncomingTCPDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}

	tr.portMapping.AddMapping(8000)
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 8000, 80, TCP)
	conn := []ConnectionStats{connStat}
	tr.setConnectionDirections(conn)
	assert.Equal(t, INCOMING, conn[0].Direction)
}

func TestOutgoingTCPDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 8000, 80, TCP)
	conn := []ConnectionStats{connStat}
	tr.setConnectionDirections(conn)
	assert.Equal(t, OUTGOING, conn[0].Direction)
}

func TestUDPConnectionDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	connStat := createConnectionStat("10.2.25.1", "38.122.226.210", 5323, 8125, UDP)
	conn := []ConnectionStats{connStat}
	tr.setConnectionDirections(conn)
	assert.Equal(t, NONE, conn[0].Direction)
}

func TestLocalDNATDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	DNatConn := createConnectionStat("10.0.25.1", "2.2.2.2", 59782, 8000, TCP)
	addIPTranslationToConnection(&DNatConn, "1.1.1.1", "10.0.25.1", 8000, 59782)
	localConn := createConnectionStat("1.1.1.1", "10.0.25.1", 8000, 59782, TCP)
	conns := []ConnectionStats{DNatConn, localConn}
	tr.setConnectionDirections(conns)
	assert.Equal(t, LOCAL, conns[0].Direction)
	assert.Equal(t, LOCAL, conns[1].Direction)
}
func TestLocalSNATDirection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	SNatConn := createConnectionStat("2.2.2.2", "10.2.0.25", 59782, 8000, TCP)
	addIPTranslationToConnection(&SNatConn, "10.2.0.25", "2.2.2.2", 8000, 59782)
	localConn := createConnectionStat("10.2.0.25", "2.2.2.2", 8000, 59782, TCP)
	conns := []ConnectionStats{SNatConn, localConn}
	tr.setConnectionDirections(conns)
	assert.Equal(t, LOCAL, conns[0].Direction)
	assert.Equal(t, LOCAL, conns[1].Direction)
}

func TestIncomingDNATConnection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	tr.portMapping.AddMapping(59782)
	DNatConn := createConnectionStat("2.2.2.2", "179.226.25.40", 59782, 8000, TCP)
	addIPTranslationToConnection(&DNatConn, "179.226.25.40", "2.2.2.2", 8000, 59782)
	conns := []ConnectionStats{DNatConn}
	tr.setConnectionDirections(conns)
	assert.Equal(t, INCOMING, conns[0].Direction)
}

func TestOutgoingSNATConnection(t *testing.T) {
	tr := &Tracer{
		portMapping: NewPortMapping(NewDefaultConfig().ProcRoot, NewDefaultConfig()),
	}
	tr.portMapping.AddMapping(59782)
	SNatConn := createConnectionStat("2.2.2.2", "10.2.0.25", 59782, 8000, TCP)
	addIPTranslationToConnection(&SNatConn, "10.2.0.25", "2.2.2.2", 8000, 59782)
	conns := []ConnectionStats{SNatConn}
	tr.setConnectionDirections(conns)
	assert.Equal(t, INCOMING, conns[0].Direction)
}

func createConnectionStat(source string, dest string, SPort uint16, DPort uint16, connType ConnectionType) ConnectionStats {
	return ConnectionStats{
		Source: util.AddressFromString(source),
		Dest:   util.AddressFromString(dest),
		SPort:  SPort,
		DPort:  DPort,
		Type:   connType,
	}
}

func addIPTranslationToConnection(conn *ConnectionStats, ReplSrcIP string, ReplDstIP string, ReplSrcPort uint16, ReplDstPort uint16) {
	translation := netlink.IPTranslation{
		ReplSrcIP:   util.AddressFromString(ReplSrcIP),
		ReplDstIP:   util.AddressFromString(ReplDstIP),
		ReplSrcPort: ReplSrcPort,
		ReplDstPort: ReplDstPort,
	}
	conn.IPTranslation = &translation
}
