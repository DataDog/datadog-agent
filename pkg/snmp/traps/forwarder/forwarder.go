// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package forwarder defines a type that receives trap data from the
// listener, formats it properly, and sends it to the backend.
package forwarder

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/formatter"
	"github.com/DataDog/datadog-agent/pkg/snmp/traps/packet"
)

// TrapForwarder consumes from a trapsIn channel, format traps and send them as EventPlatformEvents
// The TrapForwarder is an intermediate step between the listener and the epforwarder in order to limit the processing of the listener
// to the minimum. The forwarder process payloads received by the listener via the trapsIn channel, formats them and finally
// give them to the epforwarder for sending it to Datadog.
type TrapForwarder struct {
	trapsIn   packet.PacketsChannel
	formatter formatter.Formatter
	sender    sender.Sender
	stopChan  chan struct{}
	logger    log.Component
}

// NewTrapForwarder creates a simple TrapForwarder instance
func NewTrapForwarder(formatter formatter.Formatter, sender sender.Sender, packets packet.PacketsChannel, logger log.Component) (*TrapForwarder, error) {
	return &TrapForwarder{
		trapsIn:   packets,
		formatter: formatter,
		sender:    sender,
		stopChan:  make(chan struct{}),
		logger:    logger,
	}, nil
}

// Start the TrapForwarder instance. Need to Stop it manually.
func (tf *TrapForwarder) Start() {
	tf.logger.Info("Starting TrapForwarder")
	go tf.run()
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
