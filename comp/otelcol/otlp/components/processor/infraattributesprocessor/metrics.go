// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/processor"
	conventions "go.opentelemetry.io/collector/semconv/v1.21.0"
	"go.uber.org/zap"
)

type infraAttributesMetricProcessor struct {
	logger      *zap.Logger
	tagger      tagger.Component
	cardinality types.TagCardinality
}

func newInfraAttributesMetricProcessor(set processor.CreateSettings, cfg *Config, tagger tagger.Component) (*infraAttributesMetricProcessor, error) {
	iamp := &infraAttributesMetricProcessor{
		logger:      set.Logger,
		tagger:      tagger,
		cardinality: cfg.Cardinality,
	}
	set.Logger.Info("Metric Infra Attributes Processor configured")
	return iamp, nil
}

// TODO: Replace OriginIDFromAttributes in opentelemetry-mapping-go with this method
// entityIDsFromAttributes gets the entity IDs from resource attributes.
// If not found, an empty string slice is returned.
func entityIDsFromAttributes(attrs pcommon.Map) []string {
	entityIDs := make([]string, 0, 8)
	// Prefixes come from pkg/util/kubernetes/kubelet and pkg/util/containers.
	if containerID, ok := attrs.Get(conventions.AttributeContainerID); ok {
		entityIDs = append(entityIDs, fmt.Sprintf("container_id://%v", containerID.AsString()))
	}
	if containerImageID, ok := attrs.Get(conventions.AttributeContainerImageID); ok {
		splitImageID := strings.SplitN(containerImageID.AsString(), "@sha256:", 2)
		if len(splitImageID) == 2 {
			entityIDs = append(entityIDs, fmt.Sprintf("container_image_metadata://sha256:%v", splitImageID[1]))
		}
	}
	if ecsTaskArn, ok := attrs.Get(conventions.AttributeAWSECSTaskARN); ok {
		entityIDs = append(entityIDs, fmt.Sprintf("ecs_task://%v", ecsTaskArn.AsString()))
	}
	if deploymentName, ok := attrs.Get(conventions.AttributeK8SDeploymentName); ok {
		namespace, namespaceOk := attrs.Get(conventions.AttributeK8SNamespaceName)
		if namespaceOk {
			entityIDs = append(entityIDs, fmt.Sprintf("deployment://%v/%v", namespace.AsString(), deploymentName.AsString()))
		}
	}
	if namespace, ok := attrs.Get(conventions.AttributeK8SNamespaceName); ok {
		entityIDs = append(entityIDs, fmt.Sprintf("namespace://%v", namespace.AsString()))
	}
	if nodeUID, ok := attrs.Get(conventions.AttributeK8SNodeUID); ok {
		entityIDs = append(entityIDs, fmt.Sprintf("kubernetes_node_uid://%v", nodeUID.AsString()))
	}
	if podUID, ok := attrs.Get(conventions.AttributeK8SPodUID); ok {
		entityIDs = append(entityIDs, fmt.Sprintf("kubernetes_pod_uid://%v", podUID.AsString()))
	}
	if processPid, ok := attrs.Get(conventions.AttributeProcessPID); ok {
		entityIDs = append(entityIDs, fmt.Sprintf("process://%v", processPid.AsString()))
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

func (iamp *infraAttributesMetricProcessor) processMetrics(_ context.Context, md pmetric.Metrics) (pmetric.Metrics, error) {
	rms := md.ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		resourceAttributes := rms.At(i).Resource().Attributes()
		entityIDs := entityIDsFromAttributes(resourceAttributes)
		tagMap := make(map[string]string)

		// Get all unique tags from resource attributes and global tags
		for _, entityID := range entityIDs {
			entityTags, err := iamp.tagger.Tag(entityID, iamp.cardinality)
			if err != nil {
				iamp.logger.Error("Cannot get tags for entity", zap.String("entityID", entityID), zap.Error(err))
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
		globalTags, err := iamp.tagger.GlobalTags(iamp.cardinality)
		if err != nil {
			iamp.logger.Error("Cannot get global tags", zap.Error(err))
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
			resourceAttributes.PutStr(k, v)
		}
	}
	return md, nil
}
