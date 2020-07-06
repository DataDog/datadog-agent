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

/*
* Traps pipeline diagram
* ----------------------
*
* (devices) --[udp:port1]-!-> (Listener 1) \
*                         !                 (TrapServer) --[chan]--> (Forwarder) --[chan]--> (Rest of the logs pipeline...)
* (devices) --[udp:portN]-!-> (Listener N) /
*
* ! = delimitation between the outside world and the Agent
 */

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content *gosnmp.SnmpPacket
	Addr    *net.UDPAddr
}

// OutputChannel is the type of the server output channel.
type OutputChannel = chan *SnmpPacket

// TrapServer runs multiple SNMP traps listeners.
type TrapServer struct {
	Started bool

	listeners   []TrapListener
	output      OutputChannel
	stopTimeout time.Duration
}

var (
	outputChannelSize = 100
	// RunningServer holds a reference to the trap server instance running in the Agent.
	RunningServer *TrapServer
)

// NewTrapServer configures and returns a running SNMP traps server.
func NewTrapServer() (*TrapServer, error) {
	var configs []TrapListenerConfig
	err := config.Datadog.UnmarshalKey("snmp_traps_listeners", &configs)
	if err != nil {
		return nil, err
	}

	defaultBindHost := config.Datadog.GetString("bind_host")
	stopTimeout := config.Datadog.GetDuration("snmp_traps_stop_timeout") * time.Second

	output := make(OutputChannel, outputChannelSize)
	listeners := make([]TrapListener, 0, len(configs))

	for _, c := range configs {
		bindHost := c.BindHost
		if bindHost == "" {
			bindHost = defaultBindHost
		}
		listener, err := NewTrapListener(bindHost, c, output)
		if err != nil {
			return nil, err
		}

		listeners = append(listeners, *listener)
	}

	s := &TrapServer{
		Started:     false,
		listeners:   listeners,
		output:      output,
		stopTimeout: stopTimeout,
	}

	err = s.start()
	if err != nil {
		return nil, err
	}

	return s, nil
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

// Output returns the channel which listeners feed trap packets to.
func (s *TrapServer) Output() OutputChannel {
	return s.output
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

	// Let consumers know that we will not be sending any more packets.
	close(s.output)

	s.Started = false
}
