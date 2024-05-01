package failed

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FailedConnStats is a wrapper to help document the purpose of the underlying map
type FailedConnStats struct {
	CountByErrCode map[uint32]uint32
}

// String returns a string representation of the failedConnStats
func (t FailedConnStats) String() string {
	return fmt.Sprintf(
		"failedConnStats{countByErrCode: %+v}", t.CountByErrCode,
	)
}

// FailedConnMap is a map of connection tuples to failed connection stats
type FailedConnMap map[ebpf.ConnTuple]*FailedConnStats

// FailedConns is a struct to hold failed connections
type FailedConns struct {
	FailedConnMap map[ebpf.ConnTuple]*FailedConnStats
	sync.RWMutex
}

// NewFailedConns returns a new FailedConns struct
func NewFailedConns() *FailedConns {
	return &FailedConns{
		FailedConnMap: make(map[ebpf.ConnTuple]*FailedConnStats),
	}
}

// MatchFailedConn increments the failed connection counters for a given connection
func MatchFailedConn(conn *network.ConnectionStats, failedConnMap *FailedConns) {
	connTuple := connStatsToTuple(conn)
	failedConnMap.RLock()
	defer failedConnMap.RUnlock()
	if failedConn, ok := failedConnMap.FailedConnMap[connTuple]; ok {
		conn.TCPFailures = make(map[network.TCPFailure]uint32)
		for errCode, count := range failedConn.CountByErrCode {
			switch errCode {
			case 104:
				log.Infof("Incrementing TCP Failed Reset counter for connection: %+v", conn)
				conn.TCPFailures[network.TCPFailureConnectionReset] += count
			case 110:
				log.Infof("Incrementing TCP Failed Timeout counter for connection: %+v", conn)
				conn.TCPFailures[network.TCPFailureConnectionTimeout] += count
			case 111:
				log.Infof("Incrementing TCP Failed Refused counter for connection: %+v", conn)
				conn.TCPFailures[network.TCPFailureConnectionRefused] += count
			default:
				log.Infof("Incrementing TCP Failed Default counter for connection: %+v", conn)
				conn.TCPFailures[network.TCPFailureUnknown] += count
			}
		}
	}
}

func connStatsToTuple(c *network.ConnectionStats) ebpf.ConnTuple {
	var tup ebpf.ConnTuple
	tup.Sport = c.SPort
	tup.Dport = c.DPort
	tup.Netns = c.NetNS
	tup.Pid = c.Pid
	if c.Family == network.AFINET {
		tup.SetFamily(ebpf.IPv4)
	} else {
		tup.SetFamily(ebpf.IPv6)
	}
	if c.Type == network.TCP {
		tup.SetType(ebpf.TCP)
	} else {
		tup.SetType(ebpf.UDP)
	}
	if c.Source.IsZero() {
		tup.Saddr_l, tup.Saddr_h = 0, 0
	} else {
		tup.Saddr_l, tup.Saddr_h = util.ToLowHigh(c.Source)
	}
	if c.Dest.IsZero() {
		tup.Daddr_l, tup.Daddr_h = 0, 0
	} else {
		tup.Daddr_l, tup.Daddr_h = util.ToLowHigh(c.Dest)
	}
	return tup
}
