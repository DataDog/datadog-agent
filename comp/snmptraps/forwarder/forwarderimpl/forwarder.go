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
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newTrapForwarder),
)

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
	fx.In
	Config    config.Component
	Formatter formatter.Component
	Demux     demultiplexer.Component
	Listener  listener.Component
	Logger    log.Component
}

// newTrapForwarder creates a simple TrapForwarder instance
func newTrapForwarder(lc fx.Lifecycle, dep dependencies) (forwarder.Component, error) {
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
		lc.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				tf.Start()
				return nil
			},
			OnStop: func(ctx context.Context) error {
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
	flushTicker := time.NewTicker(10 * time.Second).C
	for {
		select {
		case <-tf.stopChan:
			tf.logger.Info("Stopped TrapForwarder")
			return
		case packet := <-tf.trapsIn:
			tf.sendTrap(packet)
		case <-flushTicker:
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
	tf.sender.EventPlatformEvent(data, eventplatformimpl.EventTypeSnmpTraps)
}
