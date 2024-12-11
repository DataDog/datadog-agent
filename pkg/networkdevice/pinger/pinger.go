// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pinger implements ICMP ping functionality for the agent
package pinger

import (
	"errors"
	"time"
)

const (
	defaultCount    = 2
	defaultInterval = 20 * time.Millisecond
	defaultTimeout  = 3 * time.Second
)

var (
	// ErrRawSocketUnsupported is sent when the pinger is configured to use raw sockets
	// when raw socket based pings are not supported on the system
	ErrRawSocketUnsupported = errors.New("raw socket cannot be used with this OS")
	// ErrUDPSocketUnsupported is sent when the pinger is configured to use UDP sockets
	// when UDP socket based pings are not supported on the system
	ErrUDPSocketUnsupported = errors.New("udp socket cannot be used with this OS")
)

type (
	// Config defines how pings should be run
	// across all hosts
	Config struct {
		// UseRawSocket determines the socket type to use
		// RAW or UDP
		UseRawSocket bool
		// Interval is the amount of time to wait between
		// sending ICMP packets, default is 1 second
		Interval time.Duration
		// Timeout is the total time to wait for all pings
		// to complete
		Timeout time.Duration
		// Count is the number of ICMP packets, pings, to send
		Count int
	}

	// Pinger is an interface for sending an ICMP ping to a host
	Pinger interface {
		Ping(host string) (*Result, error)
	}

	// Result encapsulates the results of a single run
	// of ping
	Result struct {
		// CanConnect is true if we receive a response from any
		// of the packets on the host
		CanConnect bool `json:"can_connect"`
		// PacketLoss indicates the percentage of packets lost
		PacketLoss float64 `json:"packet_loss"`
		// AvgRtt is the average round trip time
		AvgRtt time.Duration `json:"avg_rtt"`
	}
)
