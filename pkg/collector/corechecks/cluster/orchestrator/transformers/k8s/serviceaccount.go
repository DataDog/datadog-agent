// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
)

// ExtractServiceAccount returns the protobuf model corresponding to a
// Kubernetes ServiceAccount resource.
func ExtractServiceAccount(ctx processors.ProcessorContext, sa *corev1.ServiceAccount) *model.ServiceAccount {
	serviceAccount := &model.ServiceAccount{
		Metadata: extractMetadata(&sa.ObjectMeta),
	}
	if sa.AutomountServiceAccountToken != nil {
		serviceAccount.AutomountServiceAccountToken = *sa.AutomountServiceAccountToken
	} else {
		// Default to true if not set see https://github.com/kubernetes/kubernetes/blob/71fa43e37f198ae8035a96ff9f1c112b03b9e0fa/plugin/pkg/admission/serviceaccount/admission.go#L264.
		serviceAccount.AutomountServiceAccountToken = true
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

	pctx := ctx.(*processors.K8sProcessorContext)
	serviceAccount.Tags = append(serviceAccount.Tags, transformers.RetrieveUnifiedServiceTags(sa.ObjectMeta.Labels)...)
	serviceAccount.Tags = append(serviceAccount.Tags, transformers.RetrieveMetadataTags(sa.ObjectMeta.Labels, sa.ObjectMeta.Annotations, pctx.LabelsAsTags, pctx.AnnotationsAsTags)...)

	return serviceAccount
}
