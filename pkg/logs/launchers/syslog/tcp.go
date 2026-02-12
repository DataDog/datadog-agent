// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/syslog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// TCPListener listens for TCP connections and spawns a syslog Tailer per connection.
type TCPListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	idleTimeout      time.Duration
	listener         net.Listener
	tailers          []*tailer.Tailer
	mu               sync.Mutex
	stop             chan struct{}
}

// NewTCPListener returns an initialized syslog TCPListener.
func NewTCPListener(pipelineProvider pipeline.Provider, source *sources.LogSource) *TCPListener {
	var idleTimeout time.Duration
	if source.Config.IdleTimeout != "" {
		var err error
		idleTimeout, err = time.ParseDuration(source.Config.IdleTimeout)
		if err != nil {
			log.Errorf("Error parsing syslog idle_timeout as a duration: %s", err)
			idleTimeout = 0
		}
	}

	return &TCPListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		idleTimeout:      idleTimeout,
		tailers:          []*tailer.Tailer{},
		stop:             make(chan struct{}, 1),
	}
}

// Start begins listening for TCP connections.
func (l *TCPListener) Start() {
	log.Infof("Starting syslog TCP listener on port %d", l.source.Config.Port)
	err := l.startListener()
	if err != nil {
		log.Errorf("Can't start syslog TCP listener on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	go l.run()
}

// Stop stops the listener and all active tailers.
func (l *TCPListener) Stop() {
	log.Infof("Stopping syslog TCP listener on port %d", l.source.Config.Port)
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
	l.tailers = []*tailer.Tailer{}
}

// run accepts new TCP connections and creates a syslog tailer for each.
func (l *TCPListener) run() {
	defer l.listener.Close()
	for {
		select {
		case <-l.stop:
			return
		default:
			conn, err := l.listener.Accept()
			switch {
			case err != nil && isClosedConnError(err):
				return
			case err != nil:
				log.Warnf("Can't accept syslog connection on port %d, restarting listener: %v", l.source.Config.Port, err)
				l.listener.Close()
				err := l.startListener()
				if err != nil {
					log.Errorf("Can't restart syslog TCP listener on port %d: %v", l.source.Config.Port, err)
					l.source.Status.Error(err)
					return
				}
				l.source.Status.Success()
				continue
			default:
				if l.idleTimeout > 0 {
					conn.SetDeadline(time.Now().Add(l.idleTimeout)) //nolint:errcheck
				}
				l.startTailer(conn)
				l.source.Status.Success()
			}
		}
	}
}

// startListener binds the TCP listener socket.
func (l *TCPListener) startListener() error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		return err
	}
	l.listener = listener
	return nil
}

// startTailer creates and starts a new syslog tailer for the connection.
func (l *TCPListener) startTailer(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := tailer.NewTailer(l.source, l.pipelineProvider.NextPipelineChan(), conn)
	l.tailers = append(l.tailers, t)
	t.Start()
}

// stopTailer stops and removes a tailer from the active list.
func (l *TCPListener) stopTailer(t *tailer.Tailer) {
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

// isClosedConnError returns true if the error is related to a closed connection.
func isClosedConnError(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection")
}
