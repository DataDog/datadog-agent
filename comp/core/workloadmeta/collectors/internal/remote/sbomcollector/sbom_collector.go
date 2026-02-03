// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

// Package sbomcollector implements the remote SBOM collector for
// Workloadmeta.
package sbomcollector

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/protobuf/proto"
)

const (
	collectorID       = "sbom-collector"
	cacheValidityNoRT = 2 * time.Second
)

type client struct {
	cl sbompb.SBOMCollectorClient
}

func (c *client) StreamEntities(ctx context.Context) (remote.Stream, error) {
	log.Debug("starting a new stream")
	streamcl, err := c.cl.GetSBOMStream(
		ctx,
		&sbompb.SBOMStreamParams{},
	)
	if err != nil {
		return nil, err
	}
	return &stream{cl: streamcl}, nil
}

type stream struct {
	cl sbompb.SBOMCollector_GetSBOMStreamClient
}

func (s *stream) Recv() (interface{}, error) {
	log.Trace("calling stream recv")
	return s.cl.Recv()
}

type streamHandler struct {
	model.Reader
}

// workloadmetaEventFromSBOMEventSet converts the given SBOM message into a workloadmeta event
func workloadmetaEventFromSBOMEventSet(event *sbompb.SBOMMessage) (workloadmeta.Event, error) {
	if event == nil {
		return workloadmeta.Event{}, nil
	}

	var bom cyclonedx_v1_4.Bom
	err := proto.Unmarshal(event.Data, &bom)
	if err != nil {
		return workloadmeta.Event{}, fmt.Errorf("failed to unmarshal SBOM: %w", err)
	}

	if event.Kind != string(workloadmeta.KindContainer) {
		return workloadmeta.Event{}, fmt.Errorf("expected KindContainer, got %s", event.Kind)
	}

	if event.ID == "" {
		return workloadmeta.Event{}, fmt.Errorf("expected container ID, got empty")
	}

	log.Debugf("Received forwarded SBOM for container %s", event.ID)

	return workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   event.ID,
			},
			SBOM: &workloadmeta.SBOM{
				CycloneDXBOM: &bom,
			},
		},
	}, nil
}

// NewCollector returns a remote process collector for workloadmeta if any
func NewCollector(ipc ipc.Component) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &remote.GenericCollector{
			CollectorID: collectorID,
			// TODO(components): make sure StreamHandler uses the config component not pkg/config
			StreamHandler: &streamHandler{Reader: pkgconfigsetup.SystemProbe()},
			Catalog:       workloadmeta.NodeAgent,
			IPC:           ipc,
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
	return 0
}

func (s *streamHandler) Address() string {
	return s.GetString("runtime_security_config.socket")
}

func (s *streamHandler) Credentials() credentials.TransportCredentials {
	return insecure.NewCredentials()
}

func (s *streamHandler) IsEnabled() bool {
	if flavor.GetFlavor() != flavor.DefaultAgent {
		return false
	}

	runtimeSecurityEnabled := s.Reader.GetBool("runtime_security_config.enabled")
	runtimeSecuritySBOMEnabled := s.Reader.GetBool("runtime_security_config.sbom.enabled")

	return runtimeSecurityEnabled && runtimeSecuritySBOMEnabled
}

func (s *streamHandler) NewClient(cc grpc.ClientConnInterface) remote.GrpcClient {
	log.Debug("creating grpc client")

	return &client{cl: sbompb.NewSBOMCollectorClient(cc)}
}

func (s *streamHandler) HandleResponse(_ workloadmeta.Component, resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	log.Trace("handling response")
	response, ok := resp.(*sbompb.SBOMMessage)
	if !ok {
		return nil, fmt.Errorf("incorrect response type")
	}

	var collectorEvents []workloadmeta.CollectorEvent
	collectorEvents = handleEvents(collectorEvents, []*sbompb.SBOMMessage{response}, workloadmetaEventFromSBOMEventSet)
	log.Tracef("collected [%d] events", len(collectorEvents))
	return collectorEvents, nil
}

func handleEvents(collectorEvents []workloadmeta.CollectorEvent, sbomEvents []*sbompb.SBOMMessage, convertFunc func(*sbompb.SBOMMessage) (workloadmeta.Event, error)) []workloadmeta.CollectorEvent {
	for _, protoEvent := range sbomEvents {
		workloadmetaEvent, err := convertFunc(protoEvent)
		if err != nil {
			log.Warnf("error converting workloadmeta event: %v", err)
			continue
		}

		collectorEvent := workloadmeta.CollectorEvent{
			Type:   workloadmetaEvent.Type,
			Source: workloadmeta.SourceRemoteSBOMCollector,
			Entity: workloadmetaEvent.Entity,
		}

		collectorEvents = append(collectorEvents, collectorEvent)
	}
	return collectorEvents
}

func (s *streamHandler) HandleResync(_ workloadmeta.Component, _ []workloadmeta.CollectorEvent) {
}
