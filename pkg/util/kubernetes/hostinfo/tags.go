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
func GetTags(ctx context.Context) (tags []string, err error) {
	tags = appendNodeInfoTags(ctx, tags)

	annotationsToTags := getAnnotationsToTags()
	if len(annotationsToTags) > 0 {
		nodeAnnotations, e := GetNodeAnnotations(ctx)
		if e != nil {
			err = e
		} else {
			tags = append(tags, extractTags(nodeAnnotations, annotationsToTags)...)
		}
	}

	return
}

func appendNodeInfoTags(ctx context.Context, tags []string) []string {
	nodeInfo, err := NewNodeInfo()
	if err != nil {
		log.Debugf("Unable to auto discover node info tags: %s", err)
		return tags
	}

	nodeName, err := nodeInfo.GetNodeName(ctx)
	if err == nil && nodeName != "" {
		tags = append(tags, "kube_node:"+nodeName)
	}

	labelsToTags := getLabelsToTags()
	if len(labelsToTags) > 0 {
		var nodeLabels map[string]string
		nodeLabels, err = nodeInfo.GetNodeLabels(ctx)
		if err == nil && len(nodeLabels) > 0 {
			tags = append(tags, extractTags(nodeLabels, labelsToTags)...)
		}
	}

	return tags
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
