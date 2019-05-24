package ebpf

import (
	"bytes"

	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	agent "github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
)

var jsonMarshaler = jsonpb.Marshaler{}

// MarshalProtobuf serializes a Connections object into a Protobuf message
func MarshalProtobuf(conns *Connections) ([]byte, error) {
	agentConns := make([]*agent.Connection, len(conns.Conns))
	for i, conn := range conns.Conns {
		agentConns[i] = toAgentConnection(conn)
	}
	payload := &agent.Connections{Conns: agentConns}
	return proto.Marshal(payload)
}

// UnmarshalConnections deserializes a Protobuf message into a Connections object
func UnmarshalProtobuf(blob []byte) (*Connections, error) {
	payload := new(agent.Connections)
	if err := proto.Unmarshal(blob, payload); err != nil {
		return nil, err
	}
	conns := make([]ConnectionStats, len(payload.Conns))
	for i, conn := range payload.Conns {
		ebpfConn := fromAgentConnection(conn)
		conns[i] = ebpfConn
	}
	return &Connections{Conns: conns}, nil
}

// MarshalJSON serializes a Connections object into a JSON document
func MarshalJSON(conns *Connections) ([]byte, error) {
	agentConns := make([]*agent.Connection, len(conns.Conns))
	for i, conn := range conns.Conns {
		agentConns[i] = toAgentConnection(conn)
	}
	payload := &agent.Connections{Conns: agentConns}

	writer := new(bytes.Buffer)
	err := jsonMarshaler.Marshal(writer, payload)
	return writer.Bytes(), err
}

// UnmarshalJSON deserializes a JSON document into a Connections object
func UnmarshalJSON(blob []byte) (*Connections, error) {
	payload := new(agent.Connections)
	reader := bytes.NewReader(blob)
	if err := jsonpb.Unmarshal(reader, payload); err != nil {
		return nil, err
	}
	conns := make([]ConnectionStats, len(payload.Conns))
	for i, conn := range payload.Conns {
		ebpfConn := fromAgentConnection(conn)
		conns[i] = ebpfConn
	}
	return &Connections{Conns: conns}, nil
}

func toAgentConnection(conn ConnectionStats) *agent.Connection {
	agentConn := &agent.Connection{
		Pid:                int32(conn.Pid),
		Family:             agent.ConnectionFamily(conn.Family),
		Type:               agent.ConnectionType(conn.Type),
		TotalBytesSent:     conn.MonotonicSentBytes,
		TotalBytesReceived: conn.MonotonicRecvBytes,
		TotalRetransmits:   conn.MonotonicRetransmits,
		LastBytesSent:      conn.LastSentBytes,
		LastBytesReceived:  conn.LastRecvBytes,
		LastRetransmits:    conn.LastRetransmits,
		Direction:          agent.ConnectionDirection(conn.Direction),
		NetNS:              conn.NetNS,
	}

	if conn.Source != nil {
		agentConn.Laddr = &agent.Addr{Ip: conn.Source.String(), Port: int32(conn.SPort)}
	}

	if conn.Dest != nil {
		agentConn.Raddr = &agent.Addr{Ip: conn.Dest.String(), Port: int32(conn.DPort)}
	}

	if conn.IPTranslation != nil {
		agentConn.IpTranslation = &agent.IPTranslation{
			ReplSrcIP:   conn.IPTranslation.ReplSrcIP,
			ReplDstIP:   conn.IPTranslation.ReplDstIP,
			ReplSrcPort: int32(conn.IPTranslation.ReplSrcPort),
			ReplDstPort: int32(conn.IPTranslation.ReplDstPort),
		}
	}

	return agentConn
}

func fromAgentConnection(agentConn *agent.Connection) ConnectionStats {
	conn := ConnectionStats{
		MonotonicSentBytes:   agentConn.TotalBytesSent,
		LastSentBytes:        agentConn.LastBytesSent,
		MonotonicRecvBytes:   agentConn.TotalBytesReceived,
		LastRecvBytes:        agentConn.LastBytesReceived,
		MonotonicRetransmits: agentConn.TotalRetransmits,
		LastRetransmits:      agentConn.LastRetransmits,
		Pid:                  uint32(agentConn.Pid),
		NetNS:                agentConn.NetNS,
		Type:                 ConnectionType(agentConn.Type),
		Family:               ConnectionFamily(agentConn.Family),
		Direction:            ConnectionDirection(agentConn.Direction),
	}

	if agentConn.Laddr != nil {
		conn.Source = util.AddressFromString(agentConn.Laddr.Ip)
		conn.SPort = uint16(agentConn.Laddr.Port)
	}

	if agentConn.Raddr != nil {
		conn.Dest = util.AddressFromString(agentConn.Raddr.Ip)
		conn.DPort = uint16(agentConn.Raddr.Port)
	}

	if agentConn.IpTranslation != nil {
		conn.IPTranslation = &netlink.IPTranslation{
			ReplSrcIP:   agentConn.IpTranslation.ReplSrcIP,
			ReplDstIP:   agentConn.IpTranslation.ReplDstIP,
			ReplSrcPort: uint16(agentConn.IpTranslation.ReplSrcPort),
			ReplDstPort: uint16(agentConn.IpTranslation.ReplDstPort),
		}
	}

	return conn
}
