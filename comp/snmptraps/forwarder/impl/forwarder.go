// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package forwarderimpl implements the forwarder component.
package forwarderimpl

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	config "github.com/DataDog/datadog-agent/comp/snmptraps/config/def"
	formatter "github.com/DataDog/datadog-agent/comp/snmptraps/formatter/def"
	forwarder "github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/def"
	listener "github.com/DataDog/datadog-agent/comp/snmptraps/listener/def"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// Requires defines the dependencies for the forwarder component.
type Requires struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Config    config.Component
	Formatter formatter.Component
	Demux     demultiplexer.Component
	Listener  listener.Component
	Logger    log.Component
}

// Provides defines the output of the forwarder component.
type Provides struct {
	compdef.Out

	Comp forwarder.Component
}

// NewComponent creates a new forwarder component.
func NewComponent(reqs Requires) (Provides, error) {
	comp, err := newTrapForwarder(reqs.Lifecycle, dependencies{
		Config:    reqs.Config,
		Formatter: reqs.Formatter,
		Demux:     reqs.Demux,
		Listener:  reqs.Listener,
		Logger:    reqs.Logger,
	})
	if err != nil {
		return Provides{}, err
	}
	return Provides{Comp: comp}, nil
}

// trapForwarder consumes SNMP packets, formats traps and send them as EventPlatformEvents
// The trapForwarder is an intermediate step between the listener and the epforwarder in order to limit the processing of the listener
// to the minimum. The forwarder process payloads received by the listener via the trapsIn channel, formats them and finally
// give them to the epforwarder for sending it to Datadog.
type trapForwarder struct {
	trapsIn   packet.PacketsChannel
	formatter formatter.Component
	sender    sender.Sender
	stopChan  chan struct{}
	logger    log.Component
}

type dependencies struct {
	Config    config.Component
	Formatter formatter.Component
	Demux     demultiplexer.Component
	Listener  listener.Component
	Logger    log.Component
}

// newTrapForwarder creates a simple TrapForwarder instance
func newTrapForwarder(lc compdef.Lifecycle, dep dependencies) (forwarder.Component, error) {
	sender, err := dep.Demux.GetDefaultSender()
	if err != nil {
		return nil, err
	}
	tf := &trapForwarder{
		trapsIn:   dep.Listener.Packets(),
		formatter: dep.Formatter,
		sender:    sender,
		stopChan:  make(chan struct{}, 1),
		logger:    dep.Logger,
	}
	conf := dep.Config.Get()
	if conf.Enabled {
		lc.Append(compdef.Hook{
			OnStart: func(_ context.Context) error {
				tf.Start()
				return nil
			},
			OnStop: func(_ context.Context) error {
				tf.Stop()
				return nil
			},
		})
	}

	return tf, nil
}

// Start the TrapForwarder instance. Need to Stop it manually.
func (tf *trapForwarder) Start() {
	tf.logger.Info("Starting TrapForwarder")
	go tf.run()
}

// Stop the TrapForwarder instance.
func (tf *trapForwarder) Stop() {
	select {
	case tf.stopChan <- struct{}{}:
	default:
		tf.logger.Warn("TrapForwarder stopped twice.")
	}
}

func (tf *trapForwarder) run() {
	flushTicker := time.NewTicker(10 * time.Second)
	defer flushTicker.Stop()
	for {
		select {
		case <-tf.stopChan:
			tf.logger.Info("Stopped TrapForwarder")
			return
		case packet := <-tf.trapsIn:
			tf.sendTrap(packet)
		case <-flushTicker.C:
			tf.sender.Commit() // Commit metrics
		}
	}
}

func (tf *trapForwarder) sendTrap(packet *packet.SnmpPacket) {
	data, err := tf.formatter.FormatPacket(packet)
	if err != nil {
		tf.logger.Errorf("failed to format packet: %s", err)
		return
	}
	tf.logger.Tracef("send trap payload: %s", string(data))
	tf.sender.Count("datadog.snmp_traps.forwarded", 1, "", packet.GetTags())
	tf.sender.EventPlatformEvent(data, eventplatform.EventTypeSnmpTraps)
}
