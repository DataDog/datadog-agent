// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package kubernetes

// KubeConfigCredential represents Kubernetes configuration credentials.
type KubeConfigCredential struct {
	Context string
}

// TODO - real implementation for parsing credentials
func parseAsKubeConfigCredentials(_ interface{}) (*KubeConfigCredential, bool) {
	return &KubeConfigCredential{
		Context: "docker-desktop", // Default context, can be overridden
	}, true
}

func parseAsServiceAccountCredentials(_ interface{}) bool {
	return false
}
