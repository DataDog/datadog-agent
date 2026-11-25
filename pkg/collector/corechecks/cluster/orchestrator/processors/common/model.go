// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ModelSpec describes how to build the structured metadata of a typed resource.
type ModelSpec[P TypedObject[P], M any] struct {
	// Extract builds the metadata model of a resource.
	Extract func(ctx processors.ProcessorContext, resource P) M
	// Tags returns the tags field of a model.
	Tags func(m M) *[]string
	// YAML returns the YAML field of a model. Nil when the model does not carry the YAML.
	YAML func(m M) *[]byte
	// Entity returns the tagger entity of a resource, whose tags are added to the model. Nil adds no tagger tags.
	Entity func(ctx processors.ProcessorContext, resource P) taggertypes.EntityID
	// Scrub redacts the resource before marshalling when scrubbing is enabled. Nil scrubs nothing.
	Scrub func(resource P, scrubber *redact.DataScrubber)
	// Message builds the payload of a group of models.
	Message func(header MessageHeader, models []M) model.MessageBody
}

// MessageHeader holds the fields that every structured metadata payload shares.
type MessageHeader struct {
	ClusterName  string
	ClusterID    string
	GroupID      int32
	GroupSize    int32
	Tags         []string
	AgentVersion *model.AgentVersion
}

// ModelHandlers implements the handlers of a typed resource from its ModelSpec.
type ModelHandlers[P TypedObject[P], M any] struct {
	TypedHandlers[P]
	spec   ModelSpec[P, M]
	tagger tagger.Component
}

// NewModelHandlers creates the handlers of a typed resource. It panics when the spec misses a required function, so
// that the mistake fails when the check starts instead of silently dropping the payloads of the resource.
func NewModelHandlers[P TypedObject[P], M any](tagger tagger.Component, spec ModelSpec[P, M]) *ModelHandlers[P, M] {
	if spec.Extract == nil || spec.Tags == nil || spec.Message == nil {
		panic(fmt.Sprintf("the model spec of %T misses Extract, Tags or Message", *new(P)))
	}
	return &ModelHandlers[P, M]{spec: spec, tagger: tagger}
}

// ExtractResource builds the metadata model of the resource.
func (h *ModelHandlers[P, M]) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	return h.spec.Extract(ctx, resource.(P))
}

// EnrichModel adds the tagger tags of the resource to its model.
func (h *ModelHandlers[P, M]) EnrichModel(ctx processors.ProcessorContext, resource, resourceModel interface{}) (skip bool) {
	if h.spec.Entity == nil {
		return
	}
	r := resource.(P)
	taggerTags, err := h.tagger.Tag(h.spec.Entity(ctx, r), taggertypes.HighCardinality)
	if err != nil {
		log.Debugf("Could not retrieve tags for %s %s: %s", strings.ToLower(ctx.GetKind()), objectRef(r), err)
		return
	}

	tags := h.spec.Tags(resourceModel.(M))
	*tags = append(*tags, taggerTags...)
	return
}

// objectRef returns namespace/name for a namespaced resource and name for a cluster-scoped one.
func objectRef(r metav1.Object) string {
	if namespace := r.GetNamespace(); namespace != "" {
		return namespace + "/" + r.GetName()
	}
	return r.GetName()
}

// AfterMarshalling copies the YAML of the resource into its model.
func (h *ModelHandlers[P, M]) AfterMarshalling(_ processors.ProcessorContext, _, resourceModel interface{}, yaml []byte) (skip bool) {
	if h.spec.YAML != nil {
		*h.spec.YAML(resourceModel.(M)) = yaml
	}
	return
}

// GetMetadataTags returns the tags of the model.
func (h *ModelHandlers[P, M]) GetMetadataTags(_ processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	m, ok := resourceMetadataModel.(M)
	if !ok {
		return nil
	}
	return *h.spec.Tags(m)
}

// ScrubBeforeMarshalling redacts the resource before it is marshalled when scrubbing is enabled.
func (h *ModelHandlers[P, M]) ScrubBeforeMarshalling(ctx processors.ProcessorContext, resource interface{}) {
	if h.spec.Scrub == nil {
		return
	}
	if pctx := ctx.(*processors.K8sProcessorContext); pctx.Cfg.IsScrubbingEnabled {
		h.spec.Scrub(resource.(P), pctx.Cfg.Scrubber)
	}
}

// BuildMessageBody builds the payload of a group of models.
func (h *ModelHandlers[P, M]) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	models := make([]M, 0, len(resourceModels))
	for _, m := range resourceModels {
		models = append(models, m.(M))
	}

	return h.spec.Message(MessageHeader{
		ClusterName:  pctx.Cfg.KubeClusterName,
		ClusterID:    pctx.ClusterID,
		GroupID:      pctx.MsgGroupID,
		GroupSize:    int32(groupSize),
		Tags:         util.ImmutableTagsJoin(pctx.Cfg.ExtraTags, pctx.GetCollectorTags()),
		AgentVersion: ctx.GetAgentVersion(),
	}, models)
}

// MetadataEntity returns the tagger entity of a namespaced resource, from the Kubernetes metadata collector.
func MetadataEntity[P TypedObject[P]](ctx processors.ProcessorContext, r P) taggertypes.EntityID {
	return taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), r.GetNamespace(), r.GetName())),
	)
}

// ClusterMetadataEntity returns the tagger entity of a cluster-scoped resource, from the Kubernetes metadata collector.
func ClusterMetadataEntity[P TypedObject[P]](ctx processors.ProcessorContext, r P) taggertypes.EntityID {
	return taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), "", r.GetName())),
	)
}
