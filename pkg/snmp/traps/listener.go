// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

// TrapListener opens an UDP socket and put all received traps in a channel
type TrapListener struct {
	config        Config
	aggregator    sender.Sender
	packets       PacketsChannel
	listener      *gosnmp.TrapListener
	errorsChannel chan error
	logger        log.Component
}

// NewTrapListener creates a simple TrapListener instance but does not start it
func NewTrapListener(config Config, aggregator sender.Sender, packets PacketsChannel, logger log.Component) (*TrapListener, error) {
	var err error
	gosnmpListener := gosnmp.NewTrapListener()
	gosnmpListener.Params, err = config.BuildSNMPParams(logger)
	if err != nil {
		return nil, err
	}
	errorsChan := make(chan error, 1)
	trapListener := &TrapListener{
		config:        config,
		aggregator:    aggregator,
		packets:       packets,
		listener:      gosnmpListener,
		errorsChannel: errorsChan,
		logger:        logger,
	}

	gosnmpListener.OnNewTrap = trapListener.receiveTrap
	return trapListener, nil
}

// Start the TrapListener instance. Need to be manually Stopped
func (t *TrapListener) Start() error {
	t.logger.Infof("Start listening for traps on %s", t.config.Addr())
	go t.run()
	return t.blockUntilReady()
}

func (t *TrapListener) run() {
	err := t.listener.Listen(t.config.Addr()) // blocking call
	if err != nil {
		t.errorsChannel <- err
	}

}

func (t *TrapListener) blockUntilReady() error {
	select {
	// Wait for listener to be started and listening to traps.
	// See: https://godoc.org/github.com/gosnmp/gosnmp#TrapListener.Listening
	case <-t.listener.Listening():
		return nil
	// If the listener failed to start (eg because it couldn't bind to a socket),
	// we'll get an error here.
	case err := <-t.errorsChannel:
		return fmt.Errorf("error happened when listening for SNMP Traps: %s", err)
	}
}

// Stop the current TrapListener instance
func (t *TrapListener) Stop() {
	t.listener.Close()
}

func (t *TrapListener) receiveTrap(p *gosnmp.SnmpPacket, u *net.UDPAddr) {
	packet := &SnmpPacket{Content: p, Addr: u, Timestamp: time.Now().UnixMilli(), Namespace: t.config.Namespace}
	tags := packet.getTags()

	t.aggregator.Count("datadog.snmp_traps.received", 1, "", tags)

	if err := validatePacket(p, t.config); err != nil {
		t.logger.Debugf("Invalid credentials from %s on listener %s, dropping traps", u.String(), t.config.Addr())
		trapsPacketsAuthErrors.Add(1)
		t.aggregator.Count("datadog.snmp_traps.invalid_packet", 1, "", append(tags, "reason:unknown_community_string"))
		return
	}
	t.logger.Debugf("Packet received from %s on listener %s", u.String(), t.config.Addr())
	trapsPackets.Add(1)
	t.packets <- packet
}
