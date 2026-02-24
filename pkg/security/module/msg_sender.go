// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package module holds module related files
package module

import (
	"fmt"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/reporter"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// MsgSender defines a message sender
type MsgSender[T any] interface {
	Send(msg *T, expireFnc func(*T))
	SendTelemetry(statsd.ClientInterface)
}

// EndpointsStatusFetcher defines an interface to get the status of the endpoints
type EndpointsStatusFetcher interface {
	GetEndpointsStatus() []string
}

// EventMsgSender defines a message sender for security events
type EventMsgSender = MsgSender[api.SecurityEventMessage]

// ActivityDumpMsgSender defines a message sender for activity dump messages
type ActivityDumpMsgSender = MsgSender[api.ActivityDumpStreamMessage]

// ChanMsgSender defines a chan message sender
type ChanMsgSender[T any] struct {
	msgs chan *T
}

// Send the message
func (cs *ChanMsgSender[T]) Send(msg *T, expireFnc func(*T)) {
	select {
	case cs.msgs <- msg:
		break
	default:
		// The channel is full, consume the oldest event
		select {
		case oldestMsg := <-cs.msgs:
			expireFnc(oldestMsg)
		default:
			break
		}

		// Try to send the event again
		select {
		case cs.msgs <- msg:
			break
		default:
			// Looks like the channel is full again, expire the current message too
			expireFnc(msg)
			break
		}
		break
	}
}

// SendTelemetry sends telemetry data
func (cs *ChanMsgSender[T]) SendTelemetry(statsd.ClientInterface) {}

// NewChanMsgSender returns a new chan sender
func NewChanMsgSender[T any](msgs chan *T) *ChanMsgSender[T] {
	return &ChanMsgSender[T]{
		msgs: msgs,
	}
}

// DirectEventMsgSender defines a direct sender
type DirectEventMsgSender struct {
	reporter         common.RawReporter
	endpoints        *logsconfig.Endpoints
	secInfoReporter  common.RawReporter
	secInfoEndpoints *logsconfig.Endpoints
}

var _ MsgSender[api.SecurityEventMessage] = &DirectEventMsgSender{}
var _ EndpointsStatusFetcher = &DirectEventMsgSender{}

// Send the message
func (ds *DirectEventMsgSender) Send(msg *api.SecurityEventMessage, _ func(*api.SecurityEventMessage)) {
	if msg.Track == string(common.SecInfo) {
		ds.secInfoReporter.ReportRaw(msg.Data, msg.Service, msg.Timestamp.AsTime(), msg.Tags...)
	} else {
		ds.reporter.ReportRaw(msg.Data, msg.Service, msg.Timestamp.AsTime(), msg.Tags...)
	}
}

// SendTelemetry sends telemetry data
func (ds *DirectEventMsgSender) SendTelemetry(statsd.ClientInterface) {}

// GetEndpointsStatus returns the status of the endpoints
func (ds *DirectEventMsgSender) GetEndpointsStatus() []string {
	return append(ds.endpoints.GetStatus(), ds.secInfoEndpoints.GetStatus()...)
}

// NewDirectEventMsgSender returns a new direct sender
func NewDirectEventMsgSender(stopper startstop.Stopper, compression compression.Component, hostname string) (*DirectEventMsgSender, error) {
	useSecRuntimeTrack := pkgconfigsetup.SystemProbe().GetBool("runtime_security_config.use_secruntime_track")

	endpoints, destinationsCtx, err := common.NewLogContextRuntime(useSecRuntimeTrack)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reported endpoints: %w", err)
	}

	secInfoEndpoints, secInfoDestinationsCtx, err := common.NewLogContextSecInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to create direct secinfo endpoints: %w", err)
	}

	stopper.Add(destinationsCtx)
	stopper.Add(secInfoDestinationsCtx)

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	for _, status := range secInfoEndpoints.GetStatus() {
		log.Info(status)
	}

	// we set the hostname to the empty string to take advantage of the out of the box message hostname
	// resolution
	runtimeReporter, err := reporter.NewCWSReporter(hostname, stopper, endpoints, destinationsCtx, compression)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct reporter: %w", err)
	}

	secInfoReporter, err := reporter.NewCWSReporter(hostname, stopper, secInfoEndpoints, secInfoDestinationsCtx, compression)
	if err != nil {
		return nil, fmt.Errorf("failed to create direct secinfo reporter: %w", err)
	}

	return &DirectEventMsgSender{
		reporter:         runtimeReporter,
		endpoints:        endpoints,
		secInfoReporter:  secInfoReporter,
		secInfoEndpoints: secInfoEndpoints,
	}, nil
}

// DirectActivityDumpMsgSender defines a direct activity dump sender
type DirectActivityDumpMsgSender struct {
	backend *backend.ActivityDumpRemoteBackend
}

var _ MsgSender[api.ActivityDumpStreamMessage] = &DirectActivityDumpMsgSender{}
var _ EndpointsStatusFetcher = &DirectActivityDumpMsgSender{}

// NewDirectActivityDumpMsgSender returns a new direct activity dump sender
func NewDirectActivityDumpMsgSender() (*DirectActivityDumpMsgSender, error) {
	backend, err := backend.NewActivityDumpRemoteBackend()
	if err != nil {
		return nil, fmt.Errorf("failed to create activity dump backend: %w", err)
	}

	return &DirectActivityDumpMsgSender{
		backend: backend,
	}, nil
}

// Send the message
func (ds *DirectActivityDumpMsgSender) Send(msg *api.ActivityDumpStreamMessage, _ func(*api.ActivityDumpStreamMessage)) {
	selector := msg.GetSelector()
	image := selector.GetName()
	tag := selector.GetTag()

	err := ds.backend.HandleActivityDump(image, tag, msg.GetHeader(), msg.GetData())
	if err != nil {
		seclog.Errorf("couldn't handle activity dump: %v", err)
	}
}

// SendTelemetry sends telemetry data
func (ds *DirectActivityDumpMsgSender) SendTelemetry(statsd statsd.ClientInterface) {
	ds.backend.SendTelemetry(statsd)
}

// GetEndpointsStatus returns the status of the endpoints
func (ds *DirectActivityDumpMsgSender) GetEndpointsStatus() []string {
	return ds.backend.GetEndpointsStatus()
}
