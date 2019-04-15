// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package client

import (
	"expvar"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FIXME: Changed chanSize to a constant once we refactor packages
const (
	chanSize      = 100
	warningPeriod = 1000
)

// API key / message separator
const separator = " "

// The maximum duration after which a connection get reset.
var connLifetime = time.Hour

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
	prefixer            *prefixer
	delimiter           Delimiter
	connManager         *ConnectionManager
	destinationsContext *DestinationsContext
	conn                net.Conn
	connExpirationTime  time.Time
	inputChan           chan []byte
	once                sync.Once
}

// NewDestination returns a new destination.
func NewDestination(endpoint Endpoint, destinationsContext *DestinationsContext) *Destination {
	return &Destination{
		prefixer:            newPrefixer(endpoint.APIKey + " "),
		delimiter:           NewDelimiter(endpoint.UseProto),
		connManager:         NewConnectionManager(endpoint),
		destinationsContext: destinationsContext,
	}
}

// Send transforms a message into a frame and sends it to a remote server,
// returns an error if the operation failed.
func (d *Destination) Send(payload []byte) error {
	if d.isConnectionExpired() {
		// reset the connection to make sure the load is evenly spread
		// and the agent can target new nodes.
		d.closeConnection()
	}
	if d.conn == nil {
		var err error

		// We work only if we have a started destination context
		ctx := d.destinationsContext.Context()
		if d.conn, err = d.connManager.NewConnection(ctx); err != nil {
			return err
		}

		d.connExpirationTime = time.Now().Add(connLifetime)
	}

	content := d.prefixer.apply(payload)
	frame, err := d.delimiter.delimit(content)
	if err != nil {
		return NewFramingError(err)
	}

	_, err = d.conn.Write(frame)
	if err != nil {
		d.closeConnection()
		return err
	}

	return nil
}

// SendAsync sends a message to the destination without blocking,
// if the channel is full, the incoming messages will be dropped.
func (d *Destination) SendAsync(payload []byte) {
	host := d.connManager.endpoint.Host
	d.once.Do(func() {
		inputChan := make(chan []byte, chanSize)
		d.inputChan = inputChan
		metrics.DestinationLogsDropped.Set(host, &expvar.Int{})
		go d.runAsync()
	})

	select {
	case d.inputChan <- payload:
	default:
		// TODO: Display the warning in the status
		if metrics.DestinationLogsDropped.Get(host).(*expvar.Int).Value()%warningPeriod == 0 {
			log.Warnf("Some logs sent to additional destination %v were dropped", host)
		}
		metrics.DestinationLogsDropped.Add(host, 1)
	}
}

// runAsync read the messages from the channel and send them
func (d *Destination) runAsync() {
	ctx := d.destinationsContext.Context()
	for {
		select {
		case payload := <-d.inputChan:
			d.Send(payload)
		case <-ctx.Done():
			return
		}
	}
}

// closeConnection closes the connection.
func (d *Destination) closeConnection() {
	d.connManager.CloseConnection(d.conn)
	d.conn = nil
	d.connExpirationTime = time.Time{}
}

// isConnectionExpired returns true if the connection is expired,
// i.e. if it has been created a long time ago.
func (d *Destination) isConnectionExpired() bool {
	return d.connExpirationTime != time.Time{} && d.connExpirationTime.Before(time.Now())
}
