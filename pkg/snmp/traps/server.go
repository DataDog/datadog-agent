// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"net"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

// TrapServer runs multiple SNMP traps listeners.
type TrapServer struct {
	Started bool

	bindHost    string
	listeners   []TrapListener
	stopTimeout time.Duration
}

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer() (*TrapServer, error) {
	var configs []TrapListenerConfig
	err := config.Datadog.UnmarshalKey("snmp_traps_listeners", &configs)
	if err != nil {
		return nil, err
	}

	bindHost := config.Datadog.GetString("bind_host")
	stopTimeout := config.Datadog.GetDuration("snmp_traps_stop_timeout") * time.Second

	listeners := make([]TrapListener, 0, len(configs))

	for _, c := range configs {
		listener, err := NewTrapListener(bindHost, c)
		if err != nil {
			return nil, err
		}

		listeners = append(listeners, *listener)
	}

	s := &TrapServer{
		Started:     false,
		bindHost:    bindHost,
		listeners:   listeners,
		stopTimeout: stopTimeout,
	}

	err = s.start()
	if err != nil {
		return nil, err
	}

	return s, nil
}

// SetTrapHandler sets the trap handler for all listeners. Useful for unit testing.
func (s TrapServer) SetTrapHandler(handler func(s *gosnmp.SnmpPacket, u *net.UDPAddr)) {
	for _, l := range s.listeners {
		l.SetTrapHandler(handler)
	}
}

// start spawns listeners in the background, and waits for them to be ready to accept traffic, handling any errors.
func (s *TrapServer) start() error {
	wg := new(sync.WaitGroup)
	allReady := make(chan struct{})
	readyErrors := make(chan error)

	wg.Add(len(s.listeners))

	for _, l := range s.listeners {
		l := l
		go l.Listen()
		go func() {
			defer wg.Done()
			err := l.WaitReadyOrError()
			if err != nil {
				readyErrors <- err
			}
		}()
	}

	go func() {
		wg.Wait()
		close(allReady)
	}()

	select {
	case <-allReady:
	case err := <-readyErrors:
		close(readyErrors)
		return err
	}

	s.Started = true

	return nil
}

// NumListeners returns the number of listeners managed by this server.
func (s *TrapServer) NumListeners() int {
	return len(s.listeners)
}

// Stop stops the TrapServer.
func (s *TrapServer) Stop() {
	wg := new(sync.WaitGroup)
	stopped := make(chan bool)

	wg.Add(len(s.listeners))

	for _, listener := range s.listeners {
		l := listener
		go func() {
			defer wg.Done()
			l.Stop()
		}()
	}

	go func() {
		wg.Wait()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(s.stopTimeout):
		log.Error("snmp-traps: stopping listeners timed out")
	}

	s.Started = false
}
