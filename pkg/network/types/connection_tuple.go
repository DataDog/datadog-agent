//go:generate go run golang.org/x/tools/cmd/stringer@latest -output connection_tuple_string.go -type=ConnectionType,ConnectionFamily,ConnectionDirection -linecomment

package types

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// ConnectionType will be either TCP or UDP
type ConnectionType uint8

const (
	// TCP connection type
	TCP ConnectionType = 0

	// UDP connection type
	UDP ConnectionType = 1
)

var (
	tcpLabels = map[string]string{"ip_proto": TCP.String()}
	udpLabels = map[string]string{"ip_proto": UDP.String()}
)

// Tags returns `ip_proto` tags for use in hot-path telemetry
func (c ConnectionType) Tags() map[string]string {
	switch c {
	case TCP:
		return tcpLabels
	case UDP:
		return udpLabels
	default:
		return nil
	}
}

const (
	// AFINET represents v4 connections
	AFINET ConnectionFamily = 0 // v4

	// AFINET6 represents v6 connections
	AFINET6 ConnectionFamily = 1 // v6
)

// ConnectionFamily will be either v4 or v6
type ConnectionFamily uint8

// ConnectionDirection indicates if the connection is incoming to the host or outbound
type ConnectionDirection uint8

const (
	// UNKNOWN represents connections where the direction is not known (yet)
	UNKNOWN ConnectionDirection = 0

	// INCOMING represents connections inbound to the host
	INCOMING ConnectionDirection = 1 // incoming

	// OUTGOING represents outbound connections from the host
	OUTGOING ConnectionDirection = 2 // outgoing

	// LOCAL represents connections that don't leave the host
	LOCAL ConnectionDirection = 3 // local

	// NONE represents connections that have no direction (udp, for example)
	NONE ConnectionDirection = 4 // none
)

// ConnectionTuple represents the unique network key for a connection
type ConnectionTuple struct {
	Source    util.Address
	Dest      util.Address
	Pid       uint32
	NetNS     uint32
	SPort     uint16
	DPort     uint16
	Type      ConnectionType
	Family    ConnectionFamily
	Direction ConnectionDirection
}

func (c ConnectionTuple) String() string {
	return fmt.Sprintf(
		"[%s%s] [PID: %d] [ns: %d] [%s:%d ⇄ %s:%d] ",
		c.Type,
		c.Family,
		c.Pid,
		c.NetNS,
		c.Source,
		c.SPort,
		c.Dest,
		c.DPort,
	)
}
