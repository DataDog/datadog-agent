// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package forwarder

import (
	"context"
	"net/http"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Worker comsumes Transaction (aka transactions) from the Forwarder and
// process them. If the transaction fail to be processed the Worker will send
// it back to the Forwarder to be retried later.
type Worker struct {
	// Client the http client used to processed transactions.
	Client *http.Client
	// InputChan is the channel used to receive transaction from the Forwarder.
	InputChan <-chan Transaction
	// RequeueChan is the channel used to send failed transaction back to the Forwarder.
	RequeueChan chan<- Transaction

	stopChan    chan bool
	blockedList *blockedEndpoints
}

// NewWorker returns a new worker to consume Transaction from inputChan
// and push back erroneous ones into requeueChan.
func NewWorker(inputChan chan Transaction, requeueChan chan Transaction, blocked *blockedEndpoints) *Worker {

	transport := util.CreateHTTPTransport()

	httpClient := &http.Client{
		Timeout:   config.Datadog.GetDuration("forwarder_timeout") * time.Second,
		Transport: transport,
	}

	return &Worker{
		InputChan:   inputChan,
		RequeueChan: requeueChan,
		stopChan:    make(chan bool),
		Client:      httpClient,
		blockedList: blocked,
	}
}

// Stop stops the worker.
func (w *Worker) Stop() {
	w.stopChan <- true
}

// Start starts a Worker.
func (w *Worker) Start() {
	go func() {
		for {
			select {
			case t := <-w.InputChan:
				ctx, cancel := context.WithCancel(context.Background())

				done := make(chan interface{})
				go func() {
					w.process(ctx, t)
					done <- nil
				}()

				select {
				case <-done:
					// wait for the Transaction process to be over
				case <-w.stopChan:
					// cancel current Transaction if we need to stop the worker
					cancel()
					return
				}
				cancel()
			case <-w.stopChan:
				return
			}
		}
	}()
}

func (w *Worker) process(ctx context.Context, t Transaction) {
	// First we check if we don't have recently received an error for that endpoint
	target := t.GetTarget()
	if w.blockedList.isBlock(target) {
		t.Reschedule()
		w.RequeueChan <- t
		log.Errorf("Too many errors for endpoint '%s': retrying later", target)
	} else if err := t.Process(ctx, w.Client); err != nil {
		w.blockedList.block(target)
		t.Reschedule()
		w.RequeueChan <- t
		log.Errorf("Error while processing transaction: %v", err)
	} else {
		w.blockedList.unblock(target)
	}
}
