// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package events

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/proto/api"
	"github.com/DataDog/datadog-agent/pkg/process/events/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SysProbeListener collects process events using the event monitoring module in system-probe
type SysProbeListener struct {
	// client holds a gRPC client to connect to system-probe
	client api.EventMonitoringModuleClient
	// conn holds the connection used by the client, so it can be closed once the listener is stopped
	conn *grpc.ClientConn
	// retryInterval is how long the listener will wait before trying to reconnect to system-probe if there's a connection failure
	retryInterval time.Duration
	// handler is the EventHandler function applied to every event collected by the listener
	handler EventHandler
	// connected holds the status of the connection to system-probe
	connected atomic.Value
	// wg and exit are used to control the main listener routine
	wg   sync.WaitGroup
	exit chan struct{}
}

// NewListener returns a new SysProbeListener to listen for process events
func NewListener(handler EventHandler) (*SysProbeListener, error) {
	socketPath := ddconfig.SystemProbe.GetString("event_monitoring_config.socket")
	if socketPath == "" {
		return nil, errors.New("event_monitoring_config.socket must be set")
	}

	conn, err := grpc.Dial(socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, url string) (net.Conn, error) {
		return net.Dial("unix", url)
	}))
	if err != nil {
		return nil, err
	}

	client := api.NewEventMonitoringModuleClient(conn)
	return NewSysProbeListener(conn, client, handler)
}

// NewSysProbeListener returns a new SysPobeListener
func NewSysProbeListener(conn *grpc.ClientConn, client api.EventMonitoringModuleClient, handler EventHandler) (*SysProbeListener, error) {
	if handler == nil {
		return nil, errors.New("can't create a Listener without an EventHandler")
	}

	return &SysProbeListener{
		client:        client,
		conn:          conn,
		retryInterval: 2 * time.Second,
		handler:       handler,
		exit:          make(chan struct{}),
	}, nil
}

// Run starts a new thread to listen for process events
func (l *SysProbeListener) Run() {
	log.Info("Start listening for process events")

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.run()
	}()
}

// run keeps polling the event monitoring module process events consumer for process events
func (l *SysProbeListener) run() {
	l.connected.Store(false)
	logTicker := newLogBackoffTicker()

	running := true
	for running {
		select {
		case <-l.exit:
			running = false
			continue
		default:
			stream, err := l.client.GetProcessEvents(context.Background(), &api.GetProcessEventParams{TimeoutSeconds: 1})
			if err != nil {
				l.connected.Store(false)

				select {
				case <-logTicker.C:
					log.Warnf("Error while connecting to the runtime-security module: %v", err)
				default:
					// do nothing
				}

				time.Sleep(l.retryInterval)
				continue
			}

			if l.connected.Load() != true {
				l.connected.Store(true)

				log.Info("Successfully connected to the event monitoring module")
			}

			readStream := true
			for readStream {
				select {
				// If an exit signal is sent, stop consuming from the stream and return
				case <-l.exit:
					readStream = false
					running = false
					continue
				default:
					in, err := stream.Recv()
					if err == io.EOF || in == nil {
						readStream = false
						continue
					}
					l.consumeData(in.Data)
				}
			}
		}
	}

	log.Info("Listener stopped")
	logTicker.Stop()
}

// consumeData unmarshals the serialized process event received from the event monitoring module, filters it and applies the
// EventHandler
func (l *SysProbeListener) consumeData(data []byte) {
	var sysEvent model.ProcessEvent
	if _, err := sysEvent.UnmarshalMsg(data); err != nil {
		log.Errorf("Could not unmarshal process event: %v", err)
		return
	}

	// Only consume expected process events
	switch sysEvent.EventType {
	case model.Exec, model.Exit:
		if l.handler == nil {
			log.Error("No EventHandler set to consume event, dropping it")
			return
		}
		l.handler(&sysEvent)
	default: // drop unexpected event
	}
}

// Stop stops the thread listening for process events
func (l *SysProbeListener) Stop() {
	log.Info("Stopping listening for process events")
	close(l.exit)
	l.wg.Wait()
	// conn may be nil during tests
	if l.conn != nil {
		l.conn.Close()
	}
}

// newLogBackoffTicker returns a ticker based on an exponential backoff, used to trigger connection error logs
func newLogBackoffTicker() *backoff.Ticker {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.MaxElapsedTime = 0
	expBackoff.Reset()
	return backoff.NewTicker(expBackoff)
}
