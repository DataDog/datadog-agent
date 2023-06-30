// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processcollector

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/datadog-agent/pkg/config"
	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	protoutils "github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/telemetry"
)

const (
	collectorID = "process-collector"
)

type client struct {
	cl              pb.ProcessEntityStreamClient
	parentCollector *remoteProcessCollectorStreamHandler
}

func (c *client) StreamEntities(ctx context.Context, opts ...grpc.CallOption) (remote.Stream, error) {
	log.Info("[remoteprocesscollector] starting a new stream")
	c.parentCollector.eventIdSet = false // Can be removed when the remote workloadmeta guarantees to not skip any event
	streamcl, err := c.cl.StreamEntities(
		ctx,
		&pb.ProcessStreamEntitiesRequest{},
	)
	if err != nil {
		return nil, err
	}
	return &stream{cl: streamcl}, nil
}

type stream struct {
	cl pbgo.ProcessEntityStream_StreamEntitiesClient
}

func (s *stream) Recv() (interface{}, error) {
	log.Trace("[remoteprocesscollector] calling stream recv")
	return s.cl.Recv()
}

type remoteProcessCollectorStreamHandler struct {
	lastEventID int32
	eventIdSet  bool
}

func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &remote.GenericCollector{
			CollectorID:   collectorID,
			StreamHandler: &remoteProcessCollectorStreamHandler{},
			Port:          config.Datadog.GetInt("process_config.language_detection.grpc_port"),
			Insecure:      true, // wlm extractor currently does not support TLS
		}
	})
}

// For now, we do not use detectFeature to enable or disable the remote workloadmeta
func (s *remoteProcessCollectorStreamHandler) IsEnabled() error {
	if !config.IsFeaturePresent(config.RemoteProcessCollector) {
		return dderrors.NewDisabled(collectorID, "remote process collector not detected")
	}
	log.Trace("[remoteprocesscollector] feature is enabled")
	return nil
}

func (s *remoteProcessCollectorStreamHandler) NewClient(cc grpc.ClientConnInterface) remote.RemoteGrpcClient {
	log.Trace("[remoteprocesscollector] creating grpc client")
	return &client{cl: pb.NewProcessEntityStreamClient(cc), parentCollector: s}
}

func (s *remoteProcessCollectorStreamHandler) HandleResponse(resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	log.Trace("[remoteprocesscollector] handling response")
	response, ok := resp.(*pb.ProcessStreamResponse)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}

	if s.eventIdSet {
		if response.EventID != s.lastEventID+1 {
			// This edge case should not occur if the server does not skip any EventID which is not the case for 7.47.0 release
			log.Warnf("remote process collector server is out of sync: expected id [%d], received id [%d]", s.lastEventID+1, response.EventID)
			telemetry.RemoteProcessCollectorOutOfSync.Inc()
		}
		s.lastEventID = response.EventID
	} else {
		s.lastEventID = response.EventID
		s.eventIdSet = true
	}

	collectorEvents := make([]workloadmeta.CollectorEvent, 0, len(response.SetEvents)+len(response.UnsetEvents))

	collectorEvents = handleEvents(collectorEvents, response.UnsetEvents, protoutils.WorkloadmetaEventFromProcessEventUnset)
	collectorEvents = handleEvents(collectorEvents, response.SetEvents, protoutils.WorkloadmetaEventFromProcessEventSet)
	log.Tracef("[remoteprocesscollector] collected [%d] events", len(collectorEvents))
	return collectorEvents, nil
}

func handleEvents[T any](collectorEvents []workloadmeta.CollectorEvent, setEvents []T, convertFunc func(T) (workloadmeta.Event, error)) []workloadmeta.CollectorEvent {
	for _, protoEvent := range setEvents {
		workloadmetaEvent, err := convertFunc(protoEvent)
		if err != nil {
			log.Warnf("error converting workloadmeta event: %v", err)
			continue
		}

		collectorEvent := workloadmeta.CollectorEvent{
			Type:   workloadmetaEvent.Type,
			Source: workloadmeta.SourceRemoteProcessCollector,
			Entity: workloadmetaEvent.Entity,
		}

		collectorEvents = append(collectorEvents, collectorEvent)
	}
	return collectorEvents
}

func (s *remoteProcessCollectorStreamHandler) HandleResync(store workloadmeta.Store, events []workloadmeta.CollectorEvent) {
	var processes []workloadmeta.Entity
	for _, event := range events {
		processes = append(processes, event.Entity)
	}
	// This should be the first response that we got from workloadmeta after
	// we lost the connection and specified that a re-sync is needed. So, at
	// this point we know that "processes" contains all the existing processes
	// in the store, because when a client subscribes to the workloadmeta subscriber
	// the first response is always a bundle of events with all the existing
	// processes in the store.
	log.Debugf("[remoteprocesscollector] resync, handling [%d] events", len(processes))
	store.ResetProcesses(processes, workloadmeta.SourceRemoteProcessCollector)
}
