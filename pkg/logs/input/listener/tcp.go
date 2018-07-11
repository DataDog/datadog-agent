// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// A TCPListener listens and accepts TCP connections and delegates the read operations to a reader.
type TCPListener struct {
	pp       pipeline.Provider
	source   *config.LogSource
	listener net.Listener
	readers  []*Reader
	stop     chan struct{}
	mu       sync.Mutex
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pp pipeline.Provider, source *config.LogSource) *TCPListener {
	return &TCPListener{
		pp:      pp,
		source:  source,
		readers: []*Reader{},
		stop:    make(chan struct{}),
	}
}

// Start starts the listener to accepts new incoming connections.
func (l *TCPListener) Start() {
	log.Infof("Starting TCP forwarder on port %d", l.source.Config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		log.Errorf("Can't listen on port %d: %v", l.source.Config.Port, err)
		l.source.Status.Error(err)
		return
	}
	l.listener = listener
	go l.run()
}

// Stop stops the listener from accepting new connections and all the activer readers.
func (l *TCPListener) Stop() {
	log.Infof("Stopping TCP forwarder on port %d", l.source.Config.Port)
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, reader := range l.readers {
		stopper.Add(reader)
	}
	stopper.Stop()
}

// run accepts new TCP connections and create a dedicated reader for each one.
func (l *TCPListener) run() {
	defer l.listener.Close()
	for {
		select {
		case <-l.stop:
			// stop accepting new connections
			return
		default:
			conn, err := l.listener.Accept()
			if err != nil {
				// an error occurred, restart the listener.
				log.Warnf("Can't list on port %d, restarting a listener: %v", err)
				l.listener.Close()
				listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
				if err != nil {
					log.Errorf("Can't restart listener on port %d: %v", l.source.Config.Port, err)
					l.source.Status.Error(err)
					return
				}
				l.listener = listener
				continue
			}
			reader := l.newReader(conn)
			reader.Start()
			l.add(reader)
		}
	}
}

// newReader returns a new reader that reads from conn.
func (l *TCPListener) newReader(conn net.Conn) *Reader {
	return NewReader(l.source, conn, l.pp.NextPipelineChan(), l.handleUngracefulStop)
}

// handleUngracefulStop stops the reader.
func (l *TCPListener) handleUngracefulStop(reader *Reader) {
	reader.Stop()
	l.remove(reader)
	l.source.Status.Success()
}

// add adds the reader to the active list of readers.
func (l *TCPListener) add(reader *Reader) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.readers = append(l.readers, reader)
}

// remove removes the reader from the active list of readers.
func (l *TCPListener) remove(reader *Reader) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, t := range l.readers {
		if t == reader {
			l.readers = append(l.readers[:i], l.readers[i+1:]...)
			break
		}
	}
}
