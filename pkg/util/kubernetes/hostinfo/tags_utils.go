// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"strings"
)

const (
	// NormalizedRoleLabel is original Kubernetes label for role, we normalize the new one to this one
	NormalizedRoleLabel string = "kubernetes.io/role"
)

// LabelPreprocessor ensure different role labels are parsed correctly
func LabelPreprocessor(labelName string, labelValue string) (string, string) {
	switch {
	// Kube label syntax guarantees that a valid name is present after /
	// Label value is not used by Kube in this case
	case strings.HasPrefix(labelName, "node-role.kubernetes.io/"):
		return NormalizedRoleLabel, labelName[24:]
	}

	return labelName, labelValue
}
