// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	rbacv1 "k8s.io/api/rbac/v1"
)

// ExtractRole returns the protobuf model corresponding to a Kubernetes Role
// resource.
func ExtractRole(r *rbacv1.Role) *model.Role {
	msg := &model.Role{
		Metadata: extractMetadata(&r.ObjectMeta),
		Rules:    extractPolicyRules(r.Rules),
	}

	msg.Tags = append(msg.Tags, transformers.RetrieveUnifiedServiceTags(r.ObjectMeta.Labels)...)

	return msg
}
