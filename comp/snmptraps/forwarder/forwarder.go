// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package forwarder

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/sender"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/packet"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"go.uber.org/fx"
)

// TrapForwarder consumes from a trapsIn channel, format traps and send them as EventPlatformEvents
// The TrapForwarder is an intermediate step between the listener and the epforwarder in order to limit the processing of the listener
// to the minimum. The forwarder process payloads received by the listener via the trapsIn channel, formats them and finally
// give them to the epforwarder for sending it to Datadog.
type TrapForwarder struct {
	trapsIn   packet.PacketsChannel
	formatter formatter.Component
	sender    sender.Component
	stopChan  chan struct{}
	logger    log.Component
}

type dependencies struct {
	fx.In
	Formatter formatter.Component
	Sender    sender.Component
	Listener  listener.Component
	Logger    log.Component
}

// NewTrapForwarder creates a simple TrapForwarder instance
func NewTrapForwarder(dep dependencies) (Component, error) {
	return newTrapForwarder(dep.Listener.Packets(), dep.Formatter, dep.Sender, dep.Logger)
}

// split out for testing
func newTrapForwarder(packets packet.PacketsChannel, formatter formatter.Component, sender sender.Component, logger log.Component) (Component, error) {
	return &TrapForwarder{
		trapsIn:   packets,
		formatter: formatter,
		sender:    sender,
		stopChan:  make(chan struct{}),
		logger:    logger,
	}, nil
}

// Start the TrapForwarder instance. Need to Stop it manually
func (tf *TrapForwarder) Start() error {
	tf.logger.Info("Starting TrapForwarder")
	go tf.run()
	return nil
}

// Stop the TrapForwarder instance.
func (tf *TrapForwarder) Stop() {
	tf.stopChan <- struct{}{}
}

func (tf *TrapForwarder) run() {
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

func (tf *TrapForwarder) sendTrap(packet *packet.SnmpPacket) {
	data, err := tf.formatter.FormatPacket(packet)
	if err != nil {
		tf.logger.Errorf("failed to format packet: %s", err)
		return
	}
	tf.logger.Tracef("send trap payload: %s", string(data))
	tf.sender.Count("datadog.snmp_traps.forwarded", 1, "", packet.GetTags())
	tf.sender.EventPlatformEvent(data, epforwarder.EventTypeSnmpTraps)
}
