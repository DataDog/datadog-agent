package traps

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

// TrapListener receives traps over a socket connection and processes them.
type TrapListener struct {
	addr string
	impl *gosnmp.TrapListener
}

// NewTrapListener creates a configured trap listener.
func NewTrapListener(bindHost string, c TrapListenerConfig) (*TrapListener, error) {
	addr := fmt.Sprintf("%s:%d", bindHost, c.Port)

	params, err := c.BuildParams()
	if err != nil {
		return nil, err
	}

	impl := gosnmp.NewTrapListener()
	impl.Params = params
	impl.OnNewTrap = handleTrap

	listener := &TrapListener{
		addr: addr,
		impl: impl,
	}

	return listener, nil
}

func handleTrap(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	log.Infof("Received trap packet: %v, %v", packet, addr)
}

// Listen runs the packet reception and processing loop.
func (ln *TrapListener) Listen() {
	log.Infof("snmp-traps: starting to listen on %s", ln.addr)

	err := ln.impl.Listen(ln.addr)

	if err != nil {
		log.Errorf("snmp-traps: error occurred while listening on %s: %s", ln.addr, err)
	}
}

// Stop stops accepting incoming packets and closes the socket connection.
func (ln *TrapListener) Stop() {
	log.Infof("snmp-traps: stopping %s", ln.addr)
	// FIXME consider the case when an error occurred while listening (the socket may already be closed).
	ln.impl.Close()
}
