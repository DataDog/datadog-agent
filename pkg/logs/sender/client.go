// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package sender

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type FramingError struct {
	err error
}

func NewFramingError(err error) *FramingError {
	return &FramingError{
		err: err,
	}
}

func (e *FramingError) Error() string {
	return e.err.Error()
}

type Client struct {
	prefixer    Prefixer
	delimiter   Delimiter
	connManager *ConnectionManager
	conn        net.Conn
}

func NewClient(prefixer Prefixer, delimiter Delimiter, connManager *ConnectionManager) *Client {
	return &Client{
		prefixer:    prefixer,
		delimiter:   delimiter,
		connManager: connManager,
	}
}

func (d *Client) Write(payload message.Message) error {
	if d.conn == nil {
		d.conn = d.connManager.NewConnection()
	}

	content := d.prefixer.prefix(payload.Content())
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
