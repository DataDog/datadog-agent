// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
)

// ManifestHandlers implements the handlers of a typed resource that only has a manifest, with no structured metadata
// payload, such as ConfigMaps, Secrets and Helm releases.
type ManifestHandlers[P TypedObject[P]] struct {
	TypedHandlers[P]
}

// ExtractResource returns no model, as the resource has no structured metadata.
func (ManifestHandlers[P]) ExtractResource(processors.ProcessorContext, interface{}) interface{} {
	return nil
}

// AfterMarshalling does nothing, as there is no model to copy the YAML into.
func (ManifestHandlers[P]) AfterMarshalling(processors.ProcessorContext, interface{}, interface{}, []byte) (skip bool) {
	return
}

// BuildMessageBody returns no payload, as the resource has no structured metadata.
func (ManifestHandlers[P]) BuildMessageBody(processors.ProcessorContext, []interface{}, int) model.MessageBody {
	return nil
}

// ScrubBeforeMarshalling removes the managed fields, so that the field manager history is never sent.
func (ManifestHandlers[P]) ScrubBeforeMarshalling(_ processors.ProcessorContext, resource interface{}) {
	resource.(P).SetManagedFields(nil)
}
