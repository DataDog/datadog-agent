// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// GetMetadata returns metadata about the kubelet runtime such as the kubelet_version.
func GetMetadata() (map[string]string, error) {
	if !config.IsFeaturePresent(config.Kubernetes) {
		return nil, errors.New("kubelet feature deactivated")
	}

	ku, err := GetKubeUtil()
	if err != nil {
		return nil, err
	}

	data, err := ku.GetRawMetrics(context.TODO())
	if err != nil {
		return nil, err
	}

	metric, err := ParseMetricFromRaw(data, "kubernetes_build_info")
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile("(?:gitVersion|git_version)=\"(.*?)\"")
	matches := re.FindStringSubmatch(metric)
	if len(matches) < 1 {
		return nil, fmt.Errorf("couldn't find kubelet git version")
	}
	return map[string]string{
		"kubelet_version": matches[1],
	}, nil
}
