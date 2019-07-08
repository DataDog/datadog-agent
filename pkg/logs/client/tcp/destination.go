// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

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

// FIXME: Changed chanSize to a constant once we refactor packages
const (
	chanSize      = 100
	warningPeriod = 1000
)

// Destination is responsible for shipping logs to a remote server over TCP.
type Destination struct {
	prefixer            *prefixer
	delimiter           Delimiter
	connManager         *ConnectionManager
	destinationsContext *client.DestinationsContext
	connPool            sync.Pool
	inputChan           chan []byte
	once                sync.Once
	warningCounter      int
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
	var err error

	content := d.prefixer.apply(payload)
	frame, err := d.delimiter.delimit(content)
	if err != nil {
		return client.NewFramingError(err)
	}

	// reuse an existing connection from the pool or
	// create a new one if none is available, this can
	// happen when this method is called concurrently
	// from different places ; in such a case,
	// the size of the pool will be the maximum size
	// of concurrent calls and objects that are not used
	// will be automatically deallocated
	conn, available := d.connPool.Get().(net.Conn)
	if !available {
		ctx := d.destinationsContext.Context()
		conn, err = d.connManager.NewConnection(ctx)
		if err != nil {
			return err
		}
	}

	_, err = conn.Write(frame)
	if err != nil {
		d.connManager.CloseConnection(conn)
		return err
	}

	// make sure the connection is put back to the pool
	// for later reuse
	d.connPool.Put(conn)

	return nil
}

// SendAsync sends a message to the destination without blocking. If the channel is full, the incoming messages will be
// dropped
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
