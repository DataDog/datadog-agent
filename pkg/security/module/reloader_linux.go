// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Reloader aims to handle policies reloading triggers
type Reloader struct {
	wg         sync.WaitGroup
	ctx        context.Context
	cancelFnc  context.CancelFunc
	sighupChan chan os.Signal
	reloadChan chan struct{}
}

// NewReloader returns a new Reloader
func NewReloader() *Reloader {
	ctx, cancelFnc := context.WithCancel(context.Background())

	return &Reloader{
		sighupChan: make(chan os.Signal, 1),
		reloadChan: make(chan struct{}, 1),
		ctx:        ctx,
		cancelFnc:  cancelFnc,
	}
}

// Start the reloader
func (r *Reloader) Start() error {
	signal.Notify(r.sighupChan, syscall.SIGHUP)

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		for {
			select {
			case <-r.sighupChan:
				r.reloadChan <- struct{}{}
			case <-r.ctx.Done():
				return
			}
		}
	}()

	return nil
}

// Chan returns the chan of reload events
func (r *Reloader) Chan() <-chan struct{} {
	return r.reloadChan
}

// Stop the Reloader
func (r *Reloader) Stop() {
	signal.Stop(r.sighupChan)

	r.cancelFnc()
	r.wg.Wait()

	close(r.sighupChan)
	close(r.reloadChan)
}
