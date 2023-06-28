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
	protoutils "github.com/DataDog/datadog-agent/pkg/util/proto"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/remote"
)

const (
	collectorID = "process-collector"
)

type client struct {
	cl pb.ProcessEntityStreamClient
}

func (c *client) StreamEntities(ctx context.Context, opts ...grpc.CallOption) (remote.Stream, error) {
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
	return s.cl.Recv()
}

type remoteProcessCollectorStreamHandler struct{}

func init() {
	grpclog.SetLoggerV2(grpcutil.NewLogger())
	workloadmeta.RegisterCollector(collectorID, func() workloadmeta.Collector {
		return &remote.GenericCollector{
			StreamHandler: &remoteProcessCollectorStreamHandler{},
			Port:          config.Datadog.GetInt("process_config.language_detection.grpc_port"),
		}
	})
}

// For now, we do not use detectFeature to enable or disable the remote workloadmeta
func (s *remoteProcessCollectorStreamHandler) IsEnabled() error {
	if !config.IsFeaturePresent(config.RemoteProcessCollector) {
		return dderrors.NewDisabled(collectorID, "remote process collector not detected")
	}
	return nil
}

func (s *remoteProcessCollectorStreamHandler) NewClient(cc grpc.ClientConnInterface) remote.RemoteGrpcClient {
	return &client{cl: pb.NewProcessEntityStreamClient(cc)}
}

func (s *remoteProcessCollectorStreamHandler) HandleResponse(resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	response, ok := resp.(*pb.ProcessStreamResponse)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}
	collectorEvents := make([]workloadmeta.CollectorEvent, 0, len(response.SetEvents)+len(response.UnsetEvents))

	collectorEvents = handleEvents(collectorEvents, response.UnsetEvents, protoutils.WorkloadmetaEventFromProcessEventUnset)
	collectorEvents = handleEvents(collectorEvents, response.SetEvents, protoutils.WorkloadmetaEventFromProcessEventSet)

	return collectorEvents, nil
}

func handleEvents[T any](collectorEvents []workloadmeta.CollectorEvent, setEvents []T, convertFunc func(T) (workloadmeta.Event, error)) []workloadmeta.CollectorEvent {
	for _, protoEvent := range setEvents {
		workloadmetaEvent, err := convertFunc(protoEvent)
		if err != nil {
			return collectorEvents
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
	store.ResetProcesses(processes, workloadmeta.SourceRemoteProcessCollector)
}
