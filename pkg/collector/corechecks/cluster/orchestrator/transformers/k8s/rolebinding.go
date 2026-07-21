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

// ExtractRoleBinding returns the protobuf model corresponding to a Kubernetes
// RoleBinding resource.
//
//nolint:revive
func ExtractRoleBinding(ctx processors.ProcessorContext, rb *rbacv1.RoleBinding) *model.RoleBinding {
	msg := &model.RoleBinding{
		Metadata: extractMetadata(&rb.ObjectMeta),
		RoleRef:  extractRoleRef(&rb.RoleRef),
		Subjects: extractSubjects(rb.Subjects),
	}

	msg.Tags = append(msg.Tags, transformers.RetrieveUnifiedServiceTags(rb.ObjectMeta.Labels)...)
	msg.Tags = append(msg.Tags, transformers.RetrieveTeamTag(rb.ObjectMeta.Labels, rb.ObjectMeta.Annotations)...)

	return msg
}
