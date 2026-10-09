// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Configuration defaults shared by both roles.
const (
	DefaultSetupTimeout = 60 * time.Second
	DefaultRingCapacity = 1 << 20
	MaxRingCapacity     = 1 << 30
	// MaxSetupTimeout bounds absurd deadlines explicitly. The Rust template
	// reaches the same effect through its clock's representable range.
	MaxSetupTimeout = 100 * 365 * 24 * time.Hour
)

// SetupEndpoint selects the endpoint used only to establish a session.
type SetupEndpoint struct {
	isTCP bool
	path  string         // Unix socket path when isTCP is false
	addr  netip.AddrPort // loopback TCP address when isTCP is true
}

// UnixEndpoint returns a pathname Unix-domain socket endpoint, restricted by
// its parent-directory permissions.
func UnixEndpoint(path string) SetupEndpoint {
	return SetupEndpoint{path: path}
}

// TCPEndpoint returns a loopback TCP endpoint used only for the setup
// handshake.
func TCPEndpoint(addr netip.AddrPort) SetupEndpoint {
	return SetupEndpoint{isTCP: true, addr: addr}
}

// ParseEndpoint parses "unix:/path", "tcp:127.0.0.1:5101", or a bare path
// (treated as Unix for compatibility).
func ParseEndpoint(value string) (SetupEndpoint, error) {
	if tail, ok := strings.CutPrefix(value, "tcp:"); ok {
		addr, err := netip.ParseAddrPort(tail)
		if err != nil {
			return SetupEndpoint{}, fmt.Errorf("invalid TCP setup address: %v", err)
		}
		if !addr.Addr().IsLoopback() {
			return SetupEndpoint{}, errors.New("TCP setup address must use a loopback IP")
		}
		return TCPEndpoint(addr), nil
	}
	path, _ := strings.CutPrefix(value, "unix:")
	return UnixEndpoint(path), nil
}

// ProducerConfig holds the settings for the producer; the consumer offers the
// ring capacity.
type ProducerConfig struct {
	Endpoint     SetupEndpoint
	SetupTimeout time.Duration
}

// NewProducerConfig uses the given setup endpoint with the default timeout.
func NewProducerConfig(endpoint SetupEndpoint) ProducerConfig {
	return ProducerConfig{Endpoint: endpoint, SetupTimeout: DefaultSetupTimeout}
}

// ConsumerConfig holds the settings for the consumer, which owns the endpoint
// and shared memory.
type ConsumerConfig struct {
	Endpoint     SetupEndpoint
	SetupTimeout time.Duration
	RingCapacity int
}

// NewConsumerConfig uses the given setup endpoint with the default timeout
// and a 1 MiB ring.
func NewConsumerConfig(endpoint SetupEndpoint) ConsumerConfig {
	return ConsumerConfig{
		Endpoint:     endpoint,
		SetupTimeout: DefaultSetupTimeout,
		RingCapacity: DefaultRingCapacity,
	}
}

func (c ConsumerConfig) validate() (time.Time, error) {
	if err := validateCapacity(c.RingCapacity); err != nil {
		return time.Time{}, err
	}
	return setupDeadline(c.SetupTimeout)
}

func (p ProducerConfig) validate() (time.Time, error) {
	return setupDeadline(p.SetupTimeout)
}

func validateCapacity(capacity int) error {
	if capacity < 16 || capacity > MaxRingCapacity || capacity%8 != 0 {
		return errors.New("ring capacity must be a multiple of 8 from 16 bytes through 1 GiB")
	}
	return nil
}

func setupDeadline(timeout time.Duration) (time.Time, error) {
	if timeout <= 0 {
		return time.Time{}, errors.New("setup timeout must be positive")
	}
	if timeout > MaxSetupTimeout {
		return time.Time{}, errors.New("setup timeout is too large")
	}
	return time.Now().Add(timeout), nil
}
