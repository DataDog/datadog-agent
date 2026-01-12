// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"context"
	"errors"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	ec2internal "github.com/DataDog/datadog-agent/pkg/util/ec2/internal"
)

// GetClusterName returns the name of the cluster containing the current EC2 instance
func GetClusterName(ctx context.Context) (string, error) {
	if !configutils.IsCloudProviderEnabled(ec2internal.CloudProviderName, pkgconfigsetup.Datadog()) {
		return "", errors.New("cloud provider is disabled by configuration")
	}
	tags, err := fetchTagsFromCache(ctx)
	if err != nil {
		return "", err
	}

	return extractClusterName(tags)
}

func extractClusterName(tags []string) (string, error) {
	var clusterName string
	for _, tag := range tags {
		if strings.HasPrefix(tag, "kubernetes.io/cluster/") { // tag key format: kubernetes.io/cluster/clustername"
			key := strings.Split(tag, ":")[0]
			clusterName = strings.Split(key, "/")[2] // rely on ec2 tag format to extract clustername
			break
		}
	}

	if clusterName == "" {
		return "", errors.New("unable to parse cluster name from EC2 tags")
	}

	return clusterName, nil
}
