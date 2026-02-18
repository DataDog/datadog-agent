// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// defaultRestrictedSecurityContext is the security context used for init containers
	// in namespaces with the "restricted" Pod Security Standard.
	// https://datadoghq.atlassian.net/browse/INPLAT-492
	defaultRestrictedSecurityContext = &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		RunAsNonRoot:             ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
)

// resolveInitSecurityContext determines the appropriate security context for init containers
// based on namespace labels and global configuration.
func resolveInitSecurityContext(cfg LibraryInjectionConfig, nsName string) *corev1.SecurityContext {
	// Use the configured security context if provided
	if cfg.InitSecurityContext != nil {
		return cfg.InitSecurityContext
	}

	// If wmeta is not available, we can't check namespace labels
	if cfg.Wmeta == nil {
		return nil
	}

	// Check namespace labels for Pod Security Standard
	id := util.GenerateKubeMetadataEntityID("", "namespaces", "", nsName)
	ns, err := cfg.Wmeta.GetKubernetesMetadata(id)
	if err != nil {
		log.Warnf("error getting labels for namespace=%s: %s", nsName, err)
		return nil
	}

	if val, ok := ns.EntityMeta.Labels["pod-security.kubernetes.io/enforce"]; ok && val == "restricted" {
		return defaultRestrictedSecurityContext
	}

	return nil
}
