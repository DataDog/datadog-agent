// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common contains common functionality for both TCP and UDP
// traceroute implementations
package common

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/google/gopacket/layers"
)

type (
	// Results encapsulates a response from the
	// traceroute
	Results struct {
		Source     net.IP
		SourcePort uint16
		Target     net.IP
		DstPort    uint16
		Hops       []*Hop
	}

	// Hop encapsulates information about a single
	// hop in a traceroute
	Hop struct {
		IP       net.IP
		Port     uint16
		ICMPType layers.ICMPv4TypeCode
		RTT      time.Duration
		IsDest   bool
	}

	// CanceledError is sent when a listener
	// is canceled
	CanceledError string

	// MismatchError is an error type that indicates a MatcherFunc
	// failed due to one or more fields from the packet not matching
	// the expected information
	MismatchError string
)

// Error implements the error interface for
// CanceledError
func (c CanceledError) Error() string {
	return string(c)
}

// Error implements the error interface for
// MismatchError
func (m MismatchError) Error() string {
	return string(m)
}

// LocalAddrForHost takes in a destionation IP and port and returns the local
// address that should be used to connect to the destination. The returned connection
// should be closed by the caller when the the local UDP port is no longer needed
func LocalAddrForHost(destIP net.IP, destPort uint16) (*net.UDPAddr, net.Conn, error) {
	// this is a quick way to get the local address for connecting to the host
	// using UDP as the network type to avoid actually creating a connection to
	// the host, just get the OS to give us a local IP and local ephemeral port
	conn, err := net.Dial("udp4", net.JoinHostPort(destIP.String(), strconv.Itoa(int(destPort))))
	if err != nil {
		return nil, nil, err
	}
	localAddr := conn.LocalAddr()

	localUDPAddr, ok := localAddr.(*net.UDPAddr)
	if !ok {
		return nil, nil, fmt.Errorf("invalid address type for %s: want %T, got %T", localAddr, localUDPAddr, localAddr)
	}

	return localUDPAddr, conn, nil
}
