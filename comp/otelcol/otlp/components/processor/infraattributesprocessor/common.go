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
	conventions "go.opentelemetry.io/collector/semconv/v1.21.0"
	conventions22 "go.opentelemetry.io/collector/semconv/v1.22.0"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var unifiedServiceTagMap = map[string][]string{
	tags.Service: {conventions.AttributeServiceName},
	tags.Env:     {conventions.AttributeDeploymentEnvironment, "deployment.environment.name"},
	tags.Version: {conventions.AttributeServiceVersion},
}

type infraTagsProcessor struct {
	tagger   taggerClient
	hostname option.Option[string]
}

// newInfraTagsProcessor creates a new infraTagsProcessor instance
func newInfraTagsProcessor(
	tagger taggerClient,
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

// ProcessTags collects entities/tags from resourceAttributes and adds infra tags to resourceAttributes
func (p infraTagsProcessor) ProcessTags(
	logger *zap.Logger,
	cardinality types.TagCardinality,
	resourceAttributes pcommon.Map,
	allowHostnameOverride bool,
) {
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
			resourceAttributes.PutStr(k, v)
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

// TODO: Replace OriginIDFromAttributes in opentelemetry-mapping-go with this method
// entityIDsFromAttributes gets the entity IDs from resource attributes.
// If not found, an empty string slice is returned.
func entityIDsFromAttributes(attrs pcommon.Map) []types.EntityID {
	entityIDs := make([]types.EntityID, 0, 8)
	// Prefixes come from pkg/util/kubernetes/kubelet and pkg/util/containers.
	if containerID, ok := attrs.Get(conventions.AttributeContainerID); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.ContainerID, containerID.AsString()))
	}
	if ociManifestDigest, ok := attrs.Get(conventions22.AttributeOciManifestDigest); ok {
		splitImageID := strings.SplitN(ociManifestDigest.AsString(), "@sha256:", 2)
		if len(splitImageID) == 2 {
			entityIDs = append(entityIDs, types.NewEntityID(types.ContainerImageMetadata, fmt.Sprintf("sha256:%v", splitImageID[1])))
		}
	}
	if ecsTaskArn, ok := attrs.Get(conventions.AttributeAWSECSTaskARN); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.ECSTask, ecsTaskArn.AsString()))
	}
	if deploymentName, ok := attrs.Get(conventions.AttributeK8SDeploymentName); ok {
		namespace, namespaceOk := attrs.Get(conventions.AttributeK8SNamespaceName)
		if namespaceOk {
			entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesDeployment, fmt.Sprintf("%s/%s", namespace.AsString(), deploymentName.AsString())))
		}
	}
	if namespace, ok := attrs.Get(conventions.AttributeK8SNamespaceName); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesMetadata, fmt.Sprintf("/namespaces//%s", namespace.AsString())))
	}

	if nodeName, ok := attrs.Get(conventions.AttributeK8SNodeName); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesMetadata, fmt.Sprintf("/nodes//%s", nodeName.AsString())))
	}
	if podUID, ok := attrs.Get(conventions.AttributeK8SPodUID); ok {
		entityIDs = append(entityIDs, types.NewEntityID(types.KubernetesPodUID, podUID.AsString()))
	}
	if processPid, ok := attrs.Get(conventions.AttributeProcessPID); ok {
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
