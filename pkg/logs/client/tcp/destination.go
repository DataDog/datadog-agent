// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Destination is responsible for shipping logs to a remote server over TCP.
type Destination struct {
	prefixer            *prefixer
	delimiter           Delimiter
	connManager         *ConnectionManager
	destinationsContext *client.DestinationsContext
	conn                net.Conn
	connCreationTime    time.Time
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

func (d *Destination) ErrorStateChangeChan() chan bool {
	return nil
}

// Send transforms a message into a frame and sends it to a remote server,
// returns an error if the operation failed.
func (d *Destination) Start(payload chan []byte, hasError chan bool) {
	go func() {
		for p := range payload {
			// TODO retry
			if d.conn == nil {
				var err error

				// We work only if we have a started destination context
				ctx := d.destinationsContext.Context()
				if d.conn, err = d.connManager.NewConnection(ctx); err != nil {
					// the connection manager is not meant to fail,
					// this can happen only when the context is cancelled.
					return // err
				}
				d.connCreationTime = time.Now()
			}

			metrics.BytesSent.Add(int64(len(p)))
			metrics.TlmBytesSent.Add(float64(len(p)))
			metrics.EncodedBytesSent.Add(int64(len(p)))
			metrics.TlmEncodedBytesSent.Add(float64(len(p)))

			content := d.prefixer.apply(p)
			frame, err := d.delimiter.delimit(content)
			if err != nil {
				// the delimiter can fail when the payload can not be framed correctly.
				return //err
			}

			_, err = d.conn.Write(frame)
			if err != nil {
				d.connManager.CloseConnection(d.conn)
				d.conn = nil
				return // client.NewRetryableError(err)
			}

			if d.connManager.ShouldReset(d.connCreationTime) {
				log.Debug("Resetting TCP connection")
				d.connManager.CloseConnection(d.conn)
				d.conn = nil
			}
		}
	}()
}
