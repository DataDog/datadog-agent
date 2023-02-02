// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator
// +build orchestrator

package processors

import (
	model "github.com/DataDog/agent-payload/v5/process"
	jsoniter "github.com/json-iterator/go"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Marshal message to JSON.
// We need to enforce order consistency on underlying maps as
// the standard library does.
var json = jsoniter.ConfigCompatibleWithStandardLibrary

// ProcessorContext holds resource processing attributes
type ProcessorContext struct {
	APIClient          *apiserver.APIClient
	Cfg                *config.OrchestratorConfig
	ClusterID          string
	HostName           string
	MsgGroupID         int32
	NodeType           orchestrator.NodeType
	ApiGroupVersionTag string
}

// Handlers is the interface that is to be implemented for every resource type
// and provides a way to plug in code at different levels of the Processor
// logic.
type Handlers interface {
	// AfterMarshalling runs before the Processor marshals the resource to
	// generate a manifest. If skip is true then the resource processing loop
	// moves on to the next resource.
	AfterMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool)

	// BeforeCacheCheck runs before the Processor does a cache lookup for the
	// resource. If skip is true then the resource processing loop moves on to
	// the next resource.
	BeforeCacheCheck(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool)

	// BeforeMarshalling runs before the Processor marshals the resource to
	// generate a manifest. If skip is true then the resource processing loop
	// moves on to the next resource.
	BeforeMarshalling(ctx *ProcessorContext, resource, resourceModel interface{}) (skip bool)

	// BuildMessageBody is used to build a message containing a chunk of
	// resource models of a certain size. If skip is true then the resource
	// processing loop moves on to the next resource.
	BuildMessageBody(ctx *ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody

	// BuildManifestMessageBody is used to build a message containing a chunk of
	// resource manifests of a certain size.
	BuildManifestMessageBody(ctx *ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody

	// ExtractResource is used to build a resource model from the raw
	// resource representation.
	ExtractResource(ctx *ProcessorContext, resource interface{}) (resourceModel interface{})

	// ResourceList is used to convert a list of raw resources to a generic list
	// that can be used throughout a Processor.
	ResourceList(ctx *ProcessorContext, list interface{}) (resources []interface{})

	// ResourceUID returns the resource UID.
	ResourceUID(ctx *ProcessorContext, resource interface{}) types.UID

	// ResourceVersion returns the resource Version.
	ResourceVersion(ctx *ProcessorContext, resource, resourceModel interface{}) string

	// ScrubBeforeExtraction replaces sensitive information in the resource
	// before resource extraction.
	ScrubBeforeExtraction(ctx *ProcessorContext, resource interface{})

	// ScrubBeforeMarshalling replaces sensitive information in the resource
	// before resource marshalling.
	ScrubBeforeMarshalling(ctx *ProcessorContext, resource interface{})
}

// Processor is a generic resource processing component. It relies on a set of
// handlers to enrich its processing logic and make it a processor for resources
// of a specific type.
type Processor struct {
	h Handlers
}

// ProcessResult contains the processing result of metadata and manifest
// MetadataMessages is a list of payload, each payload contains a list of k8s resources metadata and manifest
// ManifestMessages is a list of payload, each payload contains a list of k8s resources manifest.
// ManifestMessages is a copy of part of MetadataMessages
type ProcessResult struct {
	MetadataMessages []model.MessageBody
	ManifestMessages []model.MessageBody
}

// NewProcessor creates a new processor for a resource type.
func NewProcessor(h Handlers) *Processor {
	return &Processor{
		h: h,
	}
}

// Process is used to process a list of resources of a certain type.
func (p *Processor) Process(ctx *ProcessorContext, list interface{}) (processResult ProcessResult, processed int) {
	// This default allows detection of panic recoveries.
	processed = -1

	// Make sure to recover if a panic occurs.
	defer RecoverOnPanic()

	resourceList := p.h.ResourceList(ctx, list)
	resourceMetadataModels := make([]interface{}, 0, len(resourceList))
	resourceManifestModels := make([]interface{}, 0, len(resourceList))

	for _, resource := range resourceList {
		// Scrub before extraction.
		p.h.ScrubBeforeExtraction(ctx, resource)

		// Extract the message model from the resource.
		resourceMetadataModel := p.h.ExtractResource(ctx, resource)

		// Execute code before cache check.
		if skip := p.h.BeforeCacheCheck(ctx, resource, resourceMetadataModel); skip {
			continue
		}

		// Cache check
		resourceUID := p.h.ResourceUID(ctx, resource)
		resourceVersion := p.h.ResourceVersion(ctx, resource, resourceMetadataModel)

		if orchestrator.SkipKubernetesResource(resourceUID, resourceVersion, ctx.NodeType) {
			continue
		}

		// Execute code before marshalling.
		if skip := p.h.BeforeMarshalling(ctx, resource, resourceMetadataModel); skip {
			continue
		}

		// Scrub the resource before marshalling.
		p.h.ScrubBeforeMarshalling(ctx, resource)

		// Marshal the resource to generate the YAML field.
		yaml, err := json.Marshal(resource)
		if err != nil {
			log.Warnf(newMarshallingError(err).Error())
			continue
		}

		// Stop sending yaml if manifest collecion is enabled
		if !ctx.Cfg.IsManifestCollectionEnabled {
			// Execute code after marshalling.
			if skip := p.h.AfterMarshalling(ctx, resource, resourceMetadataModel, yaml); skip {
				continue
			}
		}

		resourceMetadataModels = append(resourceMetadataModels, resourceMetadataModel)

		// Add resource manifest
		resourceManifestModels = append(resourceManifestModels, &model.Manifest{
			Type:            int32(ctx.NodeType),
			Uid:             string(resourceUID),
			ResourceVersion: resourceVersion,
			Content:         yaml,
			Version:         "v1",
			ContentType:     "json",
		})
	}

	processResult = ProcessResult{
		MetadataMessages: ChunkMetadata(ctx, p, resourceMetadataModels, resourceManifestModels),
		ManifestMessages: ChunkManifest(ctx, p.h.BuildManifestMessageBody, resourceManifestModels),
	}

	return processResult, len(resourceMetadataModels)
}

// ChunkManifest is to chunk Manifest payloads
func ChunkManifest(ctx *ProcessorContext, buildManifestBody func(ctx *ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody, resourceManifestModels []interface{}) []model.MessageBody {
	// Chunking resources based on the serialized size of their manifest and maximum messages number
	// Chunk manifest messages and use itself as weight indicator
	chunks := chunkOrchestratorPayloadsBySizeAndWeight(resourceManifestModels, resourceManifestModels, ctx.Cfg.MaxPerMessage, ctx.Cfg.MaxWeightPerMessageBytes)

	chunkCount := len(chunks)
	manifestMessages := make([]model.MessageBody, 0, chunkCount)

	for i := 0; i < chunkCount; i++ {
		manifestMessages = append(manifestMessages, buildManifestBody(ctx, chunks[i], chunkCount))
	}

	return manifestMessages
}

// ChunkMetadata is to chunk Metadata payloads
func ChunkMetadata(ctx *ProcessorContext, p *Processor, resourceMetadataModels, resourceManifestModels []interface{}) []model.MessageBody {
	// Chunking resources based on the serialized size of their manifest and maximum messages number
	// Chunk metadata messages and use resourceManifestModels as weight indicator
	chunks := chunkOrchestratorPayloadsBySizeAndWeight(resourceMetadataModels, resourceManifestModels, ctx.Cfg.MaxPerMessage, ctx.Cfg.MaxWeightPerMessageBytes)

	chunkCount := len(chunks)
	metadataMessages := make([]model.MessageBody, 0, chunkCount)
	for i := 0; i < chunkCount; i++ {
		metadataMessages = append(metadataMessages, p.h.BuildMessageBody(ctx, chunks[i], chunkCount))
	}

	return metadataMessages
}
