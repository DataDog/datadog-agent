// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package hostinfo

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetTags gets the tags from the kubernetes apiserver and the kubelet
func GetTags(ctx context.Context) ([]string, error) {
	var tags []string
	tags, err := appendNodeInfoTags(ctx, tags)
	if err != nil {
		return nil, err
	}

	annotationsToTags := getAnnotationsToTags()
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

func appendNodeInfoTags(ctx context.Context, tags []string) ([]string, error) {
	nodeInfo, err := NewNodeInfo()
	if err != nil {
		log.Debugf("Unable to auto discover node info tags: %s", err)
		return nil, err
	}

	nodeName, err := nodeInfo.GetNodeName(ctx)
	if err != nil {
		log.Debugf("Unable to auto discover node name: %s", err)
		// We can return an error here because nodeName needs to be retrieved
		// for node labels and node annotations.
		return nil, err
	}

	if nodeName != "" {
		tags = append(tags, "kube_node:"+nodeName)
	}
	labelsToTags := getLabelsToTags()
	if len(labelsToTags) == 0 {
		return tags, nil
	}

	var nodeLabels map[string]string
	nodeLabels, err = nodeInfo.GetNodeLabels(ctx)
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

func getLabelsToTags() map[string]string {
	labelsToTags := getDefaultLabelsToTags()
	for k, v := range config.Datadog.GetStringMapString("kubernetes_node_labels_as_tags") {
		// viper lower-cases map keys from yaml, but not from envvars
		labelsToTags[strings.ToLower(k)] = v
	}

	return labelsToTags
}

func getAnnotationsToTags() map[string]string {
	annotationsToTags := map[string]string{}
	for k, v := range config.Datadog.GetStringMapString("kubernetes_node_annotations_as_tags") {
		// viper lower-cases map keys from yaml, but not from envvars
		annotationsToTags[strings.ToLower(k)] = v
	}

	return annotationsToTags
}

func extractTags(nodeLabels, labelsToTags map[string]string) []string {
	tagList := utils.NewTagList()
	labelsToTags, glob := utils.InitMetadataAsTags(labelsToTags)
	for labelName, labelValue := range nodeLabels {
		labelName, labelValue := LabelPreprocessor(labelName, labelValue)
		utils.AddMetadataAsTags(labelName, labelValue, labelsToTags, glob, tagList)
	}

	tags, _, _, _ := tagList.Compute()
	return tags
}
