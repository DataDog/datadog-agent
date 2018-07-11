// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// A TCPListener listens and accepts TCP connections and delegates the work to connHandler
type TCPListener struct {
	pp       pipeline.Provider
	source   *config.LogSource
	listener net.Listener
	workers  []*Worker
	stop     chan struct{}
}

// NewTCPListener returns an initialized TCPListener
func NewTCPListener(pp pipeline.Provider, source *config.LogSource) *TCPListener {
	return &TCPListener{
		pp:      pp,
		source:  source,
		workers: []*Worker{},
		stop:    make(chan struct{}),
	}
}

// Start listens to TCP connections on another routine
func (l *TCPListener) Start() {
	log.Info("Starting TCP forwarder on port ", l.source.Config.Port)
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
	if err != nil {
		log.Errorf("Can't listen: %v", err)
		l.source.Status.Error(err)
		return
	}
	l.listener = listener
	go l.run()
}

// Stop prevents the listener to accept new incoming connections
// it blocks until connHandler is flushed
func (l *TCPListener) Stop() {
	log.Info("Stopping TCP forwarder on port ", l.source.Config.Port)
	l.stop <- struct{}{}
	stopper := restart.NewParallelStopper()
	for _, worker := range l.workers {
		stopper.Add(worker)
	}
	stopper.Stop()
}

// run accepts new TCP connections and lets connHandler handle them
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
				log.Warnf("Can't accept new connections, restarting a listener: %v", err)
				l.listener.Close()
				listener, err := net.Listen("tcp", fmt.Sprintf(":%d", l.source.Config.Port))
				if err != nil {
					log.Errorf("Can't restart a listener: %v", err)
					l.source.Status.Error(err)
					return
				}
				l.listener = listener
				continue
			}
			worker := l.newWorker(conn)
			worker.Start()
			l.workers = append(l.workers, worker)
		}
	}
}

// newWorker returns a new worker that reads from conn.
func (l *TCPListener) newWorker(conn net.Conn) *Worker {
	return NewWorker(l.source, conn, l.pp.NextPipelineChan(), false, l.recoverFromError)
}

// recoverFromError stops the worker.
func (l *TCPListener) recoverFromError(worker *Worker) {
	log.Info("Stopping a worker")
	worker.Stop()
	for i, w := range l.workers {
		if w == worker {
			l.workers = append(l.workers[:i], l.workers[i+1:]...)
			break
		}
	}
}
