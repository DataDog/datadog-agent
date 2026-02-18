// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/socket"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// A TCPListener listens and accepts TCP connections and delegates the read
// operations to a StreamTailer. The source's Format field controls whether
// syslog or unstructured parsing is used.
type TCPListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	idleTimeout      time.Duration
	frameSize        int
	listener         net.Listener
	tailers          []startstop.StartStoppable
	mu               sync.Mutex
	stop             chan struct{}
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pipelineProvider pipeline.Provider, source *sources.LogSource, frameSize int) *TCPListener {
	var idleTimeout time.Duration
	if source.Config.IdleTimeout != "" {
		var err error
		idleTimeout, err = time.ParseDuration(source.Config.IdleTimeout)
		if err != nil {
			log.Errorf("Error parsing log's idle_timeout as a duration: %s", err)
			idleTimeout = 0
		}
	}

	return &TCPListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		idleTimeout:      idleTimeout,
		frameSize:        frameSize,
		tailers:          []startstop.StartStoppable{},
		stop:             make(chan struct{}, 1),
	}
}

// Start starts the listener to accepts new incoming connections.
func (l *TCPListener) Start() {
	log.Infof("Starting TCP forwarder on port %d, with read buffer size: %d", l.source.Config.Port, l.frameSize)
	err := l.startListener()
	if err != nil {
		log.Errorf("Can't start TCP forwarder on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	go l.run()
}

// Stop stops the listener from accepting new connections and all the activer tailers.
func (l *TCPListener) Stop() {
	log.Infof("Stopping TCP forwarder on port %d", l.source.Config.Port)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stop <- struct{}{}
	if l.listener != nil {
		l.listener.Close()
	}
	stopper := startstop.NewParallelStopper()
	for _, t := range l.tailers {
		stopper.Add(t)
	}
	stopper.Stop()

	// At this point all the tailers have been stopped - remove them all from the active tailer list
	l.tailers = []startstop.StartStoppable{}
}

// run accepts new TCP connections and create a dedicated tailer for each.
func (l *TCPListener) run() {
	defer l.listener.Close()
	for {
		select {
		case <-l.stop:
			// stop accepting new connections.
			return
		default:
			conn, err := l.listener.Accept()
			switch {
			case err != nil && isClosedConnError(err):
				return
			case err != nil:
				// an error occurred, restart the listener.
				log.Warnf("Can't listen on port %d, restarting a listener: %v", l.source.Config.Port, err)
				l.listener.Close()
				err := l.startListener()
				if err != nil {
					log.Errorf("Can't restart listener on port %d: %v", l.source.Config.Port, err)
					l.source.Status.Error(err)
					return
				}
				l.source.Status.Success()
				continue
			default:
				l.startTailer(conn)
				l.source.Status.Success()
			}
		}
	}
}

// startListener starts a new listener, returns an error if it failed.
func (l *TCPListener) startListener() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return err
	}
	l.listener = listener
	return nil
}

// startTailer creates and starts a StreamTailer for the connection.
func (l *TCPListener) startTailer(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()

	outputChan := l.pipelineProvider.NextPipelineChan()
	sourceHostAddr := extractIPFromAddr(conn.RemoteAddr().String())

	t := tailer.NewStreamTailer(
		l.source,
		conn,
		outputChan,
		l.source.Config.Format,
		l.frameSize,
		l.idleTimeout,
		sourceHostAddr,
	)
	l.tailers = append(l.tailers, t)
	t.Start()
}

// stopTailer stops the tailer.
func (l *TCPListener) stopTailer(t startstop.StartStoppable) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, active := range l.tailers {
		if active == t {
			t.Stop()
			l.tailers = slices.Delete(l.tailers, i, i+1)
			break
		}
	}
}

// extractIPFromAddr strips the port from an address string (e.g. "1.2.3.4:5678" -> "1.2.3.4").
func extractIPFromAddr(addr string) string {
	lastColon := strings.LastIndex(addr, ":")
	if lastColon != -1 {
		return addr[:lastColon]
	}
	return addr
}
