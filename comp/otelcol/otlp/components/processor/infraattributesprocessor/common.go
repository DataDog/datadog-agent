// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"go.opentelemetry.io/collector/pdata/pcommon"
	conventions "go.opentelemetry.io/otel/semconv/v1.21.0"
	conventions22 "go.opentelemetry.io/otel/semconv/v1.22.0"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var unifiedServiceTagMap = map[string][]string{
	tags.Service: {string(conventions.ServiceNameKey)},
	tags.Env:     {string(conventions.DeploymentEnvironmentKey), "deployment.environment.name"},
	tags.Version: {string(conventions.ServiceVersionKey)},
}

// knownConventionKeys is the set of resource-attribute keys that the
// downstream trace-agent / Datadog exporter already promotes into
// `_dd.tags.container` through its own convention mappings. The infra
// attributes processor's container-tag-promotion logic (see
// ContainerTagPromotionMode) skips these keys, since prefixing them would
// either create duplicate entries (in `duplicate` mode) or break downstream
// consumers reading the raw key (in `rename` mode).
var knownConventionKeys = func() map[string]struct{} {
	m := make(map[string]struct{}, 2*len(attributes.ContainerMappings))
	for otelKey, ddName := range attributes.ContainerMappings {
		m[ddName] = struct{}{}  // trace-agent source 3 (containerDDTags)
		m[otelKey] = struct{}{} // trace-agent source 1 (OTel semconv)
	}
	return m
}()

type infraTagsProcessor struct {
	tagger   types.TaggerClient
	hostname option.Option[string]
}

// newInfraTagsProcessor creates a new infraTagsProcessor instance
func newInfraTagsProcessor(
	tagger types.TaggerClient,
	hostGetterOpt option.Option[SourceProviderFunc],
) infraTagsProcessor {
	infraTagsProcessor := infraTagsProcessor{
		tagger: tagger,
	}
	if hostnameGetter, found := hostGetterOpt.Get(); found {
		if hostname, err := hostnameGetter(context.Background()); err == nil {
			infraTagsProcessor.hostname = option.New(hostname)
		}
	}
	return infraTagsProcessor
}

// ProcessTags collects entities/tags from resourceAttributes and adds infra tags to resourceAttributes.
//
// The promote parameter controls how tags that are NOT recognized container-tag
// conventions are surfaced for downstream `_dd.tags.container` promotion. Known
// DD / OTel conventions (knownConventionKeys), USM keys, and the
// `datadog.host.name` host attribute are exempt and always written under their
// canonical key.
//
// If ddtags is non-nil, custom tags (i.e. those NOT covered by the exemptions
// above) are appended to *ddtags as "key:value" pairs instead of being written
// to resourceAttributes. This is used by the logs pipeline to turn tagger tags
// into real Datadog log tags rather than log attributes. Exempted keys are
// always written to resourceAttributes regardless of ddtags, since the
// Datadog logs intake already promotes them into tags on its own.
func (p infraTagsProcessor) ProcessTags(
	logger *zap.Logger,
	cardinality types.TagCardinality,
	resourceAttributes pcommon.Map,
	allowHostnameOverride bool,
	promote ContainerTagPromotionMode,
	ddtags *[]string,
) {
	if _, ok := resourceAttributes.Get(string(conventions.ContainerIDKey)); !ok {
		originInfo := originInfoFromAttributes(resourceAttributes, cardinality)
		if containerID, err := p.tagger.GenerateContainerIDFromOriginInfo(originInfo); err == nil {
			resourceAttributes.PutStr(string(conventions.ContainerIDKey), containerID)
		}
	}

	entityIDs := entityIDsFromAttributes(resourceAttributes)
	tagMap := make(map[string]string)

	// Get all unique tags from resource attributes and global tags
	for _, entityID := range entityIDs {
		entityTags, err := p.tagger.Tag(entityID, cardinality)
		if err != nil {
			logger.Error("Cannot get tags for entity", zap.String("entityID", entityID.String()), zap.Error(err))
			continue
		}
		for _, tag := range entityTags {
			k, v := splitTag(tag)
			_, hasTag := tagMap[k]
			if k != "" && v != "" && !hasTag {
				tagMap[k] = v
			}
		}
	}
	globalTags, err := p.tagger.GlobalTags(cardinality)
	if err != nil {
		logger.Error("Cannot get global tags", zap.Error(err))
	}
	for _, tag := range globalTags {
		k, v := splitTag(tag)
		_, hasTag := tagMap[k]
		if k != "" && v != "" && !hasTag {
			tagMap[k] = v
		}
	}

	// Add all tags as resource attributes
	for k, v := range tagMap {
		otelAttrs, ust := unifiedServiceTagMap[k]
		if !ust {
			writeTagAttribute(resourceAttributes, k, v, promote, ddtags)
			continue
		}

		// Add OTel semantics for unified service tags which are required in mapping
		hasOTelAttr := false
		for _, otelAttr := range otelAttrs {
			if _, ok := resourceAttributes.Get(otelAttr); ok {
				hasOTelAttr = true
				break
			}
		}
		if !hasOTelAttr {
			resourceAttributes.PutStr(otelAttrs[0], v)
		}
	}

	if allowHostnameOverride {
		if hostname, found := p.hostname.Get(); found {
			resourceAttributes.PutStr("datadog.host.name", hostname)
		}
	}
}

// writeTagAttribute writes a non-USM tag into resource attributes, honoring
// the container-tag-promotion mode. Keys already carrying the
// `datadog.container.tag.` prefix and keys in knownConventionKeys are always
// written as-is (idempotency / convention exemption); only truly custom keys
// are subject to duplication / renaming.
//
// If ddtags is non-nil, a truly custom key is instead appended to *ddtags as
// "k:v" and NOT written to resourceAttributes (the promote mode is ignored in
// this case, since it only makes sense for the resource-attribute path).
func writeTagAttribute(resourceAttributes pcommon.Map, k, v string, promote ContainerTagPromotionMode, ddtags *[]string) {
	if strings.HasPrefix(k, attributes.CustomContainerTagPrefix) {
		resourceAttributes.PutStr(k, v)
		return
	}
	if _, isKnown := knownConventionKeys[k]; isKnown {
		resourceAttributes.PutStr(k, v)
		return
	}
	if ddtags != nil {
		*ddtags = append(*ddtags, k+":"+v)
		return
	}
	switch promote {
	case ContainerTagPromotionDuplicate:
		resourceAttributes.PutStr(k, v)
		putStrIfAbsent(resourceAttributes, attributes.CustomContainerTagPrefix+k, v)
	case ContainerTagPromotionRename:
		putStrIfAbsent(resourceAttributes, attributes.CustomContainerTagPrefix+k, v)
	default: // "", ContainerTagPromotionOff
		resourceAttributes.PutStr(k, v)
	}
}

// putStrIfAbsent writes k=v only when k is not already present in attrs.
// Used by the promotion logic to avoid overwriting a `datadog.container.tag.<X>`
// value the user set themselves.
func putStrIfAbsent(attrs pcommon.Map, k, v string) {
	if _, exists := attrs.Get(k); !exists {
		attrs.PutStr(k, v)
	}
}

func originInfoFromAttributes(attrs pcommon.Map, cardinality types.TagCardinality) origindetection.OriginInfo {
	originInfo := origindetection.OriginInfo{
		ExternalData: origindetection.ExternalData{
			Init: false, // Assume non-init container by default
		},
		Cardinality:   types.TagCardinalityToString(cardinality),
		ProductOrigin: origindetection.ProductOriginOTel,
	}

	if processPid, ok := attrs.Get(string(conventions.ProcessPIDKey)); ok && processPid.Type() == pcommon.ValueTypeInt {
		originInfo.LocalData.ProcessID = uint32(processPid.Int())
	}
	if podUID, ok := attrs.Get(string(conventions.K8SPodUIDKey)); ok {
		originInfo.LocalData.PodUID = podUID.AsString()
		originInfo.ExternalData.PodUID = podUID.AsString()
	}
	if k8sContainerName, ok := attrs.Get(string(conventions.K8SContainerNameKey)); ok {
		originInfo.ExternalData.ContainerName = k8sContainerName.AsString()
	}

	// Ad-hoc attributes for data not covered by K8s semantic conventions
	if cgroupInode, ok := attrs.Get("datadog.container.cgroup_inode"); ok && cgroupInode.Type() == pcommon.ValueTypeInt {
		originInfo.LocalData.Inode = uint64(cgroupInode.Int())
	}
	if initContainer, ok := attrs.Get("datadog.container.is_init"); ok && initContainer.Type() == pcommon.ValueTypeBool {
		originInfo.ExternalData.Init = initContainer.Bool()
	}

	return originInfo
}

// TODO: Replace OriginIDFromAttributes in opentelemetry-mapping-go with this method
// entityIDsFromAttributes gets the entity IDs from resource attributes.
// If not found, an empty string slice is returned.
func entityIDsFromAttributes(attrs pcommon.Map) []types.EntityID {
	entityIDs := make([]types.EntityID, 0, 8)
	// Prefixes come from pkg/util/kubernetes/kubelet and pkg/util/containers.
	if containerID, ok := attrs.Get(string(conventions.ContainerIDKey)); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.ContainerID, containerID.AsString()))
	}
	if ociManifestDigest, ok := attrs.Get(string(conventions22.OciManifestDigestKey)); ok {
		splitImageID := strings.SplitN(ociManifestDigest.AsString(), "@sha256:", 2)
		if len(splitImageID) == 2 {
			entityIDs = append(entityIDs, types.NewEntityID(types.ContainerImageMetadata, fmt.Sprintf("sha256:%v", splitImageID[1])))
		}
	}
	if ecsTaskArn, ok := attrs.Get(string(conventions.AWSECSTaskARNKey)); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.ECSTask, ecsTaskArn.AsString()))
	}
	if deploymentName, ok := attrs.Get(string(conventions.K8SDeploymentNameKey)); ok {
		namespace, namespaceOk := attrs.Get(string(conventions.K8SNamespaceNameKey))
		if namespaceOk {
			entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesDeployment, fmt.Sprintf("%s/%s", namespace.AsString(), deploymentName.AsString())))
		}
	}
	if namespace, ok := attrs.Get(string(conventions.K8SNamespaceNameKey)); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesMetadata, "/namespaces//"+namespace.AsString()))
	}

	if nodeName, ok := attrs.Get(string(conventions.K8SNodeNameKey)); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesMetadata, "/nodes//"+nodeName.AsString()))
	}
	if podUID, ok := attrs.Get(string(conventions.K8SPodUIDKey)); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesPodUID, podUID.AsString()))
	}
	if processPid, ok := attrs.Get(string(conventions.ProcessPIDKey)); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.Process, processPid.AsString()))
	}
	return entityIDs
}

func splitTag(tag string) (key string, value string) {
	split := strings.SplitN(tag, ":", 2)
	if len(split) < 2 || split[0] == "" || split[1] == "" {
		return "", ""
	}
	return split[0], split[1]
}
