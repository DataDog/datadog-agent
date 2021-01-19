// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"google.golang.org/grpc"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	hostname      string
	reporter      event.Reporter
	conn          *grpc.ClientConn
	running       atomic.Value
	wg            sync.WaitGroup
	connected     atomic.Value
	eventReceived uint64
}

// NewRuntimeSecurityAgent instantiates a new RuntimeSecurityAgent
func NewRuntimeSecurityAgent(hostname string, reporter event.Reporter) (*RuntimeSecurityAgent, error) {
	socketPath := coreconfig.Datadog.GetString("runtime_security_config.socket")
	if socketPath == "" {
		return nil, errors.New("runtime_security_config.socket must be set")
	}

	path := "unix://" + socketPath
	conn, err := grpc.Dial(path, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return &RuntimeSecurityAgent{
		conn:     conn,
		reporter: reporter,
		hostname: hostname,
	}, nil
}

// Start the runtime security agent
func (rsa *RuntimeSecurityAgent) Start() {
	// Start the system-probe events listener
	go rsa.StartEventListener()
}

// Stop the runtime recurity agent
func (rsa *RuntimeSecurityAgent) Stop() {
	rsa.running.Store(false)
	rsa.wg.Wait()
	rsa.conn.Close()
}

// StartEventListener starts listening for new events from system-probe
func (rsa *RuntimeSecurityAgent) StartEventListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()
	apiClient := api.NewSecurityModuleClient(rsa.conn)

	rsa.connected.Store(false)

	rsa.running.Store(true)
	for rsa.running.Load() == true {
		stream, err := apiClient.GetEvents(context.Background(), &api.GetParams{})
		if err != nil {
			rsa.connected.Store(false)

			log.Warnf("Error while connecting to the runtime security module: %v", err)

			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		if rsa.connected.Load() != true {
			rsa.connected.Store(true)

			log.Info("Successfully connected to the runtime security module")
		}

		for {
			// Get new event from stream
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}
			log.Infof("Got message from rule `%s` for event `%s`", in.RuleID, string(in.Data))

			atomic.AddUint64(&rsa.eventReceived, 1)

			// Dispatch security event
			rsa.DispatchEvent(in)
		}
	}
}

// DispatchEvent dispatches a security event message to the subsytems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	// For now simply log to Datadog
	rsa.reporter.ReportRaw(evt.GetData(), evt.GetTags()...)
}

// GetStatus returns the current status on the agent
func (rsa *RuntimeSecurityAgent) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"connected":     rsa.connected.Load(),
		"eventReceived": atomic.LoadUint64(&rsa.eventReceived),
	}
}
