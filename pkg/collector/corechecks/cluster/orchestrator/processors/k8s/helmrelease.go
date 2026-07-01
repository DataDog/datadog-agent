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
		if m, ok := res.(map[string]interface{}); ok {
			redactResourceSecrets(m)
		}
	}
}

// redactResourceSecrets redacts a Secret manifest's payload, recursing into the
// items of a Kubernetes List so nested Secrets are not shipped in the clear.
func redactResourceSecrets(m map[string]interface{}) {
	switch kind, _ := m["kind"].(string); kind {
	case "Secret":
		if _, ok := m["data"]; ok {
			m["data"] = redactedValue
		}
		if _, ok := m["stringData"]; ok {
			m["stringData"] = redactedValue
		}
	case "List":
		items, ok := m["items"].([]interface{})
		if !ok {
			return
		}
		for _, item := range items {
			if im, ok := item.(map[string]interface{}); ok {
				redactResourceSecrets(im)
			}
		}
	}
}
