// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
)

// CRDHandlers implements the Handlers interface for Kubernetes CustomResourceDefinitions.
type CRDHandlers struct {
	common.TypedHandlers[*v1.CustomResourceDefinition]
}

// BuildManifestMessageBody builds the manifest payload body
func (crd *CRDHandlers) BuildManifestMessageBody(ctx processors.ProcessorContext, resourceManifests []interface{}, groupSize int) model.MessageBody {
	pctx := ctx.(*processors.K8sProcessorContext)
	cm := common.ExtractModelManifests(ctx, resourceManifests, groupSize)
	return &model.CollectorManifestCRD{
		Manifest: cm,
		// CRDs are manifests, CollectorTags should be added to the inner Manifests, not the outer (embedded) CollectorManifests
		Tags: pctx.Cfg.ExtraTags,
	}
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (crd *CRDHandlers) AfterMarshalling(ctx processors.ProcessorContext, resource, resourceModel interface{}, yaml []byte) (skip bool) {
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of
// extracted resources.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (crd *CRDHandlers) BuildMessageBody(ctx processors.ProcessorContext, resourceModels []interface{}, groupSize int) model.MessageBody {
	return nil
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (crd *CRDHandlers) ExtractResource(ctx processors.ProcessorContext, resource interface{}) (resourceModel interface{}) {
	return
}

// ResourceList is a handler called to convert a list passed as a generic
// interface to a list of generic interfaces.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (crd *CRDHandlers) ResourceList(_ processors.ProcessorContext, list interface{}) []interface{} {
	return common.RuntimeObjectList(list)
}

// GetMetadataTags returns the tags in the metadata model.
//
//nolint:revive // TODO(CAPP) Fix revive linter
func (crd *CRDHandlers) GetMetadataTags(ctx processors.ProcessorContext, resourceMetadataModel interface{}) []string {
	return nil
}
