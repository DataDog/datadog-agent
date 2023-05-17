// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host/container"
)

func init() {
	container.RegisterMetadataProvider("kubelet", getMetadata)
}

func getMetadata() (map[string]string, error) {
	metadata := make(map[string]string)
	if !config.IsFeaturePresent(config.Kubernetes) {
		return metadata, nil
	}

	ku, err := GetKubeUtil()
	if err != nil {
		return metadata, err
	}
	data, err := ku.GetRawMetrics(context.TODO())
	if err != nil {
		return metadata, err
	}
	metric, err := ParseMetricFromRaw(data, "kubernetes_build_info")
	if err != nil {
		return metadata, err
	}
	re := regexp.MustCompile("(?:gitVersion|git_version)=\"(.*?)\"")
	matches := re.FindStringSubmatch(metric)
	if len(matches) < 1 {
		return metadata, fmt.Errorf("couldn't find kubelet git version")
	}
	metadata["kubelet_version"] = matches[1]

	return metadata, nil
}
