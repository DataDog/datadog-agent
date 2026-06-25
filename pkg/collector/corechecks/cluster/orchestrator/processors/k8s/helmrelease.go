// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/redact"
)

const redactedValue = "********"

// HelmReleaseHandlers handles synthetic Helm custom resources.
type HelmReleaseHandlers struct {
	CRHandlers
}

// ScrubBeforeExtraction always scrubs Helm release content before marshalling.
func (h *HelmReleaseHandlers) ScrubBeforeExtraction(ctx processors.ProcessorContext, resource interface{}) {
	pctx := ctx.(*processors.K8sProcessorContext)
	r := resource.(*unstructured.Unstructured)

	redact.ScrubCRManifest(r, pctx.Cfg.Scrubber)

	// Secret data keys are intentionally redacted regardless of key names.
	redactSecretData(r)
}

// redactSecretData wipes Secret payloads embedded in spec.resources.
func redactSecretData(r *unstructured.Unstructured) {
	spec, ok := r.Object["spec"].(map[string]interface{})
	if !ok {
		return
	}
	resources, ok := spec["resources"].([]interface{})
	if !ok {
		return
	}
	for _, res := range resources {
		m, ok := res.(map[string]interface{})
		if !ok {
			continue
		}
		if kind, _ := m["kind"].(string); kind != "Secret" {
			continue
		}
		if _, ok := m["data"]; ok {
			m["data"] = redactedValue
		}
		if _, ok := m["stringData"]; ok {
			m["stringData"] = redactedValue
		}
	}
}
