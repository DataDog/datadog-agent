// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"context"
	"expvar"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
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
	shouldRetry         bool
	retryLock           sync.Mutex
	lastRetryError      error
	isMRF               bool
}

// NewDestination returns a new destination.
func NewDestination(endpoint config.Endpoint, useProto bool, destinationsContext *client.DestinationsContext, shouldRetry bool, status statusinterface.Status) *Destination {
	metrics.DestinationLogsDropped.Set(endpoint.Host, &expvar.Int{})
	return &Destination{
		prefixer:            newPrefixer(endpoint.GetAPIKey),
		delimiter:           NewDelimiter(useProto),
		connManager:         NewConnectionManager(endpoint, status),
		destinationsContext: destinationsContext,
		retryLock:           sync.Mutex{},
		shouldRetry:         shouldRetry,
		lastRetryError:      nil,
		isMRF:               endpoint.IsMRF,
	}
}

// IsMRF indicates that this destination is a Multi-Region Failover destination.
func (d *Destination) IsMRF() bool {
	return d.isMRF
}

// Target is the address of the destination.
func (d *Destination) Target() string {
	return d.connManager.address()
}

// Metadata is not supported for TCP destinations
func (d *Destination) Metadata() *client.DestinationMetadata {
	return client.NewNoopDestinationMetadata()
}

// Start reads from the input, transforms a message into a frame and sends it to a remote server,
func (d *Destination) Start(input chan *message.Payload, output chan *message.Payload, isRetrying chan bool) (stopChan <-chan struct{}) {
	stop := make(chan struct{})
	go func() {
		for payload := range input {
			d.sendAndRetry(payload, output, isRetrying)
		}
		d.updateRetryState(nil, isRetrying)
		stop <- struct{}{}
	}()
	return stop
}

func (d *Destination) sendAndRetry(payload *message.Payload, output chan *message.Payload, isRetrying chan bool) {
	for {
		if d.conn == nil {
			var err error

			// We work only if we have a started destination context
			ctx := d.destinationsContext.Context()
			if d.conn, err = d.connManager.NewConnection(ctx); err != nil {
				// the connection manager is not meant to fail,
				// this can happen only when the context is cancelled.
				d.incrementErrors(true)
				return
			}
			d.connCreationTime = time.Now()
		}

		content := d.prefixer.apply(payload.Encoded)
		frame, err := d.delimiter.delimit(content)
		if err != nil {
			// the delimiter can fail when the payload can not be framed correctly.
			d.incrementErrors(true)
			return
		}

		_, err = d.conn.Write(frame)
		if err != nil {
			d.connManager.CloseConnection(d.conn)
			d.conn = nil

			if d.shouldRetry {
				d.updateRetryState(err, isRetrying)
				d.incrementErrors(false)
				// retry (will try to open a new connection)
				continue
			}
			d.incrementErrors(true)
			d.updateRetryState(nil, isRetrying)
			log.Debugf("Resetting TCP connection following write error: %s", err)
			return
		}

		d.updateRetryState(nil, isRetrying)

		metrics.LogsSent.Add(1)
		metrics.TlmLogsSent.Inc()
		metrics.BytesSent.Add(int64(payload.UnencodedSize))

		// TCP is only used for logs data, so we always use "logs" as the source tag
		sourceTag := "logs"
		// Default Compression for TCP is none
		compressionKind := "none"

		metrics.TlmBytesSent.Add(float64(payload.UnencodedSize), sourceTag)
		metrics.EncodedBytesSent.Add(int64(len(payload.Encoded)))
		metrics.TlmEncodedBytesSent.Add(float64(len(payload.Encoded)), sourceTag, compressionKind)
		output <- payload

		if d.connManager.ShouldReset(d.connCreationTime) {
			log.Debug("Resetting TCP connection")
			d.connManager.CloseConnection(d.conn)
			d.conn = nil
		}
		return
	}
}

func (d *Destination) incrementErrors(drop bool) {
	if drop {
		host := d.connManager.endpoint.Host
		metrics.DestinationLogsDropped.Add(host, 1)
		metrics.TlmLogsDropped.Inc(host)
	}
	metrics.DestinationErrors.Add(1)
	metrics.TlmDestinationErrors.Inc()
}

func (d *Destination) updateRetryState(err error, isRetrying chan bool) {
	d.retryLock.Lock()
	defer d.retryLock.Unlock()

	if err != nil {
		if isRetrying != nil && d.lastRetryError == nil {
			isRetrying <- true
		}
	} else {
		if isRetrying != nil && d.lastRetryError != nil {
			isRetrying <- false
		}
	}
	d.lastRetryError = err
}

// CheckConnectivityDiagnose is a diagnosis for TCP connections
func CheckConnectivityDiagnose(endpoint config.Endpoint, timeoutSeconds int) (url string, err error) {
	operationTimeout := time.Second * time.Duration(timeoutSeconds)
	connManager := NewConnectionManager(endpoint, statusinterface.NewNoopStatusProvider())
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()

	_, err = connManager.NewConnection(ctx)

	return fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port), err
}
