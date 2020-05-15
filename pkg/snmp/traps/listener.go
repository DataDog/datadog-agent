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
func NewTrapListener(bindHost string, c TrapListenerConfig) (*TrapListener, error) {
	addr := fmt.Sprintf("%s:%d", bindHost, c.Port)

	params, err := c.BuildParams()
	if err != nil {
		return nil, err
	}

	impl := gosnmp.NewTrapListener()
	impl.Params = params
	impl.OnNewTrap = handleTrap

	errors := make(chan error, 1)

	listener := &TrapListener{
		addr:   addr,
		impl:   impl,
		errors: errors,
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
func (ln *TrapListener) Stop() {
	log.Debugf("snmp-traps: stopping %s", ln.addr)
	ln.impl.Close()
}
