// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
)

// NamespacesToWatch returns the namespaces to watch. If the
// "containerd_namespace" option has been set, it returns the namespaces it contains.
// Otherwise, it returns all of them.
func NamespacesToWatch(ctx context.Context, containerdClient ContainerdItf) ([]string, error) {
	namespaces := config.Datadog.GetStringSlice("containerd_namespace")

	if len(namespaces) == 0 {
		return containerdClient.Namespaces(ctx)
	}

	return namespaces, nil
}

// FiltersWithNamespaces returns the given list of filters adapted to take into
// account the namespaces that we need to watch.
// For example, if the given filter is `topic=="/container/create"`, and the
// namespace that we need to watch is "ns1", this function returns
// `topic=="/container/create",namespace=="ns1"`.
func FiltersWithNamespaces(filters []string) []string {
	namespaces := config.Datadog.GetStringSlice("containerd_namespace")

	if len(namespaces) == 0 {
		// Watch all namespaces. No need to add them to the filters.
		return filters
	}

	var res []string

	for _, filter := range filters {
		for _, namespace := range namespaces {
			res = append(res, fmt.Sprintf(`%s,namespace==%q`, filter, namespace))
		}
	}

	return res
}
