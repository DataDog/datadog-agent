// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agent holds agent related files
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/golang/protobuf/ptypes/empty"
	"go.uber.org/atomic"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	statsdClient            statsd.ClientInterface
	hostname                string
	reporter                common.RawReporter
	client                  *RuntimeSecurityClient
	running                 *atomic.Bool
	wg                      sync.WaitGroup
	connected               *atomic.Bool
	eventReceived           *atomic.Uint64
	activityDumpReceived    *atomic.Uint64
	profContainersTelemetry *profContainersTelemetry
	endpoints               *config.Endpoints
	cancel                  context.CancelFunc

	// activity dump
	storage ADStorage

	// grpc server
	api.UnimplementedSecurityAgentAPIServer
	grpcServer *module.GRPCServer
	apiServer  api.SecurityAgentAPIServer
}

// ADStorage represents the interface for the activity dump storage
type ADStorage interface {
	backend.ActivityDumpHandler

	SendTelemetry(_ statsd.ClientInterface)
}

// RSAOptions represents the runtime security agent options
type RSAOptions struct {
	LogProfiledWorkloads bool
}

// SendEvent dispatches events to the backend
func (rsa *RuntimeSecurityAgent) SendEvent(stream grpc.ClientStreamingServer[api.SecurityEventMessage, empty.Empty]) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break // read done.
		}

		if err != nil {
			return err
		}

		if seclog.DefaultLogger.IsTracing() {
			seclog.DefaultLogger.Tracef("Got message from rule `%s` for event `%s`", msg.RuleID, string(msg.Data))
		}

		rsa.eventReceived.Inc()

		rsa.DispatchEvent(msg)
	}

	return nil
}

// SendActivityDumpStream dispatches activity dumps to the backend
func (rsa *RuntimeSecurityAgent) SendActivityDumpStream(stream grpc.ClientStreamingServer[api.ActivityDumpStreamMessage, empty.Empty]) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break // read done.
		}

		if err != nil {
			return err
		}

		if seclog.DefaultLogger.IsTracing() {
			seclog.DefaultLogger.Tracef("Got activity dump [%s]", msg.GetSelector())
		}

		rsa.activityDumpReceived.Inc()

		// Dispatch activity dump
		rsa.DispatchActivityDump(msg)
	}

	return nil
}

// Start the runtime security agent
func (rsa *RuntimeSecurityAgent) Start(reporter common.RawReporter, endpoints *config.Endpoints) {
	rsa.reporter = reporter
	rsa.endpoints = endpoints

	ctx, cancel := context.WithCancel(context.Background())
	rsa.cancel = cancel

	rsa.running.Store(true)

	if runtime.GOOS == "linux" {
		go rsa.startActivityDumpStorageTelemetry(ctx)
	}

	if rsa.profContainersTelemetry != nil {
		// Send Profiled Containers telemetry
		go rsa.profContainersTelemetry.run(ctx)
	}

	err := rsa.grpcServer.Start()
	if err != nil {
		seclog.Errorf("error starting security agent grpc server: %v", err)
	}
}

// Stop the runtime recurity agent
func (rsa *RuntimeSecurityAgent) Stop() {
	rsa.cancel()
	rsa.running.Store(false)
	rsa.client.Close()
	rsa.grpcServer.Stop()
	rsa.wg.Wait()
}

// DispatchEvent dispatches a security event message to the subsytems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	if rsa.reporter == nil {
		return
	}
	rsa.reporter.ReportRaw(evt.GetData(), evt.Service, evt.GetTags()...)
}

// DispatchActivityDump forwards an activity dump message to the backend
func (rsa *RuntimeSecurityAgent) DispatchActivityDump(msg *api.ActivityDumpStreamMessage) {
	selector := msg.GetSelector()
	image := selector.GetName()
	tag := selector.GetTag()

	if rsa.profContainersTelemetry != nil {
		// register for telemetry for this container
		if image != "" {
			rsa.profContainersTelemetry.registerProfiledContainer(image, tag)
		}
	}

	// storage might be nil, on windows for example
	if rsa.storage != nil {
		err := rsa.storage.HandleActivityDump(image, tag, msg.GetHeader(), msg.GetData())
		if err != nil {
			seclog.Errorf("couldn't handle activity dump: %v", err)
		}
	}
}

// newLogBackoffTicker returns a ticker based on an exponential backoff, used to trigger connect error logs
func newLogBackoffTicker() *backoff.Ticker {
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.MaxElapsedTime = 0
	expBackoff.Reset()
	return backoff.NewTicker(expBackoff)
}

func (rsa *RuntimeSecurityAgent) startActivityDumpStorageTelemetry(ctx context.Context) {
	metricsTicker := time.NewTicker(1 * time.Minute)
	defer metricsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-metricsTicker.C:
			if rsa.storage != nil {
				rsa.storage.SendTelemetry(rsa.statsdClient)
			}
		}
	}
}

func (rsa *RuntimeSecurityAgent) setupAPIServer() error {
	cfgSocketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket")
	socketPath := cfgSocketPath

	if socketPath == "" {
		return errors.New("runtime_security_config.socket_events must be set")
	}

	family := secconfig.GetFamilyAddress(socketPath)
	if family == "unix" {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("unix sockets are not supported on Windows")
		}

		socketPath = fmt.Sprintf("unix://%s", socketPath)
	}

	rsa.grpcServer = module.NewGRPCServer(family, cfgSocketPath)
	api.RegisterSecurityAgentAPIServer(rsa.grpcServer.ServiceRegistrar(), rsa)

	return nil
}
