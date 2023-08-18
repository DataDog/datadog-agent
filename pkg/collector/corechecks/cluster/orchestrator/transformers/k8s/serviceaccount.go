// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
)

// ExtractServiceAccount returns the protobuf model corresponding to a
// Kubernetes ServiceAccount resource.
func ExtractServiceAccount(sa *corev1.ServiceAccount) *model.ServiceAccount {
	serviceAccount := &model.ServiceAccount{
		Metadata: extractMetadata(&sa.ObjectMeta),
	}
	if sa.AutomountServiceAccountToken != nil {
		serviceAccount.AutomountServiceAccountToken = *sa.AutomountServiceAccountToken
	}
	// Extract secret references.
	for _, secret := range sa.Secrets {
		serviceAccount.Secrets = append(serviceAccount.Secrets, &model.ObjectReference{
			ApiVersion:      secret.APIVersion,
			FieldPath:       secret.FieldPath,
			Kind:            secret.Kind,
			Name:            secret.Name,
			Namespace:       secret.Namespace,
			ResourceVersion: secret.ResourceVersion,
			Uid:             string(secret.UID),
		})
	}
	// Extract secret references for pulling images.
	for _, imgPullSecret := range sa.ImagePullSecrets {
		serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, &model.TypedLocalObjectReference{
			Name: imgPullSecret.Name,
		})
	}

	serviceAccount.Tags = append(serviceAccount.Tags, transformers.RetrieveUnifiedServiceTags(sa.ObjectMeta.Labels)...)

	return serviceAccount
}
