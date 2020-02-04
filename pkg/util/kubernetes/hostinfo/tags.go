// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubelet,kubeapiserver

package hostinfo

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
)

// GetTags gets the tags from the kubernetes apiserver
func GetTags() ([]string, error) {
	labelsToTags := getLabelsToTags()
	if len(labelsToTags) == 0 {
		// Nothing to extract
		return nil, nil
	}

	nodeLabels, err := GetNodeLabels()
	if err != nil {
		return nil, err
	}

	return extractTags(nodeLabels, labelsToTags), nil
}

func getDefaultLabelsToTags() map[string]string {
	return map[string]string{
		"kubernetes.io/role": "kube_node_role",
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

func extractTags(nodeLabels, labelsToTags map[string]string) []string {
	tagList := utils.NewTagList()

	for labelName, labelValue := range nodeLabels {
		labelName, labelValue := labelPreprocessor(labelName, labelValue)

		if tagName, found := labelsToTags[strings.ToLower(labelName)]; found {
			tagList.AddLow(tagName, labelValue)
		}
	}

	tags, _, _ := tagList.Compute()
	return tags
}

func labelPreprocessor(labelName string, labelValue string) (string, string) {
	switch {
	// Kube label syntax guarantees that a valid name is present after /
	// Label value is not used by Kube in this case
	case strings.HasPrefix(labelName, "node-role.kubernetes.io/"):
		return "kubernetes.io/role", labelName[24:]
	}

	return labelName, labelValue
}
