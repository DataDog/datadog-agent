// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/orchestrator/configmapdata"
	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// ConfigMapHandlers implements the Handlers interface for Kubernetes ConfigMaps.
// ConfigMap is manifest-only (IsMetadataProducer: false): no structured metadata model is
// produced or forwarded. Data and BinaryData are stripped before the manifest is emitted, unless
// the ConfigMap is on the remote config allow-list carried by the processor context.
type ConfigMapHandlers struct {
	common.BaseHandlers

	// scrubber redacts secret-looking values from the data of allow-listed ConfigMaps. Built once
	// because installing the custom sensitive words compiles a regular expression.
	scrubber *scrubber.Scrubber
}

// NewConfigMapHandlers creates a new ConfigMapHandlers.
func NewConfigMapHandlers() *ConfigMapHandlers {
	return &ConfigMapHandlers{scrubber: newConfigMapScrubber()}
}

// newConfigMapScrubber returns the scrubber applied to ConfigMap data that is kept. It mirrors the
// kubeactions get_resource executor: the default replacers plus the words the user configured for
// the orchestrator explorer.
func newConfigMapScrubber() *scrubber.Scrubber {
	scrb := scrubber.NewWithDefaults()

	if pkgconfigsetup.Datadog().IsConfigured("orchestrator_explorer.custom_sensitive_words") {
		sensitiveWords := pkgconfigsetup.Datadog().GetStringSlice("orchestrator_explorer.custom_sensitive_words")
		if len(sensitiveWords) > 0 {
			regex, err := regexp.Compile(strings.Join(sensitiveWords, "|"))
			if err != nil {
				log.Warnf("Could not compile orchestrator_explorer.custom_sensitive_words, ConfigMap data will only be scrubbed with the defaults: %v", err)
			} else {
				scrb.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
					Regex: regex,
					Repl:  []byte("********"),
				})
			}
		}
	}

	return scrb
}

// isDataCollected reports whether this ConfigMap is opted into full data collection, according to
// the allow-list snapshotted at the start of the run.
func isDataCollected(ctx processors.ProcessorContext, cm *corev1.ConfigMap) bool {
	pctx, ok := ctx.(*processors.K8sProcessorContext)
	if !ok {
		return false
	}
	return pctx.ConfigMapAllowSet.IsAllowed(cm.Namespace, cm.Name)
}

// resolveResourceVersion returns the resource version to report for a ConfigMap, with the high bit
// set when the ConfigMap is opted into full data collection. Both the agent-side resource cache and
// the backend last-seen caches compare this value for equality only, so tagging it makes an
// allow-list flip look like a change even though the object itself did not change. etcd revisions
// are nowhere near 2^63, so the tag can never collide with a real resource version, and masking it
// off recovers the original. The value has to stay a base-10 uint64 string because the backend
// parses it as one. A resource version is opaque per the Kubernetes API contract, so a non-numeric
// one is passed through unchanged.
func resolveResourceVersion(resourceVersion string, dataCollected bool) string {
	if !dataCollected {
		return resourceVersion
	}
	n, err := strconv.ParseUint(resourceVersion, 10, 64)
	if err != nil {
		return resourceVersion
	}
	return strconv.FormatUint(n|(1<<63), 10)
}

// AfterMarshalling is a handler called after resource marshalling.
//
//nolint:revive
func (h *ConfigMapHandlers) AfterMarshalling(_ processors.ProcessorContext, _, _ interface{}, _ []byte) (skip bool) {
	return
}

// BeforeMarshalling is a handler called before resource marshalling.
// Sets Kind and APIVersion on the object, which the Kubernetes API omits on typed responses.
//
//nolint:revive
func (h *ConfigMapHandlers) BeforeMarshalling(ctx processors.ProcessorContext, resource, _ interface{}) (skip bool) {
	r := resource.(*corev1.ConfigMap)
	r.Kind = ctx.GetKind()
	r.APIVersion = ctx.GetAPIVersion()
	return
}

// BuildMessageBody is a handler called to build a message body out of a list of extracted resources.
// ConfigMap is manifest-only so no metadata message body is ever sent.
//
//nolint:revive
func (h *ConfigMapHandlers) BuildMessageBody(_ processors.ProcessorContext, _ []interface{}, _ int) model.MessageBody {
	return nil
}

// ExtractResource is a handler called to extract the resource model out of a raw resource.
// ConfigMap is manifest-only; no structured model is produced.
//
//nolint:revive
func (h *ConfigMapHandlers) ExtractResource(_ processors.ProcessorContext, _ interface{}) interface{} {
	return nil
}

// ResourceList converts the raw list to a slice of generic interfaces.
// It also snapshots the data collection allow-list, because this runs exactly once per run while
// the allow-list is consulted three times per ConfigMap. Remote config can swap the list at any
// point, and a per-call read could emit a manifest whose cache token disagrees with its content.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceList(ctx processors.ProcessorContext, list interface{}) []interface{} {
	pctx := ctx.(*processors.K8sProcessorContext)
	pctx.ConfigMapAllowSet = configmapdata.Get().Snapshot(pctx.GetClusterID())

	resourceList := list.([]*corev1.ConfigMap)
	resources := make([]interface{}, 0, len(resourceList))
	for _, r := range resourceList {
		resources = append(resources, r)
	}
	return resources
}

// CloneResource returns a deep copy of the ConfigMap so mutations during scrubbing
// do not affect the informer cache.
//
//nolint:revive
func (h *ConfigMapHandlers) CloneResource(resource interface{}) interface{} {
	return resource.(*corev1.ConfigMap).DeepCopy()
}

// ResourceVersionFromRaw returns the resource version without requiring model extraction.
// The version is tagged when the ConfigMap is opted into full data collection, so that the flip
// invalidates the agent cache. This must return the same value as ResourceVersion for a given
// ConfigMap in a given run: the value returned here is what lands in the agent cache, while the one
// returned by ResourceVersion is what goes on the wire, and the two have to agree.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceVersionFromRaw(ctx processors.ProcessorContext, resource interface{}) string {
	r := resource.(*corev1.ConfigMap)
	return resolveResourceVersion(r.ResourceVersion, isDataCollected(ctx, r))
}

// ResourceUID returns the UID of the ConfigMap.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceUID(_ processors.ProcessorContext, resource interface{}) types.UID {
	return resource.(*corev1.ConfigMap).UID
}

// ResourceVersion returns the resource version of the ConfigMap, tagged when the ConfigMap is
// opted into full data collection. This is the value carried by the payload, and every backend
// last-seen cache compares it for equality, so the tag is what makes an opt-in or an opt-out
// propagate on the next tick instead of being deduped away.
//
//nolint:revive
func (h *ConfigMapHandlers) ResourceVersion(ctx processors.ProcessorContext, resource, _ interface{}) string {
	r := resource.(*corev1.ConfigMap)
	return resolveResourceVersion(r.ResourceVersion, isDataCollected(ctx, r))
}

// ScrubBeforeExtraction redacts sensitive annotation and label keys before the resource is processed.
//
//nolint:revive
func (h *ConfigMapHandlers) ScrubBeforeExtraction(_ processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.ConfigMap)
	redact.RemoveSensitiveAnnotationsAndLabels(r.Annotations, r.Labels)
}

// ScrubBeforeMarshalling strips Data, BinaryData, and ManagedFields so that ConfigMap
// values and field-manager history are never included in the emitted manifest.
// All three are kept when the ConfigMap is opted into full data collection, in which case Data is
// scrubbed for secret-looking values first. ManagedFields is kept because it records which manager
// last set each data key, which is part of what someone opting a ConfigMap in is looking for.
//
//nolint:revive
func (h *ConfigMapHandlers) ScrubBeforeMarshalling(ctx processors.ProcessorContext, resource interface{}) {
	r := resource.(*corev1.ConfigMap)
	if isDataCollected(ctx, r) {
		h.scrubData(r)
		return
	}
	r.Data = nil
	r.BinaryData = nil
	r.ManagedFields = nil
}

// scrubData redacts secret-looking values from the ConfigMap data that is about to be emitted.
//
// Scrubbing is line by line rather than through ScrubBytes because ScrubBytes reads its input as a
// config file: it drops comment lines and collapses blank ones, which would silently rewrite a
// ConfigMap holding a YAML or nginx config. ScrubLine leaves everything it does not redact alone.
//
// BinaryData is left as it is: its values are arbitrary bytes rather than text, so running the
// replacers over them would risk corrupting the content without meaningfully redacting it.
func (h *ConfigMapHandlers) scrubData(cm *corev1.ConfigMap) {
	for k, v := range cm.Data {
		lines := strings.Split(v, "\n")
		for i, line := range lines {
			lines[i] = h.scrubber.ScrubLine(line)
		}
		cm.Data[k] = strings.Join(lines, "\n")
	}
}
