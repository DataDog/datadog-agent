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

func init() {
	mergeNamespaceConfigs()
}

// NamespacesToWatch returns the namespaces to watch. If the
// "containerd_namespace" option has been set, it returns the namespaces it contains.
// Otherwise, it returns all of them.
func NamespacesToWatch(ctx context.Context, containerdClient ContainerdItf) ([]string, error) {
	namespaces := config.Datadog.GetStringSlice("containerd_namespaces")

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
	namespaces := config.Datadog.GetStringSlice("containerd_namespaces")

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

// mergeNamespaceConfig merges and dedupes containerd_namespaces and containerd_namespace
func mergeNamespaceConfigs() {
	namespaces := merge(config.Datadog.GetStringSlice("containerd_namespaces"), config.Datadog.GetStringSlice("containerd_namespace"))
	config.Datadog.Set("containerd_namespaces", namespaces)
	config.Datadog.Set("containerd_namespace", namespaces)
}

// merge merges and dedupes 2 slices without changing order
func merge(s1, s2 []string) []string {
	dedupe := map[string]struct{}{}
	merged := []string{}

	for _, elem := range append(s1, s2...) {
		if _, seen := dedupe[elem]; !seen {
			merged = append(merged, elem)
		}

		dedupe[elem] = struct{}{}
	}

	return merged
}
