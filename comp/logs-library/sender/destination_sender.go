// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sender

import (
	"sync"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DestinationSender wraps a destination to send messages blocking on a full buffer, but not blocking when
// a destination is retrying
type DestinationSender struct {
	config            pkgconfigmodel.Reader
	sendEnabled       bool
	input             chan *message.Payload
	destination       client.Destination
	retryReader       chan bool
	stopChan          <-chan struct{}
	retryLock         sync.Mutex
	lastRetryState    bool
	cancelSendChan    chan struct{}
	lastSendSucceeded bool
}

// NewDestinationSender creates a new DestinationSender
func NewDestinationSender(config pkgconfigmodel.Reader, destination client.Destination, output chan *message.Payload, bufferSize int) *DestinationSender {
	inputChan := make(chan *message.Payload, bufferSize)
	retryReader := make(chan bool, 1)
	stopChan := destination.Start(inputChan, output, retryReader)

	d := &DestinationSender{
		config:            config,
		sendEnabled:       true,
		input:             inputChan,
		destination:       destination,
		retryReader:       retryReader,
		stopChan:          stopChan,
		retryLock:         sync.Mutex{},
		lastRetryState:    false,
		cancelSendChan:    nil,
		lastSendSucceeded: false,
	}
	d.startRetryReader()

	return d
}

func (d *DestinationSender) startRetryReader() {
	go func() {
		for v := range d.retryReader {
			d.retryLock.Lock()
			if d.cancelSendChan != nil && !d.lastRetryState {
				select {
				case d.cancelSendChan <- struct{}{}:
				default:
				}
			}
			d.lastRetryState = v
			d.retryLock.Unlock()
		}
	}()
}

// Stop stops the DestinationSender
func (d *DestinationSender) Stop() {
	close(d.input)
	<-d.stopChan
	close(d.retryReader)
}

func (d *DestinationSender) canSend() bool {
	if d.destination.IsMRF() {
		if !d.sendEnabled {
			if d.config.GetBool("multi_region_failover.enabled") && d.config.GetBool("multi_region_failover.failover_logs") {
				d.sendEnabled = true
				log.Infof("Forwarder for domain %v has been failed over to, enabling it for MRF.", d.destination.Target())
			} else {
				log.Debugf("Forwarder for domain %v is disabled; dropping transaction for this domain.", d.destination.Target())
			}
		} else {
			if !d.config.GetBool("multi_region_failover.enabled") || !d.config.GetBool("multi_region_failover.failover_logs") {
				d.sendEnabled = false
				log.Infof("Forwarder for domain %v was disabled; transactions will be dropped for this domain.", d.destination.Target())
			}
		}
	}

	return d.sendEnabled
}

// Send sends a payload and blocks if the input is full. It will not block if the destination
// is retrying payloads and will cancel the blocking attempt if the retry state changes
func (d *DestinationSender) Send(payload *message.Payload) bool {
	d.lastSendSucceeded = false
	d.retryLock.Lock()
	d.cancelSendChan = make(chan struct{}, 1)
	isRetrying := d.lastRetryState
	d.retryLock.Unlock()

	defer func() {
		d.retryLock.Lock()
		close(d.cancelSendChan)
		d.cancelSendChan = nil
		d.retryLock.Unlock()
	}()

	if !isRetrying {
		// if we can't send, we consider the send call as successful because we don't want to block the
		// pipeline when HA failover is knowingly disabled
		if !d.canSend() {
			d.lastSendSucceeded = true
			return true
		}

		select {
		case d.input <- payload:
			d.lastSendSucceeded = true
			return true
		case <-d.cancelSendChan:
		}
	}
	return false
}

// NonBlockingSend tries to send the payload and fails silently if the input is full.
// returns false if the buffer is full - true if successful.
func (d *DestinationSender) NonBlockingSend(payload *message.Payload) bool {
	select {
	case d.input <- payload:
		return true
	default:
	}
	return false
}
