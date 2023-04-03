// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package traps

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TrapForwarder consumes from a trapsIn channel, format traps and send them as EventPlatformEvents
// The TrapForwarder is an intermediate step between the listener and the epforwarder in order to limit the processing of the listener
// to the minimum. The forwarder process payloads received by the listener via the trapsIn channel, formats them and finally
// give them to the epforwarder for sending it to Datadog.
type TrapForwarder struct {
	trapsIn   PacketsChannel
	formatter Formatter
	sender    aggregator.Sender
	stopChan  chan struct{}
}

// NewTrapForwarder creates a simple TrapForwarder instance
func NewTrapForwarder(formatter Formatter, sender aggregator.Sender, packets PacketsChannel) (*TrapForwarder, error) {
	return &TrapForwarder{
		trapsIn:   packets,
		formatter: formatter,
		sender:    sender,
		stopChan:  make(chan struct{}),
	}, nil
}

// Start the TrapForwarder instance. Need to Stop it manually
func (tf *TrapForwarder) Start() {
	log.Info("Starting TrapForwarder")
	go tf.run()
}

// Stop the TrapForwarder instance.
func (tf *TrapForwarder) Stop() {
	tf.stopChan <- struct{}{}
}

func (tf *TrapForwarder) run() {
	for {
		select {
		case <-tf.stopChan:
			log.Info("Stopped TrapForwarder")
			return
		case packet := <-tf.trapsIn:
			tf.sendTrap(packet)
		}
	}
}

func (tf *TrapForwarder) sendTrap(packet *SnmpPacket) {
	data, err := tf.formatter.FormatPacket(packet)
	if err != nil {
		log.Errorf("failed to format packet: %s", err)
		return
	}
	log.Tracef("send trap payload: %s", string(data))
	tf.sender.EventPlatformEvent(data, epforwarder.EventTypeSnmpTraps)
}
