// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"

	rbacv1 "k8s.io/api/rbac/v1"
)

// RoleBindingHandlers implements the Handlers interface for Kubernetes RoleBindings.
type RoleBindingHandlers = common.ModelHandlers[*rbacv1.RoleBinding, *model.RoleBinding]

// NewRoleBindingHandlers creates a new RoleBindingHandlers.
func NewRoleBindingHandlers(tagger tagger.Component) *RoleBindingHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*rbacv1.RoleBinding, *model.RoleBinding]{
		Extract: k8sTransformers.ExtractRoleBinding,
		Tags:    func(m *model.RoleBinding) *[]string { return &m.Tags },
		YAML:    func(m *model.RoleBinding) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*rbacv1.RoleBinding],
		Message: func(h common.MessageHeader, models []*model.RoleBinding) model.MessageBody {
			return &model.CollectorRoleBinding{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				RoleBindings: models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
