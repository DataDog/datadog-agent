package traps

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TrapForwarder consumes from a trapsIn channel, format traps and send them as EventPlatformEvents
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
	tf.sender.EventPlatformEvent(string(data), epforwarder.EventTypeSnmpTraps)
}
