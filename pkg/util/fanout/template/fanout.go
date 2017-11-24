// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package fanout

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/fanout"
)

// Message is the placeholder type for your data
type Message string

// messageFanout holds the fan-out logic. It can either be embedded or used by itself
type messageFanout struct {
	sync.RWMutex
	config    fanout.Config
	dataInput chan Message
	stopChan  chan error
	listeners map[string]*messageOutput
	running   bool
}

// setup has to be called once and returns the input channels
func (f *messageFanout) setup(cfg fanout.Config) (chan<- Message, error) {
	if cfg.WriteTimeout.Nanoseconds() == 0 {
		return nil, errors.New("WriteTimeout must be higher than 0")
	}
	if cfg.OutputBufferSize == 0 {
		return nil, errors.New("OutputBufferSize must be higher than 0")
	}
	if cfg.Name == "" {
		return nil, errors.New("Name can't be empty")
	}

	f.Lock()
	defer f.Unlock()

	f.config = cfg
	f.dataInput = make(chan Message)
	f.stopChan = make(chan error, 1)
	f.listeners = make(map[string]*messageOutput)

	return f.dataInput, nil
}

// StopOnEOF will trigger the Stop logic, unsuscribing all listeners in the process
func (f *messageFanout) StopOnEOF() {
	f.StopOnError(io.EOF)
}

// StopOnError will trigger the Stop logic, unsuscribing all listeners in the process
func (f *messageFanout) StopOnError(err error) {
	f.stopChan <- err
}

// SuscribeChannel adds a new suscriber to the fanout. If it's the first, the dispatching goroutine starts
func (f *messageFanout) SuscribeChannel(name string) (<-chan Message, <-chan error, error) {
	f.Lock()
	defer f.Unlock()

	if _, found := f.listeners[name]; found {
		return nil, nil, fmt.Errorf("listener %s is already suscribed to %s", name, f.config.Name)
	}

	out := &messageOutput{
		dataOutput:   make(chan Message, f.config.OutputBufferSize),
		errorOutput:  make(chan error, 1),
		writeTimeout: f.config.WriteTimeout,
	}
	f.listeners[name] = out

	if !f.running {
		f.running = true
		go f.dispatch()
	}

	return out.dataOutput, out.errorOutput, nil
}

// TODO: add SubscribeCallback when needed

// Unsuscribe removes a suscriber from the fanout with the EOF error. If it's the last, the dispatching goroutine stops and we return true
func (f *messageFanout) Unsuscribe(name string) (bool, error) {
	return f.UnsuscribeWithError(name, io.EOF)
}

// UnsuscribeWithError removes a suscriber from the fanout with a custom error. If it's the last, the dispatching goroutine stops and we return true
func (f *messageFanout) UnsuscribeWithError(name string, err error) (bool, error) {
	f.Lock()
	defer f.Unlock()

	if _, found := f.listeners[name]; !found {
		return false, fmt.Errorf("listener %s is not suscribed to %s", name, f.config.Name)
	}
	f.listeners[name].close(err)
	delete(f.listeners, name)

	if f.running && len(f.listeners) == 0 {
		f.StopOnEOF()
		return true, nil
	}

	return false, nil
}

// dispatch is the business logic goroutine
func (f *messageFanout) dispatch() {
	badListeners := make(map[string]error)

	// Outer loop handles unsuscribing unresponsive listeners
	for {
	TRANSMIT: // Inner loop handles communication, breaks on write timeouts
		for {
			select {
			case err := <-f.stopChan:
				f.Lock()
				for name, output := range f.listeners {
					output.close(err)
					delete(f.listeners, name)
				}
				f.running = false
				f.Unlock()
				return
			case data := <-f.dataInput:
				f.RLock()
				for name, output := range f.listeners {
					err := output.sendData(data)
					if err != nil {
						badListeners[name] = err
					}
				}
				f.RUnlock()
				break TRANSMIT
			}
		}
		if len(badListeners) == 0 {
			continue
		}
		for name, err := range badListeners {
			log.Infof("forcefully unsuscribing %s from %s: %s", name, f.config.Name, err)
			f.UnsuscribeWithError(name, err)
		}
		badListeners = make(map[string]error)
	}
}

// messageOutput holds the output channels for suscriber
type messageOutput struct {
	dataOutput   chan Message
	errorOutput  chan error
	writeTimeout time.Duration
}

func (o *messageOutput) sendData(data Message) error {
	select {
	case o.dataOutput <- data:
		return nil
	case <-time.After(o.writeTimeout):
		return fanout.ErrTimeout
	}
}

func (o *messageOutput) sendError(err error) error {
	select {
	case o.errorOutput <- err:
		return nil
	case <-time.After(o.writeTimeout):
		return fanout.ErrTimeout
	}
}

func (o *messageOutput) close(err error) {
	o.sendError(err)
	close(o.dataOutput)
	close(o.errorOutput)
}
