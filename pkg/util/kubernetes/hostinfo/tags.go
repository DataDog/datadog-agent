// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package hostinfo

import (
	"context"

	k8smetadata "github.com/DataDog/datadog-agent/comp/core/tagger/k8s_metadata"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	"github.com/DataDog/datadog-agent/pkg/config"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// KubeNodeTagsProvider allows computing node tags based on the user configurations for node labels and annotations as tags
type KubeNodeTagsProvider struct {
	metadataAsTags configutils.MetadataAsTags
}

// NewKubeNodeTagsProvider creates and returns a new kube node tags provider object
func NewKubeNodeTagsProvider(conf config.Reader) KubeNodeTagsProvider {
	return KubeNodeTagsProvider{configutils.GetMetadataAsTags(conf)}
}

// GetTags gets the tags from the kubernetes apiserver and the kubelet
func (k KubeNodeTagsProvider) GetTags(ctx context.Context) ([]string, error) {
	tags, err := k.getNodeInfoTags(ctx)
	if err != nil {
		return nil, err
	}

	annotationsToTags := k.metadataAsTags.GetNodeAnnotationsAsTags()
	if len(annotationsToTags) == 0 {
		return tags, nil
	}

	nodeAnnotations, err := GetNodeAnnotations(ctx)
	if err != nil {
		return nil, err
	}
	tags = append(tags, extractTags(nodeAnnotations, annotationsToTags)...)

	return tags, nil
}

// getNodeInfoTags gets the tags from the kubelet and the cluster-agent
func (k KubeNodeTagsProvider) getNodeInfoTags(ctx context.Context) ([]string, error) {
	nodeInfo, err := NewNodeInfo()
	if err != nil {
		log.Debugf("Unable to auto discover node info tags: %s", err)
		return nil, err
	}

	nodeName, err := nodeInfo.GetNodeName(ctx)
	if err != nil || nodeName == "" {
		log.Debugf("Unable to auto discover node name: %s", err)
		// We can return an error here because nodeName needs to be retrieved
		// for node labels and node annotations.
		return nil, err
	}
	tags := []string{"kube_node:" + nodeName}
	labelsToTags := k.metadataAsTags.GetNodeLabelsAsTags()
	if len(labelsToTags) == 0 {
		return tags, nil
	}

	nodeLabels, err := nodeInfo.GetNodeLabels(ctx)
	if err != nil {
		log.Errorf("Unable to auto discover node labels: %s", err)
		return nil, err
	}
	if len(nodeLabels) > 0 {
		tags = append(tags, extractTags(nodeLabels, labelsToTags)...)
	}

	return tags, nil
}

func getDefaultLabelsToTags() map[string]string {
	return map[string]string{
		NormalizedRoleLabel: kubernetes.KubeNodeRoleTagName,
	}
}

func extractTags(nodeLabels, labelsToTags map[string]string) []string {
	tagList := taglist.NewTagList()
	labelsToTags, glob := k8smetadata.InitMetadataAsTags(labelsToTags)
	for labelName, labelValue := range nodeLabels {
		labelName, labelValue := LabelPreprocessor(labelName, labelValue)
		k8smetadata.AddMetadataAsTags(labelName, labelValue, labelsToTags, glob, tagList)
	}

	tags, _, _, _ := tagList.Compute()
	return tags
}
