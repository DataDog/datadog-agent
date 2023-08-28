// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"fmt"
	"strings"
)

var kubeKindToWorkloadmetaKindMap = map[string]Kind{
	"Pod":        KindKubernetesPod,
	"Deployment": KindKubernetesDeployment,
	"Node":       KindKubernetesNode,
}

// KubernetesKindToWorkloadMetaKind maps a Kubernetes Kind to a workloadmeta Kind.
func KubernetesKindToWorkloadMetaKind(kind string) (Kind, bool) {
	v, ok := kubeKindToWorkloadmetaKindMap[kind]
	return v, ok
}

func mapToString(m map[string]string) string {
	var sb strings.Builder
	for k, v := range m {
		fmt.Fprintf(&sb, "%s:%s ", k, v)
	}

	return sb.String()
}

func sliceToString(s []string) string {
	return strings.Join(s, " ")
}

// filterAndFormatEnvVars extracts and formats a subset of allowed environment variables.
func filterAndFormatEnvVars(envs map[string]string) string {
	allowedEnvVariables := []string{"DD_SERVICE", "DD_ENV", "DD_VERSION"}
	var sb strings.Builder
	for _, allowed := range allowedEnvVariables {
		if val, found := envs[allowed]; found {
			fmt.Fprintf(&sb, "%s:%s ", allowed, val)
		}
	}

	return sb.String()
}
