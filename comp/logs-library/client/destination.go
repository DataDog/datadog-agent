// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package client provides log destination client functionality
package client

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Destination sends a payload to a specific endpoint over a given network protocol.
type Destination interface {
	// Whether or not destination is used for Multi-Region Failover mode
	IsMRF() bool

	// Destination target (e.g. https://agent-intake.logs.datadoghq.com)
	Target() string

	// Metadata returns the metadata of the destination
	Metadata() *DestinationMetadata

	// Start starts the destination send loop. close the intput to stop listening for payloads. stopChan is
	// signaled when the destination has fully shutdown and all buffered payloads have been flushed. isRetrying is
	// signaled when the retry state changes. isRetrying can be nil if you don't need to handle retries.
	Start(input chan *message.Payload, output chan *message.Payload, isRetrying chan bool) (stopChan <-chan struct{})
}
