// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package util

import "strings"

// SplitCollectorName splits a collector name of the form <group_version>/<resource> into its group version and its
// resource. The group version is empty when the name has none, for instance "nodes".
func SplitCollectorName(name string) (groupVersion, resource string) {
	idx := strings.LastIndex(name, "/")
	if idx == -1 {
		return "", name
	}
	return name[:idx], name[idx+1:]
}
