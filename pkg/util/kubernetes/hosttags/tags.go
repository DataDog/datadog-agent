// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet,kubeapiserver

package hosttags

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// GetTags gets the tags from the kubernetes apiserver
func GetTags() ([]string, error) {
	labelsToTags := config.Datadog.GetStringMapString("kubernetes_node_labels_as_tags")
	if len(labelsToTags) == 0 {
		// Nothing to extract
		return nil, nil
	}
	nodeName, err := kubelet.HostnameProvider("")
	if err != nil {
		return nil, err
	}
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, err
	}
	nodeLabels, err := client.NodeLabels(nodeName)
	if err != nil {
		return nil, err
	}
	return extractTags(nodeLabels, labelsToTags), nil
}

func extractTags(nodeLabels, labelsToTags map[string]string) []string {
	var tags []string

	for labelName, labelValue := range nodeLabels {
		// viper lower-cases map keys, so we must lowercase before matching
		if tagName, found := labelsToTags[strings.ToLower(labelName)]; found {
			tags = append(tags, fmt.Sprintf("%s:%s", tagName, labelValue))
		}
	}

	return tags
}
