// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/hook"
)

// connectionViewAdapter wraps a *model.Connection to implement hook.ConnectionView.
// Zero-copy: the adapter references the original protobuf struct directly.
type connectionViewAdapter struct {
	conn *model.Connection
}

var _ hook.ConnectionView = (*connectionViewAdapter)(nil)

func (a *connectionViewAdapter) GetPid() int32                  { return a.conn.Pid }
func (a *connectionViewAdapter) GetLocalIP() string             { return a.conn.Laddr.GetIp() }
func (a *connectionViewAdapter) GetLocalPort() int32            { return a.conn.Laddr.GetPort() }
func (a *connectionViewAdapter) GetLocalContainerID() string    { return a.conn.Laddr.GetContainerId() }
func (a *connectionViewAdapter) GetRemoteIP() string            { return a.conn.Raddr.GetIp() }
func (a *connectionViewAdapter) GetRemotePort() int32           { return a.conn.Raddr.GetPort() }
func (a *connectionViewAdapter) GetRemoteContainerID() string   { return a.conn.Raddr.GetContainerId() }
func (a *connectionViewAdapter) GetFamily() uint32              { return uint32(a.conn.Family) }
func (a *connectionViewAdapter) GetConnType() uint32            { return uint32(a.conn.Type) }
func (a *connectionViewAdapter) GetDirection() uint32           { return uint32(a.conn.Direction) }
func (a *connectionViewAdapter) GetNetNS() uint32               { return a.conn.NetNS }
func (a *connectionViewAdapter) GetLastBytesSent() uint64       { return a.conn.LastBytesSent }
func (a *connectionViewAdapter) GetLastBytesReceived() uint64   { return a.conn.LastBytesReceived }
func (a *connectionViewAdapter) GetLastPacketsSent() uint64     { return a.conn.LastPacketsSent }
func (a *connectionViewAdapter) GetLastPacketsReceived() uint64 { return a.conn.LastPacketsReceived }
func (a *connectionViewAdapter) GetLastRetransmits() uint32     { return a.conn.LastRetransmits }
func (a *connectionViewAdapter) GetRtt() uint32                 { return a.conn.Rtt }
func (a *connectionViewAdapter) GetRttVar() uint32              { return a.conn.RttVar }
func (a *connectionViewAdapter) GetIntraHost() bool             { return a.conn.IntraHost }
func (a *connectionViewAdapter) GetDnsSuccessfulResponses() uint32 {
	return a.conn.DnsSuccessfulResponses
}
func (a *connectionViewAdapter) GetDnsFailedResponses() uint32 { return a.conn.DnsFailedResponses }
func (a *connectionViewAdapter) GetDnsTimeouts() uint32        { return a.conn.DnsTimeouts }
func (a *connectionViewAdapter) GetDnsSuccessLatencySum() uint64 {
	return a.conn.DnsSuccessLatencySum
}
func (a *connectionViewAdapter) GetDnsFailureLatencySum() uint64 {
	return a.conn.DnsFailureLatencySum
}
func (a *connectionViewAdapter) GetLastTcpEstablished() uint32 { return a.conn.LastTcpEstablished }
func (a *connectionViewAdapter) GetLastTcpClosed() uint32      { return a.conn.LastTcpClosed }
