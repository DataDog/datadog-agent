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

	backoffticker "github.com/cenkalti/backoff/v5"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	empty "google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	grpcutils "github.com/DataDog/datadog-agent/pkg/security/utils/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

// RuntimeSecurityAgent represents the main wrapper for the Runtime Security product
type RuntimeSecurityAgent struct {
	statsdClient         statsd.ClientInterface
	hostname             string
	reporter             common.RawReporter
	secInfoReporter      common.RawReporter
	eventClient          *RuntimeSecurityEventClient
	cmdClient            *RuntimeSecurityCmdClient
	running              *atomic.Bool
	wg                   sync.WaitGroup
	connected            *atomic.Bool
	eventReceived        *atomic.Uint64
	activityDumpReceived *atomic.Uint64
	endpoints            *config.Endpoints
	secInfoEndpoints     *config.Endpoints
	cancel               context.CancelFunc

	// activity dump
	storage ADStorage

	// grpc server
	api.UnimplementedSecurityAgentAPIServer
	eventGPRCServer *grpcutils.Server
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
func (rsa *RuntimeSecurityAgent) Start(reporter common.RawReporter, endpoints *config.Endpoints, secInfoReporter common.RawReporter, secInfoEndpoints *config.Endpoints) {
	rsa.reporter = reporter
	rsa.endpoints = endpoints
	rsa.secInfoReporter = secInfoReporter
	rsa.secInfoEndpoints = secInfoEndpoints

	ctx, cancel := context.WithCancel(context.Background())
	rsa.cancel = cancel

	rsa.running.Store(true)

	if runtime.GOOS == "linux" {
		go rsa.startActivityDumpStorageTelemetry(ctx)
	}

	if rsa.eventGPRCServer != nil {
		seclog.Infof("start security agent event grpc server")

		err := rsa.eventGPRCServer.Start()
		if err != nil {
			seclog.Errorf("error starting security agent grpc server: %v", err)
		}
	} else {
		seclog.Infof("start listening for events from system-probe")

		go rsa.startEventStreamListener()
		go rsa.startActivityDumpStreamListener()
	}
}

// Stop the runtime recurity agent
func (rsa *RuntimeSecurityAgent) Stop() {
	rsa.cancel()
	rsa.running.Store(false)
	rsa.cmdClient.Close()

	if rsa.eventClient != nil {
		rsa.eventClient.Close()
	}
	if rsa.eventGPRCServer != nil {
		rsa.eventGPRCServer.Stop()
	}
	rsa.wg.Wait()
}

// startEventStreamListener starts listening for new events from system-probe. communication system-probe -> security-agent
func (rsa *RuntimeSecurityAgent) startEventStreamListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()

	rsa.connected.Store(false)

	logTicker := newLogBackoffTicker()

	for rsa.running.Load() {
		stream, err := rsa.eventClient.GetEventStream()
		if err != nil {
			rsa.connected.Store(false)

			select {
			case <-logTicker.C:
				msg := fmt.Sprintf("error while connecting to the runtime security module: %v", err)

				if e, ok := status.FromError(err); ok {
					switch e.Code() {
					case codes.Unavailable:
						msg += ", please check that the runtime security module is enabled in the system-probe.yaml config file"
					}
				}
				seclog.Errorf("%s", msg)
			default:
				// do nothing
			}

			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		if !rsa.connected.Load() {
			rsa.connected.Store(true)

			seclog.Infof("Successfully connected to the runtime security module")
		}

		for {
			// Get new event from stream
			in, err := stream.Recv()
			if err == io.EOF || in == nil {
				break
			}

			if seclog.DefaultLogger.IsTracing() {
				seclog.DefaultLogger.Tracef("Got message from rule `%s` for event `%s`", in.RuleID, string(in.Data))
			}

			rsa.eventReceived.Inc()

			// Dispatch security event
			rsa.DispatchEvent(in)
		}
	}
}

// startActivityDumpstartActivityDumpStreamListenerListener starts listening for new activity dumps from system-probe. communication system-probe -> security-agent
func (rsa *RuntimeSecurityAgent) startActivityDumpStreamListener() {
	rsa.wg.Add(1)
	defer rsa.wg.Done()

	for rsa.running.Load() {
		stream, err := rsa.eventClient.GetActivityDumpStream()
		if err != nil {
			// retry in 2 seconds
			time.Sleep(2 * time.Second)
			continue
		}

		for {
			// Get new activity dump from stream
			msg, err := stream.Recv()
			if err == io.EOF || msg == nil {
				break
			}

			if seclog.DefaultLogger.IsTracing() {
				seclog.DefaultLogger.Tracef("Got activity dump [%s]", msg.GetSelector())
			}

			rsa.activityDumpReceived.Inc()

			// Dispatch activity dump
			rsa.DispatchActivityDump(msg)
		}
	}
}

// DispatchEvent dispatches a security event message to the subsystems of the runtime security agent
func (rsa *RuntimeSecurityAgent) DispatchEvent(evt *api.SecurityEventMessage) {
	if evt.Track == string(common.SecInfo) {
		if rsa.secInfoReporter == nil {
			return
		}
		rsa.secInfoReporter.ReportRaw(evt.GetData(), evt.Service, evt.Timestamp.AsTime(), evt.GetTags()...)
	} else {
		if rsa.reporter == nil {
			return
		}
		rsa.reporter.ReportRaw(evt.GetData(), evt.Service, evt.Timestamp.AsTime(), evt.GetTags()...)
	}
}

// DispatchActivityDump forwards an activity dump message to the backend
func (rsa *RuntimeSecurityAgent) DispatchActivityDump(msg *api.ActivityDumpStreamMessage) {
	selector := msg.GetSelector()
	image := selector.GetName()
	tag := selector.GetTag()

	// storage might be nil, on windows for example
	if rsa.storage != nil {
		err := rsa.storage.HandleActivityDump(image, tag, msg.GetHeader(), msg.GetData())
		if err != nil {
			seclog.Errorf("couldn't handle activity dump: %v", err)
		}
	}
}

// newLogBackoffTicker returns a ticker based on an exponential backoff, used to trigger connect error logs
func newLogBackoffTicker() *backoffticker.Ticker {
	expBackoff := backoffticker.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Second
	expBackoff.MaxInterval = 60 * time.Second
	expBackoff.Reset()
	return backoffticker.NewTicker(expBackoff)
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

//nolint:unused,deadcode
func (rsa *RuntimeSecurityAgent) setupGPRC() error {
	if pkgconfigsetup.Datadog().GetString("runtime_security_config.event_grpc_server") == "security-agent" {
		socketPath := pkgconfigsetup.Datadog().GetString("runtime_security_config.socket")
		if socketPath == "" {
			return errors.New("runtime_security_config.socket must be set")
		}

		family, socketPath := socket.GetSocketAddress(socketPath)
		if family == "unix" && runtime.GOOS == "windows" {
			return errors.New("unix sockets are not supported on Windows")
		}

		rsa.eventGPRCServer = grpcutils.NewServer(family, socketPath)
		api.RegisterSecurityAgentAPIServer(rsa.eventGPRCServer.ServiceRegistrar(), rsa)
	} else {
		client, err := NewRuntimeSecurityEventClient()
		if err != nil {
			return err
		}
		rsa.eventClient = client
	}

	return nil
}
