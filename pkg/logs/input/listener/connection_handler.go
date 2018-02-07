// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// checkPeriod defines the repeated period of time after which we check the state of the workers to stop them if needed
const checkPeriod = 60 * time.Second

// ConnectionHandler creates a worker for each new connection and releases the ones that must be stopped
type ConnectionHandler struct {
	pp       pipeline.Provider
	source   *config.LogSource
	connChan chan net.Conn
	workers  []*Worker
	done     chan struct{}
}

// NewConnectionHandler returns a new ConnectionHandler
func NewConnectionHandler(pp pipeline.Provider, source *config.LogSource) *ConnectionHandler {
	return &ConnectionHandler{
		pp:       pp,
		source:   source,
		connChan: make(chan net.Conn),
		workers:  []*Worker{},
		done:     make(chan struct{}),
	}
}

// Start starts the handler
func (h *ConnectionHandler) Start() {
	go h.run()
}

// Stop stops all the workers in parallel,
// this call returns only when connChan is flushed and all workers are stopped
func (h *ConnectionHandler) Stop() {
	close(h.connChan)
	<-h.done
	stopper := restart.NewParallelStopper()
	for _, worker := range h.workers {
		stopper.Add(worker)
	}
	stopper.Stop()
	h.workers = h.workers[:0]
}

// HandleConnection forwards the new connection to connChan
func (h *ConnectionHandler) HandleConnection(conn net.Conn) {
	h.connChan <- conn
}

// run creates workers for each new connection and check periodically if some should be stopped
func (h *ConnectionHandler) run() {
	checkTicker := time.NewTicker(checkPeriod)
	defer func() {
		// the connChan has successfully been flushed
		checkTicker.Stop()
		h.done <- struct{}{}
	}()
	for {
		select {
		case <-checkTicker.C:
			// stop workers that are inactive
			h.checkWorkers()
		case conn, isOpen := <-h.connChan:
			if !isOpen {
				// connChan has been closed, no need to create workers anymore
				return
			}
			// create a worker for the new connection
			h.createWorker(conn)
		}
	}
}

// createWorker initializes and starts a new worker for conn
func (h *ConnectionHandler) createWorker(conn net.Conn) {
	worker := NewWorker(h.source, conn, h.pp.NextPipelineChan())
	worker.Start()
	h.workers = append(h.workers, worker)
}

// checkWorkers stops all the workers that should be stopped
func (h *ConnectionHandler) checkWorkers() {
	activeWorkers := []*Worker{}
	for _, worker := range h.workers {
		if worker.shouldStop {
			worker.Stop()
		} else {
			activeWorkers = append(activeWorkers, worker)
		}
	}
	h.workers = activeWorkers
}
