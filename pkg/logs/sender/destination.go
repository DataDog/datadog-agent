// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
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
	prefixer    Prefixer
	delimiter   Delimiter
	connManager *ConnectionManager
	conn        net.Conn
}

// NewDestination returns a new destination.
func NewDestination(endpoint config.Endpoint) *Destination {
	return &Destination{
		prefixer:    NewAPIKeyPrefixer(endpoint.APIKey, endpoint.Logset),
		delimiter:   NewDelimiter(endpoint.UseProto),
		connManager: NewConnectionManager(endpoint),
	}
}

// Send transforms a message into a frame and sends it to a remote server,
// returns an error if the operation failed.
func (d *Destination) Send(payload *message.Message) error {
	if d.conn == nil {
		d.conn = d.connManager.NewConnection()
	}

	content := d.prefixer.prefix(payload.Content)
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
