// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import "fmt"

const (
	checkIDAnnotationFormat = "ad.datadoghq.com/%s.check.id"
)

// GetCustomCheckID returns whether there is a custom check ID for a given container based on the pod annotations
func GetCustomCheckID(annotations map[string]string, containerName string) (string, bool) {
	id, found := annotations[fmt.Sprintf(checkIDAnnotationFormat, containerName)]
	return id, found
}
