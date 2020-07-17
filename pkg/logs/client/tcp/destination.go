// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package tcp

import (
	"expvar"
	"net"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	warningPeriod = 1000
)

// Destination is responsible for shipping logs to a remote server over TCP.
type Destination struct {
	prefixer            *prefixer
	delimiter           Delimiter
	connManager         *ConnectionManager
	destinationsContext *client.DestinationsContext
	conn                net.Conn
	inputChan           chan []byte
	once                sync.Once
}

// NewDestination returns a new destination.
func NewDestination(endpoint config.Endpoint, useProto bool, destinationsContext *client.DestinationsContext) *Destination {
	prefix := endpoint.APIKey + string(' ')
	return &Destination{
		prefixer:            newPrefixer(prefix),
		delimiter:           NewDelimiter(useProto),
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
			// the connection manager is not meant to fail,
			// this can happen only when the context is cancelled.
			return err
		}
	}

	metrics.BytesSent.Add(int64(len(payload)))
	metrics.TlmBytesSent.Add(float64(len(payload)))
	metrics.EncodedBytesSent.Add(int64(len(payload)))
	metrics.TlmEncodedBytesSent.Add(float64(len(payload)))

	content := d.prefixer.apply(payload)
	frame, err := d.delimiter.delimit(content)
	if err != nil {
		// the delimiter can fail when the payload can not be framed correctly.
		return err
	}

	_, err = d.conn.Write(frame)
	if err != nil {
		d.connManager.CloseConnection(d.conn)
		d.conn = nil
		return client.NewRetryableError(err)
	}

	return nil
}

// SendAsync sends a message to the destination without blocking. If the channel is full, the incoming messages will be
// dropped
func (d *Destination) SendAsync(payload []byte) {
	host := d.connManager.endpoint.Host
	d.once.Do(func() {
		inputChan := make(chan []byte, config.ChanSize)
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
		metrics.TlmLogsDropped.Inc(host)
	}
}

// runAsync read the messages from the channel and send them
func (d *Destination) runAsync() {
	ctx := d.destinationsContext.Context()
	for {
		select {
		case payload := <-d.inputChan:
			d.Send(payload) //nolint:errcheck
		case <-ctx.Done():
			return
		}
	}
}
