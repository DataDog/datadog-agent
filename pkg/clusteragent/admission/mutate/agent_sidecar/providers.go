// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	corev1 "k8s.io/api/core/v1"
)

////////////////////////////////
//                            //
//     Provider Overrides     //
//                            //
////////////////////////////////

// ProviderIsSupported indicates whether the provider is supported by agent sidecar injection
func ProviderIsSupported(provider string) bool {
	switch provider {
	case providerFargate:
		return true
	case "":
		// case of empty provider
		return true
	default:
		return false
	}
}

func applyProviderOverrides(container *corev1.Container) error {
	provider := config.Datadog.GetString("admission_controller.agent_sidecar.provider")

	if !ProviderIsSupported(provider) {
		return fmt.Errorf("unsupported provider: %v", provider)
	}

	switch provider {
	case providerFargate:
		return applyFargateOverrides(container)
	}

	return nil
}

func applyFargateOverrides(container *corev1.Container) error {
	if container == nil {
		return fmt.Errorf("can't apply profile overrides to nil containers")
	}

	err := withEnvOverrides(container, corev1.EnvVar{
		Name:  "DD_EKS_FARGATE",
		Value: "true",
	})

	if err != nil {
		return err
	}

	return nil
}
