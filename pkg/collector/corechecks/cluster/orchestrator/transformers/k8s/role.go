// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	rbacv1 "k8s.io/api/rbac/v1"
)

// ExtractRole returns the protobuf model corresponding to a Kubernetes Role
// resource.
func ExtractRole(ctx processors.ProcessorContext, r *rbacv1.Role) *model.Role {
	msg := &model.Role{
		Metadata: extractMetadata(&r.ObjectMeta),
		Rules:    extractPolicyRules(r.Rules),
	}

	pctx := ctx.(*processors.K8sProcessorContext)
	msg.Tags = append(msg.Tags, transformers.RetrieveUnifiedServiceTags(r.ObjectMeta.Labels)...)
	msg.Tags = append(msg.Tags, transformers.RetrieveMetadataTags(r.ObjectMeta.Labels, r.ObjectMeta.Annotations, pctx.LabelsAsTags, pctx.AnnotationsAsTags)...)
	return msg
}
