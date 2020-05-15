package snmp

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

// TrapServer represents an SNMP traps server.
type TrapServer struct {
	config   *TrapConfig
	listener *gosnmp.TrapListener
}

// NewTrapServer returns a running SNMP traps server.
func NewTrapServer() (*TrapServer, error) {
	c := TrapConfig{
		BindHost:  config.Datadog.GetString("bind_host"),
		Port:      1620,
		Version:   "2",
		Community: "public",
	}

	params, err := c.BuildParams()
	if err != nil {
		return nil, err
	}

	listener := gosnmp.NewTrapListener()
	listener.Params = params

	s := &TrapServer{
		config:   &c,
		listener: listener,
	}

	listener.OnNewTrap = s.handleTrapPacket

	go s.listenForTraps()

	return s, nil
}

func (s *TrapServer) handleTrapPacket(packet *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	log.Infof("Received trap packet: %v, %v", packet, addr)
}

// Run the traps intake loop. Should be run in its own goroutine.
func (s *TrapServer) listenForTraps() {
	addr := fmt.Sprintf("%s:%d", s.config.BindHost, s.config.Port)
	log.Infof("snmp-traps: starting to listen on %s", addr)

	err := s.listener.Listen(addr)
	if err != nil {
		log.Errorf("snmp-traps: error occurred while listening for traps: %s", err)
	}
}

// Stop stops the TrapServer.
func (s *TrapServer) Stop() {
	// FIXME: it looks like if an error occurred while calling `.Listen()`, this call results a panic error.
	s.listener.Close()
}
