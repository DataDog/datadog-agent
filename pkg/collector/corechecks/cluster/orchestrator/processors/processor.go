// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package processors

import (
	"reflect"
	"time"

	jsoniter "github.com/json-iterator/go"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	pkgorchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Marshal message to JSON.
// We need to enforce order consistency on underlying maps as
// the standard library does.
var json = jsoniter.ConfigCompatibleWithStandardLibrary

// ProcessorContext implements context for processor
type ProcessorContext interface {
	GetOrchestratorConfig() *config.OrchestratorConfig
	GetNodeType() pkgorchestratormodel.NodeType
	GetMsgGroupID() int32
	IsManifestProducer() bool
	GetKind() string
	GetAPIVersion() string
	IsTerminatedResources() bool
	GetCollectorTags() []string
	GetAgentVersion() *model.AgentVersion
}

// BaseProcessorContext is the base context for all processors
type BaseProcessorContext struct {
	Cfg                 *config.OrchestratorConfig
	NodeType            pkgorchestratormodel.NodeType
	MsgGroupID          int32
	ClusterID           string
	ManifestProducer    bool
	Kind                string
	APIVersion          string
	CollectorTags       []string
	TerminatedResources bool
	AgentVersion        *model.AgentVersion
}

// GetOrchestratorConfig returns the orchestrator config
func (c *BaseProcessorContext) GetOrchestratorConfig() *config.OrchestratorConfig {
	return c.Cfg
}

// GetNodeType returns the node type
func (c *BaseProcessorContext) GetNodeType() pkgorchestratormodel.NodeType {
	return c.NodeType
}

// GetMsgGroupID returns the message group ID
func (c *BaseProcessorContext) GetMsgGroupID() int32 {
	return c.MsgGroupID
}

// GetClusterID returns the cluster ID
func (c *BaseProcessorContext) GetClusterID() string {
	return c.ClusterID
}

// IsManifestProducer returns true if the collector is a manifest producer
func (c *BaseProcessorContext) IsManifestProducer() bool {
	return c.ManifestProducer
}

// GetKind returns the kind
func (c *BaseProcessorContext) GetKind() string {
	return c.Kind
}

// GetAPIVersion returns the version
func (c *BaseProcessorContext) GetAPIVersion() string {
	return c.APIVersion
}

// GetCollectorTags returns the CollectorTags
func (c *BaseProcessorContext) GetCollectorTags() []string {
	return c.CollectorTags
}

// IsTerminatedResources returns true if resources are terminated
func (c *BaseProcessorContext) IsTerminatedResources() bool {
	return c.TerminatedResources
}

// GetAgentVersion returns the agent version
func (c *BaseProcessorContext) GetAgentVersion() *model.AgentVersion {
	return c.AgentVersion
}

// K8sProcessorContext holds k8s resource processing attributes
type K8sProcessorContext struct {
	BaseProcessorContext
	APIClient         *apiserver.APIClient
	HostName          string
	SystemInfo        *model.SystemInfo
	ResourceType      string
	LabelsAsTags      map[string]string
	AnnotationsAsTags map[string]string
	NodeName          string
}

// ECSProcessorContext holds ECS resource processing attributes
type ECSProcessorContext struct {
	BaseProcessorContext
	AWSAccountID string
	ClusterName  string
	Region       string
	SystemInfo   *model.SystemInfo
	Hostname     string
}

// Handlers is the interface that is to be implemented for every resource type
// and provides a way to plug in code at different levels of the Processor
// logic.
type Handlers interface {
	// AfterMarshalling runs before the Processor marshals the resource to
	// generate a manifest. If skip is true then the resource processing loop
	// moves on to the next resource.
	AfterMarshalling(ctx ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool)

	// BeforeCacheCheck runs before the Processor does a cache lookup for the
	// resource. If skip is true then the resource processing loop moves on to
	// the next resource.
	BeforeCacheCheck(ctx ProcessorContext, resource, resourceModel interface{}) (skip bool)

	// BeforeMarshalling runs before the Processor marshals the resource to
	// generate a manifest. If skip is true then the resource processing loop
	// moves on to the next resource.
	BeforeMarshalling(ctx ProcessorContext, resource, resourceModel interface{}) (skip bool)

	// BuildMessageBody is used to build a message containing a chunk of
	// resource models of a certain size. If skip is true then the resource
	// processing loop moves on to the next resource.
	BuildMessageBody(ctx ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody

	// BuildManifestMessageBody is used to build a message containing a chunk of
	// resource manifests of a certain size.
	BuildManifestMessageBody(ctx ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody

	// ExtractResource is used to build a resource model from the raw
	// resource representation.
	ExtractResource(ctx ProcessorContext, resource interface{}) (resourceModel interface{})

	// ResourceList is used to convert a list of raw resources to a generic list
	// that can be used throughout a Processor.
	ResourceList(ctx ProcessorContext, list interface{}) (resources []interface{})

	// ResourceUID returns the resource UID.
	ResourceUID(ctx ProcessorContext, resource interface{}) types.UID

	// ResourceVersion returns the resource Version.
	ResourceVersion(ctx ProcessorContext, resource, resourceModel interface{}) string

	// GetMetadataTags returns the resource tags with the metadata
	GetMetadataTags(ctx ProcessorContext, resourceMetadataModel interface{}) []string

	GetNodeName(ctx ProcessorContext, resource interface{}) string

	// ScrubBeforeExtraction replaces sensitive information in the resource
	// before resource extraction.
	ScrubBeforeExtraction(ctx ProcessorContext, resource interface{})

	// ScrubBeforeMarshalling replaces sensitive information in the resource
	// before resource marshalling.
	ScrubBeforeMarshalling(ctx ProcessorContext, resource interface{})
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

// Handlers returns handlers used by the processor.
func (p *Processor) Handlers() Handlers {
	return p.h
}

// Process is used to process a list of resources of a certain type.
func (p *Processor) Process(ctx ProcessorContext, list interface{}) (processResult ProcessResult, listed, processed int) {
	// This default allows detection of panic recoveries.
	processed = -1

	// Make sure to recover if a panic occurs.
	defer RecoverOnPanic()

	resourceList := p.h.ResourceList(ctx, list)
	resourceMetadataModels := make([]interface{}, 0, len(resourceList))
	resourceManifestModels := make([]interface{}, 0, len(resourceList))
	now := time.Now()

	for _, resource := range resourceList {
		if ctx.IsTerminatedResources() {
			resource = insertDeletionTimestampIfPossible(resource, now)
		}
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

		if orchestrator.SkipKubernetesResource(resourceUID, resourceVersion, ctx.GetNodeType()) {
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
			log.Warnc(NewMarshallingError(err).Error(), orchestrator.ExtraLogContext...)
			continue
		}

		// Stop sending yaml if manifest collecion is enabled
		if !ctx.GetOrchestratorConfig().IsManifestCollectionEnabled {
			// Execute code after marshalling.
			if skip := p.h.AfterMarshalling(ctx, resource, resourceMetadataModel, yaml); skip {
				continue
			}
		}

		resourceMetadataModels = append(resourceMetadataModels, resourceMetadataModel)

		// Add resource manifest
		resourceManifestModels = append(resourceManifestModels, &model.Manifest{
			Type:            int32(ctx.GetNodeType()),
			Uid:             string(resourceUID),
			ResourceVersion: resourceVersion,
			Content:         yaml,
			Version:         "v1",
			ContentType:     "json",
			// include collector tags as buffered Manifests share types, and only ExtraTags should be included in CollectorManifests
			Tags:         util.ImmutableTagsJoin(ctx.GetCollectorTags(), p.h.GetMetadataTags(ctx, resourceMetadataModel)),
			IsTerminated: ctx.IsTerminatedResources(),
			Kind:         ctx.GetKind(),
			ApiVersion:   ctx.GetAPIVersion(),
			NodeName:     p.h.GetNodeName(ctx, resource),
		})
	}

	processResult = ProcessResult{
		MetadataMessages: ChunkMetadata(ctx, p, resourceMetadataModels, resourceManifestModels),
	}
	if ctx.IsManifestProducer() {
		processResult.ManifestMessages = ChunkManifest(ctx, p.h.BuildManifestMessageBody, resourceManifestModels)
	}

	return processResult, len(resourceList), len(resourceMetadataModels)
}

// ChunkManifest is to chunk Manifest payloads
func ChunkManifest(ctx ProcessorContext, buildManifestBody func(ctx ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody, resourceManifestModels []interface{}) []model.MessageBody {
	// Chunking resources based on the serialized size of their manifest and maximum messages number
	// Chunk manifest messages and use itself as weight indicator
	chunks := chunkOrchestratorPayloadsBySizeAndWeight(resourceManifestModels, resourceManifestModels, ctx.GetOrchestratorConfig().MaxPerMessage, ctx.GetOrchestratorConfig().MaxWeightPerMessageBytes)

	chunkCount := len(chunks)
	manifestMessages := make([]model.MessageBody, 0, chunkCount)

	for i := 0; i < chunkCount; i++ {
		manifestMessages = append(manifestMessages, buildManifestBody(ctx, chunks[i], chunkCount))
	}

	return manifestMessages
}

// ChunkMetadata is to chunk Metadata payloads
func ChunkMetadata(ctx ProcessorContext, p *Processor, resourceMetadataModels, resourceManifestModels []interface{}) []model.MessageBody {
	// Chunking resources based on the serialized size of their manifest and maximum messages number
	// Chunk metadata messages and use resourceManifestModels as weight indicator
	chunks := chunkOrchestratorPayloadsBySizeAndWeight(resourceMetadataModels, resourceManifestModels, ctx.GetOrchestratorConfig().MaxPerMessage, ctx.GetOrchestratorConfig().MaxWeightPerMessageBytes)

	chunkCount := len(chunks)
	metadataMessages := make([]model.MessageBody, 0, chunkCount)
	for i := 0; i < chunkCount; i++ {
		metadataMessages = append(metadataMessages, p.h.BuildMessageBody(ctx, chunks[i], chunkCount))
	}

	return metadataMessages
}

func insertDeletionTimestampIfPossible(obj interface{}, ts time.Time) interface{} {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		log.Debugf("object is not a pointer to a nil pointer, got type: %T", obj)
		return obj
	}

	v = v.Elem()
	if v.Kind() != reflect.Struct {
		log.Debugf("obj must point to a struct, got type: %T", obj)
		return obj
	}

	metaTs := metav1.NewTime(ts)

	if _, ok := obj.(*unstructured.Unstructured); ok {
		obj.(*unstructured.Unstructured).SetDeletionTimestamp(&metaTs)
		return obj
	}

	// Look for metadata field
	metadataField := v.FieldByName("ObjectMeta")
	if !metadataField.IsValid() || metadataField.Kind() != reflect.Struct {
		log.Debugf("obj does not have ObjectMeta field, got type: %T", obj)
		return obj
	}

	// Access deletionTimestamp field within ObjectMeta
	deletionTimestampField := metadataField.FieldByName("DeletionTimestamp")
	if !deletionTimestampField.IsValid() || !deletionTimestampField.CanSet() {
		log.Debugf("ObjectMeta does not have a settable DeletionTimestamp, got field: %T", obj)
		return obj
	}

	// Do nothing if it's already set
	if !deletionTimestampField.IsNil() {
		return obj
	}

	// Set the deletionTimestamp
	deletionTimestampField.Set(reflect.ValueOf(&metaTs))
	return obj
}
