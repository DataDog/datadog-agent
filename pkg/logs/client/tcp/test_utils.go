// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package tcp

import (
	"net"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

// AddrToHostPort converts a net.Addr to a (string, int).
func AddrToHostPort(remoteAddr net.Addr) (string, int) {
	switch addr := remoteAddr.(type) {
	case *net.UDPAddr:
		return addr.IP.String(), addr.Port
	case *net.TCPAddr:
		return addr.IP.String(), addr.Port
	}
	return "", 0
}

// AddrToEndPoint creates an EndPoint from an Addr.
func AddrToEndPoint(addr net.Addr) config.Endpoint {
	host, port := AddrToHostPort(addr)
	return config.NewEndpoint("", "", host, port, config.EmptyPathPrefix, false)
}

// AddrToDestination creates a Destination from an Addr
func AddrToDestination(addr net.Addr, ctx *client.DestinationsContext, status statusinterface.Status) *Destination {
	return NewDestination(AddrToEndPoint(addr), true, ctx, true, status)
}
