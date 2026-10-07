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
	"sync"

	"github.com/hashicorp/golang-lru/v2/simplelru"
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
	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/protobuf/proto"
)

const (
	collectorID = "sbom-collector"

	// maxPendingReports bounds the reports waiting for the SBOM of their image.
	maxPendingReports = 256

	// Runtime property names, aliased from pkg/sbom so that the producer of
	// the enriched SBOM and this merger agree on them.
	LastAccessProperty    = sbompkg.LastAccessProperty
	HasSetSuidBitProperty = sbompkg.HasSetSuidBitProperty
	RunningAsRootProperty = sbompkg.RunningAsRootProperty
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
	agentConfig       model.Reader
	systemProbeConfig model.Reader
	pending           *pendingReports
}

// errNotReady marks a report that arrived before its container, its image or
// the SBOM of its image, which the collector keeps to merge later.
var errNotReady = errors.New("not ready")

// pendingReports holds the latest report of each container that arrived before
// the SBOM of its image could take it.
type pendingReports struct {
	mu      sync.Mutex
	reports *simplelru.LRU[string, *sbompb.SBOMMessage]
}

func newPendingReports() *pendingReports {
	reports, _ := simplelru.NewLRU[string, *sbompb.SBOMMessage](maxPendingReports, nil)
	return &pendingReports{reports: reports}
}

// add keeps msg as the latest report of its container.
func (p *pendingReports) add(msg *sbompb.SBOMMessage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reports.Add(msg.ID, msg)
}

// remove drops the report of container id, or only msg when msg is set, and
// reports whether it dropped one.
func (p *pendingReports) remove(id string, msg *sbompb.SBOMMessage) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if kept, ok := p.reports.Peek(id); ok && (msg == nil || kept == msg) {
		return p.reports.Remove(id)
	}
	return false
}

func (p *pendingReports) list() []*sbompb.SBOMMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reports.Values()
}

// forget drops the reports of the containers that events remove.
func (p *pendingReports) forget(events []workloadmeta.Event) {
	for _, ev := range events {
		if id := ev.Entity.GetID(); ev.Type == workloadmeta.EventTypeUnset && id.Kind == workloadmeta.KindContainer {
			p.remove(id.ID, nil)
		}
	}
}

// merge returns the events merging the reports that the store can now take. A
// report decodes once its image is ready, as every store change calls merge.
func (p *pendingReports) merge(store workloadmeta.Component) []workloadmeta.CollectorEvent {
	var merged []workloadmeta.CollectorEvent
	for _, msg := range p.list() {
		if _, _, err := readyImage(store, msg.ID); err != nil {
			continue
		}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		if errors.Is(err, errNotReady) || !p.remove(msg.ID, msg) {
			continue
		}
		if err != nil {
			log.Warnf("error converting a pending SBOM report: %v", err)
			continue
		}
		merged = append(merged, workloadmeta.CollectorEvent{
			Type:   event.Type,
			Source: workloadmeta.SourceRemoteSBOMCollector,
			Entity: event.Entity,
		})
	}
	return merged
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

	existingImage, imageID, err := readyImage(store, event.ID)
	if err != nil {
		return workloadmeta.Event{}, err
	}

	log.Debugf("Container %s uses image %s, updating image SBOM", event.ID, imageID)

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

// readyImage returns the image of container id and the ID the container gives it,
// once its SBOM can take a report, or else an error wrapping errNotReady.
func readyImage(store workloadmeta.Component, id string) (*workloadmeta.ContainerImageMetadata, string, error) {
	// Get container to find its image
	container, err := store.GetContainer(id)
	if err != nil || container == nil {
		return nil, "", fmt.Errorf("container %s not found in workloadmeta: %w", id, errNotReady)
	}

	// Get the image ID from the container
	imageID := container.Image.ID
	if imageID == "" {
		return nil, "", fmt.Errorf("container %s has no image ID: %w", id, errNotReady)
	}

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
		return nil, "", fmt.Errorf("image %s not found in workloadmeta: %w", imageID, errNotReady)
	}

	if existingImage.SBOM == nil {
		return nil, "", fmt.Errorf("existing image %s has no SBOM to merge with: %w", imageID, errNotReady)
	}

	if existingImage.SBOM.Status == workloadmeta.Pending || existingImage.SBOM.Status == "" {
		return nil, "", fmt.Errorf("image %s SBOM is still pending: %w", imageID, errNotReady)
	}

	return existingImage, imageID, nil
}

// collector merges the reports of system-probe into image SBOMs, and forgets
// them once the runtime removes their image.
type collector struct {
	*remote.GenericCollector
}

// Start starts the stream of reports and the watch of removed images
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if err := c.GenericCollector.Start(ctx, store); err != nil {
		return err
	}
	forgetRemovedImages(ctx, store)
	if handler, ok := c.StreamHandler.(*streamHandler); ok {
		mergePendingReports(ctx, store, handler.pending)
	}
	return nil
}

// mergePendingReports merges the pending reports as the store changes. Merges
// notify the store from a second goroutine, as the subscription gets the events.
func mergePendingReports(ctx context.Context, store workloadmeta.Component, pending *pendingReports) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		AddKind(workloadmeta.KindContainerImageMetadata).
		Build()
	ch := store.Subscribe(collectorID+"-pending", workloadmeta.NormalPriority, filter)
	changed := make(chan struct{}, 1)

	go func() {
		defer store.Unsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-ch:
				if !ok {
					return
				}
				bundle.Acknowledge()
				pending.forget(bundle.Events)
				select {
				case changed <- struct{}{}:
				default:
				}
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
				if events := pending.merge(store); len(events) > 0 {
					store.Notify(events)
				}
			}
		}
	}()
}

// forgetRemovedImages unsets the image entities of this collector once the
// runtime removes their image.
func forgetRemovedImages(ctx context.Context, store workloadmeta.Component) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainerImageMetadata).
		SetSource(workloadmeta.SourceRuntime).
		SetEventType(workloadmeta.EventTypeUnset).
		Build()
	ch := store.Subscribe(collectorID, workloadmeta.NormalPriority, filter)

	go func() {
		defer store.Unsubscribe(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case bundle, ok := <-ch:
				if !ok {
					return
				}
				bundle.Acknowledge()
				if len(bundle.Events) == 0 {
					continue
				}
				events := make([]workloadmeta.CollectorEvent, 0, len(bundle.Events))
				for _, ev := range bundle.Events {
					events = append(events, workloadmeta.CollectorEvent{
						Type:   workloadmeta.EventTypeUnset,
						Source: workloadmeta.SourceRemoteSBOMCollector,
						Entity: &workloadmeta.ContainerImageMetadata{EntityID: ev.Entity.GetID()},
					})
				}
				store.Notify(events)
			}
		}
	}()
}

// NewCollector returns a remote process collector for workloadmeta if any
func NewCollector(ipc ipc.Component) (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			GenericCollector: &remote.GenericCollector{
				CollectorID: collectorID,
				// TODO(components): make sure StreamHandler uses the config component not pkg/config
				StreamHandler: &streamHandler{agentConfig: pkgconfigsetup.Datadog(), systemProbeConfig: pkgconfigsetup.SystemProbe(), pending: newPendingReports()},
				Config:        pkgconfigsetup.Datadog(), //nolint:depguard
				Catalog:       workloadmeta.NodeAgent,
				IPC:           ipc,
			},
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
	collectorEvents = s.handleEvents(store, collectorEvents, []*sbompb.SBOMMessage{response}, workloadmetaEventFromSBOMEventSet)
	log.Tracef("collected [%d] events", len(collectorEvents))
	return collectorEvents, nil
}

func (s *streamHandler) handleEvents(store workloadmeta.Component, collectorEvents []workloadmeta.CollectorEvent, sbomEvents []*sbompb.SBOMMessage, convertFunc func(workloadmeta.Component, *sbompb.SBOMMessage) (workloadmeta.Event, error)) []workloadmeta.CollectorEvent {
	for _, protoEvent := range sbomEvents {
		workloadmetaEvent, err := convertFunc(store, protoEvent)
		if errors.Is(err, errNotReady) && s.pending != nil {
			log.Debugf("keeping an SBOM report for later: %v", err)
			s.pending.add(protoEvent)
			continue
		}
		if err != nil {
			log.Warnf("error converting workloadmeta event: %v", err)
			continue
		}
		if s.pending != nil && protoEvent != nil {
			s.pending.remove(protoEvent.ID, nil)
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

// HandleResync notifies the events of the first response after a reconnect, as
// Run does for any other response. The SBOM stream sends each report as it
// comes, so the first one after a reconnect is one update among the others.
func (s *streamHandler) HandleResync(store workloadmeta.Component, events []workloadmeta.CollectorEvent) {
	store.Notify(events)
}
