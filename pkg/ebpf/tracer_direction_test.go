package ebpf

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
)

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
