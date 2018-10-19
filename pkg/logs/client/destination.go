// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package client

import (
	"net"
)

// FramingError represents a kind of error that can occur when a log can not properly
// be transformed into a frame.
type FramingError struct {
	err error
}

// NewFramingError returns a new framing error.
func NewFramingError(err error) *FramingError {
	return &FramingError{
		err: err,
	}
}

// Error returns the message of the error.
func (e *FramingError) Error() string {
	return e.err.Error()
}

// Destination is responsible for shipping logs to a remote server over TCP.
type Destination struct {
	prefixer            Prefixer
	delimiter           Delimiter
	connManager         *ConnectionManager
	destinationsContext *DestinationsContext
	conn                net.Conn
	inputChan           chan []byte
}

// NewDestination returns a new destination.
func NewDestination(endpoint Endpoint, destinationsContext *DestinationsContext) *Destination {
	return &Destination{
		prefixer:            NewAPIKeyPrefixer(endpoint.APIKey, endpoint.Logset),
		delimiter:           NewDelimiter(endpoint.UseProto),
		connManager:         NewConnectionManager(endpoint),
		destinationsContext: destinationsContext,
	}
}

// Send transforms a message into a frame and sends it to a remote server,
// returns an error if the operation failed.
func (d *Destination) Send(payload []byte) error {
	if d.conn == nil {
		var err error

		// We work only if we have a started destination context
		ctx := d.destinationsContext.Context()
		if d.conn, err = d.connManager.NewConnection(ctx); err != nil {
			return err
		}
	}

	content := d.prefixer.prefix(payload)
	frame, err := d.delimiter.delimit(content)
	if err != nil {
		return NewFramingError(err)
	}

	_, err = d.conn.Write(frame)
	if err != nil {
		d.connManager.CloseConnection(d.conn)
		d.conn = nil
		return err
	}

	return nil
}

// SendAsync sends a message to the destination without blocking. If the queue is full, the incoming messages will be
// dropped
func (d *Destination) SendAsync(payload []byte) {
	select {
	case d.inputChan <- payload:
	default:
		// FIXME: Is is OK to drop the logs when the pipe is full?
		// Should we warn users when messages are dropped for the
		// additional destinations
	}
}

// ConsumeAsync read the messages from the queue and send them
func (d *Destination) ConsumeAsync() {
	// FIXME: Remove this magic number, how to decide of the right buffer size?
	inputChan := make(chan []byte, 100)
	d.inputChan = inputChan
	ctx := d.destinationsContext.Context()
	for {
		select {
		case payload := <-d.inputChan:
			d.Send(payload)
		case <-ctx.Done():
		}
	}
}
