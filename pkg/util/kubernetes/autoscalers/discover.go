// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscalers

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
)

const (
	autoscalingGroup = "autoscaling"
	hpaResource      = "horizontalpodautoscalers"
)

var preferredHPAVersions = map[string]int{
	"v2":      3,
	"v2beta2": 2,
	"v2beta1": 1,
}

func DiscoverHPAGroupVersionResource(client kubernetes.Interface) (schema.GroupVersionResource, error) {
	groups, _, err := client.Discovery().ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return schema.GroupVersionResource{}, err
		}

		for group, apiGroupErr := range err.(*discovery.ErrGroupDiscoveryFailed).Groups {
			log.Warnf("unable to perform resource discovery for group %s: %s", group, apiGroupErr)
		}
	}

	for _, group := range groups {
		if group.Name != autoscalingGroup {
			continue
		}

		var (
			chosenVersion       string
			chosenVersionWeight int
		)

		for _, version := range group.Versions {
			weight, ok := preferredHPAVersions[version.Version]
			if !ok {
				continue
			}

			if weight > chosenVersionWeight {
				chosenVersion = version.Version
				chosenVersionWeight = weight
			}
		}

		if chosenVersion == "" {
			return schema.GroupVersionResource{}, fmt.Errorf("cannot find supported HPA version. available versions: %v", group.Versions)
		}

		return schema.GroupVersionResource{
			Group:    autoscalingGroup,
			Version:  chosenVersion,
			Resource: hpaResource,
		}, nil
	}

	return schema.GroupVersionResource{}, fmt.Errorf("cannot find group %q", autoscalingGroup)
}
