// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package processcollector implements the remote process collector for
// Workloadmeta.
package processcollector

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/process"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID       = "process-collector"
	cacheValidityNoRT = 2 * time.Second
)

func toLanguage(proto *pbgo.Language) *languagemodels.Language {
	if proto == nil {
		return nil
	}
	return &languagemodels.Language{
		Name: languagemodels.LanguageName(proto.GetName()),
	}
}

type client struct {
	cl              pbgo.ProcessEntityStreamClient
	parentCollector *streamHandler
}

func (c *client) StreamEntities(ctx context.Context, opts ...grpc.CallOption) (remote.Stream, error) { //nolint:revive // TODO fix revive unused-parameter
	log.Debug("starting a new stream")
	streamcl, err := c.cl.StreamEntities(
		ctx,
		&pbgo.ProcessStreamEntitiesRequest{},
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
	log.Trace("calling stream recv")
	return s.cl.Recv()
}

type streamHandler struct {
	port int
	config.Reader
	lastContainerRates map[string]*proccontainers.ContainerRateMetrics
	pidToCid           map[int]string
}

// WorkloadmetaEventFromProcessEventSet converts the given ProcessEventSet into a workloadmeta.Event
func (s *streamHandler) WorkloadmetaEventFromProcessEventSet(protoEvent *pbgo.ProcessEventSet) (workloadmeta.Event, error) {
	if protoEvent == nil {
		return workloadmeta.Event{}, nil
	}

	ctrID := protoEvent.GetContainerID()

	if ctrID == "" && s.pidToCid != nil {
		if cidFromMapping, found := s.pidToCid[int(protoEvent.GetPid())]; found {
			ctrID = cidFromMapping
		}
	}

	if ctrID == "" {
		log.Debugf("failed to obtain container id for process with process id %d", protoEvent.GetPid())
	}

	return workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(protoEvent.GetPid())),
			},
			NsPid:        protoEvent.GetNspid(),
			ContainerID:  ctrID,
			CreationTime: time.UnixMilli(protoEvent.GetCreationTime()), // TODO: confirm what we receive as creation time here
			Language:     toLanguage(protoEvent.GetLanguage()),
		},
	}, nil
}

// WorkloadmetaEventFromProcessEventUnset converts the given ProcessEventSet into a workloadmeta.Event
func (s *streamHandler) WorkloadmetaEventFromProcessEventUnset(protoEvent *pbgo.ProcessEventUnset) (workloadmeta.Event, error) {
	if protoEvent == nil {
		return workloadmeta.Event{}, nil
	}

	return workloadmeta.Event{
		Type: workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(protoEvent.GetPid())),
			},
		},
	}, nil
}

// NewCollector returns a remote process collector for workloadmeta if any
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &remote.GenericCollector{
			CollectorID: collectorID,
			// TODO(components): make sure StreamHandler uses the config component not pkg/config
			StreamHandler: &streamHandler{
				Reader:             config.Datadog(),
				lastContainerRates: make(map[string]*proccontainers.ContainerRateMetrics),
			},
			Catalog:  workloadmeta.NodeAgent,
			Insecure: true, // wlm extractor currently does not support TLS
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func init() {
	// TODO(components): verify the grpclogin is initialized elsewhere and clean up
	grpclog.SetLoggerV2(grpcutil.NewLogger())
}

func (s *streamHandler) Port() int {
	if s.port == 0 {
		return s.Reader.GetInt("process_config.language_detection.grpc_port")
	}
	// for test purposes
	return s.port
}

func (s *streamHandler) IsEnabled() bool {
	if flavor.GetFlavor() != flavor.DefaultAgent {
		return false
	}
	return s.Reader.GetBool("language_detection.enabled")
}

func (s *streamHandler) NewClient(cc grpc.ClientConnInterface) remote.GrpcClient {
	log.Debug("creating grpc client")
	return &client{cl: pbgo.NewProcessEntityStreamClient(cc), parentCollector: s}
}

// fetchContainerID updates the PID to Container ID mapping if at least one event is missing container ID field
func (s *streamHandler) fetchContainerID(setEvents []*pbgo.ProcessEventSet, store workloadmeta.Component) {
	requireFetch := false

	for _, event := range setEvents {
		if event != nil && event.GetContainerID() == "" {
			requireFetch = true
		}
	}

	if requireFetch {
		containerProvider := proccontainers.GetSharedContainerProvider(store)
		_, _, pidToCid, err := containerProvider.GetContainers(cacheValidityNoRT, s.lastContainerRates)
		if err != nil {
			log.Warnf("error getting container id for process entity")
		}

		s.pidToCid = pidToCid
	}

}

func (s *streamHandler) HandleResponse(store workloadmeta.Component, resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	log.Trace("handling response")
	response, ok := resp.(*pbgo.ProcessStreamResponse)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}

	s.fetchContainerID(response.SetEvents, store)

	collectorEvents := make([]workloadmeta.CollectorEvent, 0, len(response.SetEvents)+len(response.UnsetEvents))
	collectorEvents = handleEvents(collectorEvents, response.UnsetEvents, s.WorkloadmetaEventFromProcessEventUnset)
	collectorEvents = handleEvents(collectorEvents, response.SetEvents, s.WorkloadmetaEventFromProcessEventSet)
	log.Tracef("collected [%d] events", len(collectorEvents))
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

func (s *streamHandler) HandleResync(store workloadmeta.Component, events []workloadmeta.CollectorEvent) {
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
	log.Debugf("resync, handling [%d] events", len(processes))
	store.ResetProcesses(processes, workloadmeta.SourceRemoteProcessCollector)
}
