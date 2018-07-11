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

// A TCPListener listens and accepts TCP connections and delegates the read operations to a tailer.
type TCPListener struct {
	pp       pipeline.Provider
	source   *config.LogSource
	listener net.Listener
	tailers  []*Tailer
	stop     chan struct{}
	mu       sync.Mutex
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pp pipeline.Provider, source *config.LogSource) *TCPListener {
	return &TCPListener{
		pp:      pp,
		source:  source,
		tailers: []*Tailer{},
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

// Stop stops the listener from accepting new connections and all the activer tailers.
func (l *TCPListener) Stop() {
	log.Infof("Stopping TCP forwarder on port %d", l.source.Config.Port)
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, tailer := range l.tailers {
		stopper.Add(tailer)
	}
	stopper.Stop()
}

// run accepts new TCP connections and create a dedicated tailer for each one.
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
			tailer := l.newTailer(conn)
			tailer.Start()
			l.add(tailer)
		}
	}
}

// newTailer returns a new tailer that reads from conn.
func (l *TCPListener) newTailer(conn net.Conn) *Tailer {
	return NewTailer(l.source, conn, l.pp.NextPipelineChan(), false, l.recoverFromError)
}

// recoverFromError stops the tailer.
func (l *TCPListener) recoverFromError(tailer *Tailer) {
	tailer.Stop()
	l.remove(tailer)
	l.source.Status.Success()
}

// add adds the tailer to the active list of tailers.
func (l *TCPListener) add(tailer *Tailer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tailers = append(l.tailers, tailer)
}

// remove removes the tailer from the active list of tailers.
func (l *TCPListener) remove(tailer *Tailer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, t := range l.tailers {
		if t == tailer {
			l.tailers = append(l.tailers[:i], l.tailers[i+1:]...)
			break
		}
	}
}
