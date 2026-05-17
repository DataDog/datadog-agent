// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package listenerimpl implements the Listener component.
package listenerimpl

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	config "github.com/DataDog/datadog-agent/comp/snmptraps/config/def"
	listener "github.com/DataDog/datadog-agent/comp/snmptraps/listener/def"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	status "github.com/DataDog/datadog-agent/comp/snmptraps/status/def"
)

// Requires defines the dependencies for the listener component.
type Requires struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Config    config.Component
	Demux     demultiplexer.Component
	Logger    log.Component
	Status    status.Component
}

// Provides defines the output of the listener component.
type Provides struct {
	compdef.Out

	Comp listener.Component
}

// NewComponent creates a new listener component.
func NewComponent(reqs Requires) (Provides, error) {
	comp, err := newTrapListener(reqs.Lifecycle, dependencies{
		Config: reqs.Config,
		Demux:  reqs.Demux,
		Logger: reqs.Logger,
		Status: reqs.Status,
	})
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: comp}, nil
}

// trapListener opens an UDP socket and put all received traps in a channel
type trapListener struct {
	config        *config.TrapsConfig
	sender        sender.Sender
	packets       packet.PacketsChannel
	listener      *gosnmp.TrapListener
	errorsChannel chan error
	logger        log.Component
	status        status.Component
}

type dependencies struct {
	Config config.Component
	Demux  demultiplexer.Component
	Logger log.Component
	Status status.Component
}

// newTrapListener creates a TrapListener and registers it with the lifecycle.
func newTrapListener(lc compdef.Lifecycle, dep dependencies) (listener.Component, error) {
	sender, err := dep.Demux.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	cfg := dep.Config.Get()
	gosnmpListener := gosnmp.NewTrapListener()
	gosnmpListener.Params, err = cfg.BuildSNMPParams(dep.Logger)
	if err != nil {
		return nil, err
	}
	errorsChan := make(chan error, 1)
	tl := &trapListener{
		config:        cfg,
		sender:        sender,
		packets:       make(packet.PacketsChannel, cfg.GetPacketChannelSize()),
		listener:      gosnmpListener,
		errorsChannel: errorsChan,
		logger:        dep.Logger,
		status:        dep.Status,
	}

	gosnmpListener.OnNewTrap = tl.receiveTrap
	if cfg.Enabled {
		lc.Append(compdef.Hook{
			OnStart: func(_ context.Context) error {
				return tl.start()
			},
			OnStop: func(_ context.Context) error {
				return tl.stop()
			},
		})
	}

	return tl, nil
}

// Packets returns the packets channel to which the listener publishes.
func (t *trapListener) Packets() packet.PacketsChannel {
	return t.packets
}

// start the TrapListener instance.
func (t *trapListener) start() error {
	t.logger.Infof("Start listening for traps on %s", t.config.Addr())
	go t.run()
	return t.blockUntilReady()
}

func (t *trapListener) run() {
	err := t.listener.Listen(t.config.Addr()) // blocking call
	if err != nil {
		t.errorsChannel <- err
	}
}

func (t *trapListener) blockUntilReady() error {
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

// stop the current TrapListener instance
func (t *trapListener) stop() error {

	stopped := make(chan interface{})

	go func() {
		t.logger.Infof("Stop listening on %s", t.config.Addr())
		t.listener.Close()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(time.Duration(t.config.StopTimeout) * time.Second):
		return fmt.Errorf("TrapListener.Stop() timed out after %d seconds", t.config.StopTimeout)
	}
	return nil
}

func (t *trapListener) receiveTrap(p *gosnmp.SnmpPacket, u *net.UDPAddr) {
	pkt := &packet.SnmpPacket{Content: p, Addr: u, Timestamp: time.Now().UnixMilli(), Namespace: t.config.Namespace}
	tags := pkt.GetTags()

	t.sender.Count("datadog.snmp_traps.received", 1, "", tags)

	if err := validatePacket(p, t.config); err != nil {
		t.logger.Debugf("Invalid credentials from %s on listener %s, dropping traps", u.String(), t.config.Addr())
		t.status.AddTrapsPacketsUnknownCommunityString(1)
		t.sender.Count("datadog.snmp_traps.invalid_packet", 1, "", append(tags, "reason:unknown_community_string"))
		return
	}
	t.logger.Debugf("Packet received from %s on listener %s", u.String(), t.config.Addr())
	t.status.AddTrapsPackets(1)
	t.packets <- pkt
}

func validatePacket(p *gosnmp.SnmpPacket, c *config.TrapsConfig) error {
	if p.Version == gosnmp.Version3 {
		// v3 Packets are already decrypted and validated by gosnmp
		return nil
	}

	// At least one of the known community strings must match.
	for _, community := range c.CommunityStrings {
		// Simple string equality check, but in constant time to avoid timing attacks
		if subtle.ConstantTimeCompare([]byte(community), []byte(p.Community)) == 1 {
			return nil
		}
	}

	return errors.New("unknown community string")
}
