// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

// TrapListener receives traps over a socket connection and processes them.
type TrapListener struct {
	addr   string
	impl   *gosnmp.TrapListener
	errors chan error
}

// NewTrapListener creates a configured trap listener.
func NewTrapListener(bindHost string, c TrapListenerConfig, output OutputChannel) (*TrapListener, error) {
	addr := fmt.Sprintf("%s:%d", bindHost, c.Port)

	params, err := c.BuildParams()
	if err != nil {
		return nil, err
	}

	impl := gosnmp.NewTrapListener()
	impl.Params = params
	impl.OnNewTrap = func(p *gosnmp.SnmpPacket, u *net.UDPAddr) {
		if !validateCredentials(p, c) {
			log.Warnf("snmp-traps: invalid credentials from %s on listener %s, dropping packet", u.String(), addr)
			trapsPacketsAuthErrors.Add(1)
			return
		}
		trapsPackets.Add(1)
		output <- &SnmpPacket{Content: p, Addr: u}
	}

	ln := &TrapListener{
		addr:   addr,
		impl:   impl,
		errors: make(chan error, 1),
	}

	return ln, nil
}

func validateCredentials(p *gosnmp.SnmpPacket, config TrapListenerConfig) bool {
	if p.Version != gosnmp.Version2c {
		// Unsupported.
		return false
	}

	// Enforce that one of the known community strings matches.
	for _, community := range config.CommunityStrings {
		if community == p.Community {
			return true
		}
	}

	return false
}

// Addr returns the listener socket address.
func (ln *TrapListener) Addr() string {
	return ln.addr
}

// Listen runs the packet reception and processing loop.
func (ln *TrapListener) Listen() {
	log.Infof("snmp-traps: starting to listen on %s", ln.addr)

	err := ln.impl.Listen(ln.addr)
	if err != nil {
		ln.errors <- err
	}
}

// WaitReadyOrError blocks until the listener is ready to receive incoming packets, or an error occurred.
func (ln *TrapListener) WaitReadyOrError() error {
	ready := ln.impl.Listening()

	select {
	case <-ready:
		break
	case err := <-ln.errors:
		close(ln.errors)
		return err
	}

	return nil
}

// Stop stops accepting incoming packets and closes the socket connection.
// Should only be called if the listener is currently running.
func (ln *TrapListener) Stop() {
	log.Debugf("snmp-traps: stopping %s", ln.addr)
	ln.impl.Close()
}
