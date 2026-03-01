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
	"github.com/golang/protobuf/proto"
)

const (
	collectorID           = "sbom-collector"
	LastAccessProperty    = "LastSeenRunning"
	HasSetSuidBitProperty = "HasSetSuidBit"
	RunningAsRootProperty = "RunningAsRoot"
)

// normalizeVersion normalizes version strings to handle epoch differences
// e.g., "1:4.4.36-4build1" and "4.4.36-4build1" should both map to "4.4.36-4build1"
// Returns both the normalized version (without epoch) and the original version
func normalizeVersion(version string) (normalized string, hasEpoch bool) {
	// Check if version has epoch prefix (e.g., "1:4.4.36-4build1")
	if idx := strings.Index(version, ":"); idx > 0 {
		// Extract the part after the epoch
		return version[idx+1:], true
	}
	return version, false
}

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
	var finalBom *cyclonedx_v1_4.Bom
	var finalCompressedSBOM *workloadmeta.CompressedSBOM

	existingImage, err := store.GetImage(imageID)
	if err == nil && existingImage != nil && existingImage.SBOM != nil {
		// Decompress existing image SBOM to get CycloneDXBOM
		existingSBOM, err := sbomutil.UncompressSBOM(existingImage.SBOM)
		if err == nil && existingSBOM != nil && existingSBOM.CycloneDXBOM != nil {
			// Merge runtime properties from new BOM into existing image SBOM
			finalBom = mergeRuntimeProperties(existingSBOM.CycloneDXBOM, &newBom)
			log.Debugf("Merged runtime properties for image %s SBOM", imageID)
		} else {
			// Decompression failed or no CycloneDXBOM, use the new one directly
			finalBom = &newBom
			if err != nil {
				log.Warnf("Failed to decompress existing SBOM for image %s: %v, using new SBOM", imageID, err)
			} else {
				log.Debugf("No existing CycloneDXBOM for image %s, using new SBOM", imageID)
			}
		}
	} else {
		// No existing SBOM on image, use the new one directly
		finalBom = &newBom
		if err != nil {
			log.Debugf("Could not get image %s from store: %v, using new SBOM", imageID, err)
		} else {
			log.Debugf("No existing SBOM for image %s, using new SBOM", imageID)
		}
	}

	// Compress the final merged SBOM for storage
	finalCompressedSBOM, err = sbomutil.CompressSBOM(&workloadmeta.SBOM{
		CycloneDXBOM: finalBom,
	})
	if err != nil {
		return workloadmeta.Event{}, fmt.Errorf("failed to compress SBOM for image %s: %w", imageID, err)
	}

	// Return event to update the ContainerImageMetadata entity
	return workloadmeta.Event{
		Type: workloadmeta.EventTypeSet,
		Entity: &workloadmeta.ContainerImageMetadata{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainerImageMetadata,
				ID:   imageID,
			},
			SBOM: finalCompressedSBOM,
		},
	}, nil
}

// mergeRuntimeProperties merges runtime properties from newBom into existingBom.
// Returns a new BOM whose component list is deduplicated (by name+normalised version) and
// enriched with runtime properties (LastSeenRunning / HasSetSuidBit / RunningAsRoot) taken
// from newBom.  Deduplication is necessary because a previously-merged SBOM stored in the
// image entity may already carry both an original Trivy entry and a runtime-enriched copy of
// the same package; without it every merge round would re-emit both copies.
func mergeRuntimeProperties(existingBom, newBom *cyclonedx_v1_4.Bom) *cyclonedx_v1_4.Bom {
	if newBom == nil || len(newBom.Components) == 0 {
		return existingBom
	}

	// Build a lookup map from newBom (system-probe) components by name+normalised version.
	// We normalise versions to handle epoch differences (e.g. "1:4.4.36" vs "4.4.36").
	newComponentsMap := make(map[string]*cyclonedx_v1_4.Component)
	for _, comp := range newBom.Components {
		if comp != nil {
			normalizedVersion, _ := normalizeVersion(comp.Version)
			key := comp.Name + "@" + normalizedVersion
			newComponentsMap[key] = comp
		}
	}

	// Shallow-copy the BOM envelope; Components is rebuilt below.
	mergedBom := &cyclonedx_v1_4.Bom{
		SpecVersion:        existingBom.SpecVersion,
		Version:            existingBom.Version,
		SerialNumber:       existingBom.SerialNumber,
		Metadata:           existingBom.Metadata,
		Services:           existingBom.Services,
		ExternalReferences: existingBom.ExternalReferences,
		Dependencies:       existingBom.Dependencies,
		Compositions:       existingBom.Compositions,
		Vulnerabilities:    existingBom.Vulnerabilities,
	}

	// seen tracks which name@version keys have already been emitted so that duplicate
	// entries for the same package (which can accumulate across merge rounds) are dropped.
	seen := make(map[string]struct{}, len(existingBom.Components))

	for _, existingComp := range existingBom.Components {
		if existingComp == nil {
			continue
		}

		normalizedVersion, _ := normalizeVersion(existingComp.Version)
		key := existingComp.Name + "@" + normalizedVersion

		// Emit only the first occurrence of each package.
		if _, already := seen[key]; already {
			continue
		}
		seen[key] = struct{}{}

		// Copy all fields so we do not mutate the original BOM.
		mergedComp := &cyclonedx_v1_4.Component{
			Type:               existingComp.Type,
			MimeType:           existingComp.MimeType,
			BomRef:             existingComp.BomRef,
			Supplier:           existingComp.Supplier,
			Author:             existingComp.Author,
			Publisher:          existingComp.Publisher,
			Group:              existingComp.Group,
			Name:               existingComp.Name,
			Version:            existingComp.Version,
			Description:        existingComp.Description,
			Scope:              existingComp.Scope,
			Hashes:             existingComp.Hashes,
			Licenses:           existingComp.Licenses,
			Copyright:          existingComp.Copyright,
			Cpe:                existingComp.Cpe,
			Purl:               existingComp.Purl,
			Swid:               existingComp.Swid,
			Modified:           existingComp.Modified,
			Pedigree:           existingComp.Pedigree,
			ExternalReferences: existingComp.ExternalReferences,
			Components:         existingComp.Components,
			Properties:         existingComp.Properties,
			Evidence:           existingComp.Evidence,
			ReleaseNotes:       existingComp.ReleaseNotes,
		}

		// Add or update runtime properties from newBom.
		if newComp, exists := newComponentsMap[key]; exists && newComp.Properties != nil {
			updateProperty := func(propertyName string) {
				var newProp *cyclonedx_v1_4.Property
				for _, prop := range newComp.Properties {
					if prop != nil && prop.Name == propertyName {
						newProp = prop
						break
					}
				}
				if newProp == nil {
					return
				}
				if mergedComp.Properties == nil {
					mergedComp.Properties = []*cyclonedx_v1_4.Property{}
				}
				for j, prop := range mergedComp.Properties {
					if prop != nil && prop.Name == propertyName {
						mergedComp.Properties[j] = newProp
						log.Tracef("Updated %s for component %s@%s", propertyName, existingComp.Name, existingComp.Version)
						return
					}
				}
				mergedComp.Properties = append(mergedComp.Properties, newProp)
				log.Tracef("Added %s for component %s@%s", propertyName, existingComp.Name, existingComp.Version)
			}

			updateProperty(LastAccessProperty)
			updateProperty(HasSetSuidBitProperty)
			updateProperty(RunningAsRootProperty)
		}

		mergedBom.Components = append(mergedBom.Components, mergedComp)
	}

	return mergedBom
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
	// SBOM collector service is on the command socket, not the main runtime security socket
	cmdSocket := s.GetString("runtime_security_config.cmd_socket")
	if cmdSocket != "" {
		return cmdSocket
	}

	// If cmd_socket not explicitly set, derive it from main socket (adds "cmd-" prefix)
	mainSocket := s.GetString("runtime_security_config.socket")
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

	runtimeSecurityEnabled := s.Reader.GetBool("runtime_security_config.enabled")
	runtimeSecuritySBOMEnabled := s.Reader.GetBool("runtime_security_config.sbom.enabled")

	return runtimeSecurityEnabled && runtimeSecuritySBOMEnabled
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
