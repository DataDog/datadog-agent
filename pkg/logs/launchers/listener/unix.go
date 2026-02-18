// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listener

import (
	"net"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/socket"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

// A UnixStreamListener listens on a Unix domain stream socket and delegates
// read operations to per-connection StreamTailers. The source's Format field
// controls whether syslog or unstructured parsing is used.
type UnixStreamListener struct {
	pipelineProvider pipeline.Provider
	source           *sources.LogSource
	idleTimeout      time.Duration
	frameSize        int
	listener         net.Listener
	tailers          []startstop.StartStoppable
	mu               sync.Mutex
	stop             chan struct{}
}

// NewUnixStreamListener returns an initialized UnixStreamListener.
func NewUnixStreamListener(pipelineProvider pipeline.Provider, source *sources.LogSource, frameSize int) *UnixStreamListener {
	var idleTimeout time.Duration
	if source.Config.IdleTimeout != "" {
		var err error
		idleTimeout, err = time.ParseDuration(source.Config.IdleTimeout)
		if err != nil {
			log.Errorf("Error parsing unix socket idle_timeout as a duration: %s", err)
			idleTimeout = 0
		}
	}

	return &UnixStreamListener{
		pipelineProvider: pipelineProvider,
		source:           source,
		idleTimeout:      idleTimeout,
		frameSize:        frameSize,
		tailers:          []startstop.StartStoppable{},
		stop:             make(chan struct{}, 1),
	}
}

// Start creates the Unix socket file and begins accepting connections.
func (l *UnixStreamListener) Start() {
	log.Infof("Starting Unix stream listener on %s", l.source.Config.SocketPath)
	err := l.startListener()
	if err != nil {
		log.Errorf("Can't start Unix stream listener on %s: %v", l.source.Config.SocketPath, err)
		l.source.Status.Error(err)
		return
	}
	l.source.Status.Success()
	go l.run()
}

// Stop stops the listener and all active tailers, then cleans up the socket file.
func (l *UnixStreamListener) Stop() {
	log.Infof("Stopping Unix stream listener on %s", l.source.Config.SocketPath)
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
	l.tailers = []startstop.StartStoppable{}
	// Clean up the socket file
	os.Remove(l.source.Config.SocketPath) //nolint:errcheck
}

// run accepts new connections and creates a tailer for each.
func (l *UnixStreamListener) run() {
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
				log.Warnf("Can't accept Unix stream connection on %s, restarting listener: %v", l.source.Config.SocketPath, err)
				l.listener.Close()
				err := l.startListener()
				if err != nil {
					log.Errorf("Can't restart Unix stream listener on %s: %v", l.source.Config.SocketPath, err)
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

// startListener removes any stale socket file and binds a new Unix stream listener.
func (l *UnixStreamListener) startListener() error {
	// Remove stale socket file if it exists
	os.Remove(l.source.Config.SocketPath) //nolint:errcheck

	listener, err := net.Listen("unix", l.source.Config.SocketPath)
	if err != nil {
		return err
	}
	l.listener = listener
	return nil
}

// startTailer creates and starts a StreamTailer for the connection.
func (l *UnixStreamListener) startTailer(conn net.Conn) {
	l.mu.Lock()
	defer l.mu.Unlock()

	outputChan := l.pipelineProvider.NextPipelineChan()

	t := tailer.NewStreamTailer(
		l.source,
		conn,
		outputChan,
		l.source.Config.Format,
		l.frameSize,
		l.idleTimeout,
		"", // no source_host for Unix sockets
	)
	t.SetOnDone(func() { l.removeTailer(t) })
	l.tailers = append(l.tailers, t)
	t.Start()
}

// removeTailer removes a finished tailer from the active list.
// Called by the tailer's onDone callback when readLoop exits.
func (l *UnixStreamListener) removeTailer(t startstop.StartStoppable) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, active := range l.tailers {
		if active == t {
			l.tailers = slices.Delete(l.tailers, i, i+1)
			break
		}
	}
}
