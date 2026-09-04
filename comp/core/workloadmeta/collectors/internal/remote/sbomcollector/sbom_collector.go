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
	"errors"
	"fmt"
	"strings"

	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/internal/remote"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup" //nolint:depguard
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/protobuf/proto"
)

const collectorID = "sbom-collector"

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
	agentConfig       model.Reader
	systemProbeConfig model.Reader
}

// workloadmetaEventFromSBOMEventSet converts the given SBOM message into a workloadmeta event
func workloadmetaEventFromSBOMEventSet(store workloadmeta.Component, event *sbompb.SBOMMessage) (workloadmeta.Event, error) {
	if event == nil {
		return workloadmeta.Event{}, nil
	}

	var newBom cyclonedx_v1_4.Bom
	err := proto.Unmarshal(event.Data, &newBom)
	if err != nil {
		return workloadmeta.Event{}, fmt.Errorf("failed to unmarshal SBOM: %w", err)
	}

	if event.Kind != string(workloadmeta.KindContainer) {
		return workloadmeta.Event{}, fmt.Errorf("expected KindContainer, got %s", event.Kind)
	}

	if event.ID == "" {
		return workloadmeta.Event{}, errors.New("expected container ID, got empty")
	}

	log.Debugf("Received forwarded SBOM for container %s", event.ID)

	// Get container to find its image
	container, err := store.GetContainer(event.ID)
	if err != nil || container == nil {
		return workloadmeta.Event{}, fmt.Errorf("container %s not found in workloadmeta: %w", event.ID, err)
	}

	// Get the image ID from the container
	imageID := container.Image.ID
	if imageID == "" {
		return workloadmeta.Event{}, fmt.Errorf("container %s has no image ID", event.ID)
	}

	log.Debugf("Container %s uses image %s, updating image SBOM", event.ID, imageID)

	// Get existing image to merge SBOM data
	existingImage, err := store.GetImage(imageID)
	if err != nil || existingImage == nil {
		// Kubelet reports Image.ID as the manifest/repo digest (e.g. "docker.io/foo@sha256:9fb3...")
		// but images are stored by config digest. Fall back to a linear search on RepoDigests.
		for _, img := range store.ListImages() {
			for _, digest := range img.RepoDigests {
				if digest == imageID {
					existingImage = img
					break
				}
			}
			if existingImage != nil {
				break
			}
		}
	}
	if existingImage == nil {
		log.Debugf("Ignoring system-probe SBOM for image %s: image not found in workloadmeta", imageID)
		return workloadmeta.Event{}, nil
	}

	if existingImage.SBOM == nil {
		log.Debugf("Existing image %s has no SBOM, skipping", imageID)
		return workloadmeta.Event{}, fmt.Errorf("existing image %s has no SBOM to merge with", imageID)
	}

	if existingImage.SBOM.Status == workloadmeta.Pending || existingImage.SBOM.Status == "" {
		log.Debugf("Image %s SBOM is still in state '%s', skipping merge for now", imageID, existingImage.SBOM.Status)
		return workloadmeta.Event{}, fmt.Errorf("image %s SBOM is still pending", imageID)
	}

	// Decompress existing image SBOM to get CycloneDXBOM
	existingSBOM, err := sbomutil.UncompressSBOM(existingImage.SBOM)
	if err != nil || existingSBOM == nil || existingSBOM.CycloneDXBOM == nil {
		return workloadmeta.Event{}, fmt.Errorf("Failed to decompress existing SBOM for image %s: %v, using new SBOM", imageID, err)
	}

	// Merge runtime properties from new BOM into existing image SBOM
	finalBom := sbomutil.MergeRuntimeProperties(existingSBOM.CycloneDXBOM, &newBom)
	log.Debugf("Merged runtime properties for image %s SBOM", imageID)

	// Compress the final merged SBOM, preserving scan metadata from the existing
	// SBOM so Status/GenerationTime/etc. survive the runtime-enrichment update.
	sbomToCompress := &workloadmeta.SBOM{
		CycloneDXBOM:       finalBom,
		Status:             existingImage.SBOM.Status,
		GenerationTime:     existingImage.SBOM.GenerationTime,
		GenerationDuration: existingImage.SBOM.GenerationDuration,
		GenerationMethod:   existingImage.SBOM.GenerationMethod,
		Error:              existingImage.SBOM.Error,
	}

	finalCompressedSBOM, err := sbomutil.CompressSBOM(sbomToCompress)
	if err != nil {
		return workloadmeta.Event{}, fmt.Errorf("failed to compress SBOM for image %s: %w", imageID, err)
	}

	// Emit the enriched SBOM under the resolved image's own EntityID, so it lands
	// on the same ContainerImageMetadata the runtime collector populates and
	// workloadmeta merges the two sources. existingImage is looked up by
	// container.Image.ID, or by matching RepoDigest when the kubelet reports that
	// ID as a manifest/repo digest, so its ID can differ from imageID.
	return workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.ContainerImageMetadata{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainerImageMetadata,
				ID:   existingImage.EntityID.ID,
			},
			SBOM: finalCompressedSBOM,
		},
	}, nil
}

// NewCollector returns a remote process collector for workloadmeta if any
func NewCollector(ipc ipc.Component) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &remote.GenericCollector{
			CollectorID: collectorID,
			// TODO(components): make sure StreamHandler uses the config component not pkg/config
			StreamHandler: &streamHandler{agentConfig: pkgconfigsetup.Datadog(), systemProbeConfig: pkgconfigsetup.SystemProbe()},
			Config:        pkgconfigsetup.Datadog(), //nolint:depguard
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
	// SBOM collector service is on the command socket, not the main runtime security socket
	cmdSocket := s.systemProbeConfig.GetString("runtime_security_config.cmd_socket")
	if cmdSocket != "" {
		return cmdSocket
	}

	// If cmd_socket not explicitly set, derive it from main socket (adds "cmd-" prefix)
	mainSocket := s.systemProbeConfig.GetString("runtime_security_config.socket")
	if mainSocket == "" {
		return ""
	}

	// Derive command socket path (same logic as server)
	// For unix sockets: /path/to/runtime-security.sock -> /path/to/cmd-runtime-security.sock
	if dir := mainSocket[:strings.LastIndex(mainSocket, "/")+1]; dir != "" {
		filename := mainSocket[strings.LastIndex(mainSocket, "/")+1:]
		return dir + "cmd-" + filename
	}

	return mainSocket
}

func (s *streamHandler) Credentials() credentials.TransportCredentials {
	return insecure.NewCredentials()
}

func (s *streamHandler) IsEnabled() bool {
	if flavor.GetFlavor() != flavor.DefaultAgent {
		return false
	}

	sbomEnrichmentEnabled := s.agentConfig.GetBool("sbom.enrichment.usage.enabled")
	runtimeSecuritySBOMDisabled := s.systemProbeConfig.IsConfigured("runtime_security_config.sbom.enabled") && !s.systemProbeConfig.GetBool("runtime_security_config.sbom.enabled")

	return sbomEnrichmentEnabled && !runtimeSecuritySBOMDisabled
}

func (s *streamHandler) NewClient(cc grpc.ClientConnInterface) remote.GrpcClient {
	log.Debug("creating grpc client")

	return &client{cl: sbompb.NewSBOMCollectorClient(cc)}
}

func (s *streamHandler) HandleResponse(store workloadmeta.Component, resp interface{}) ([]workloadmeta.CollectorEvent, error) {
	log.Trace("handling response")
	response, ok := resp.(*sbompb.SBOMMessage)
	if !ok {
		return nil, errors.New("incorrect response type")
	}

	var collectorEvents []workloadmeta.CollectorEvent
	collectorEvents = handleEvents(store, collectorEvents, []*sbompb.SBOMMessage{response}, workloadmetaEventFromSBOMEventSet)
	log.Tracef("collected [%d] events", len(collectorEvents))
	return collectorEvents, nil
}

func handleEvents(store workloadmeta.Component, collectorEvents []workloadmeta.CollectorEvent, sbomEvents []*sbompb.SBOMMessage, convertFunc func(workloadmeta.Component, *sbompb.SBOMMessage) (workloadmeta.Event, error)) []workloadmeta.CollectorEvent {
	for _, protoEvent := range sbomEvents {
		workloadmetaEvent, err := convertFunc(store, protoEvent)
		if err != nil {
			log.Warnf("error converting workloadmeta event: %v", err)
			continue
		}
		if workloadmetaEvent.Entity == nil {
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

// IsResyncComplete always returns true because the SBOM collector does not
// use chunked snapshots.
func (s *streamHandler) IsResyncComplete(_ interface{}) bool {
	return true
}

func (s *streamHandler) HandleResync(_ workloadmeta.Component, _ []workloadmeta.CollectorEvent) {
}
