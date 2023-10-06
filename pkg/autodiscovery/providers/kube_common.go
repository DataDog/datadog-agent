// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver

package providers

import "strings"

const ignoreADTagsAnnotationSuffix = "ignore_autodiscovery_tags"

// ignoreADTagsFromAnnotations returns whether the check should have autodiscovery tags from the service (e.g kube_namespace)
// based on the value of the annotation ad.datadoghq.com/ignore_autodiscovery_tags
func ignoreADTagsFromAnnotations(annotations map[string]string, prefix string) bool {
	if annotations == nil {
		return false
	}
	return strings.ToLower(annotations[prefix+ignoreADTagsAnnotationSuffix]) == "true"
}
