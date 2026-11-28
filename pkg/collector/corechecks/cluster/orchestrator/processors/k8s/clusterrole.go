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

// ClusterRoleHandlers implements the Handlers interface for Kubernetes ClusterRoles.
type ClusterRoleHandlers = common.ModelHandlers[*rbacv1.ClusterRole, *model.ClusterRole]

// NewClusterRoleHandlers creates a new ClusterRoleHandlers.
func NewClusterRoleHandlers(tagger tagger.Component) *ClusterRoleHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*rbacv1.ClusterRole, *model.ClusterRole]{
		Extract: k8sTransformers.ExtractClusterRole,
		Tags:    func(m *model.ClusterRole) *[]string { return &m.Tags },
		YAML:    func(m *model.ClusterRole) *[]byte { return &m.Yaml },
		Entity:  common.ClusterMetadataEntity[*rbacv1.ClusterRole],
		Message: func(h common.MessageHeader, models []*model.ClusterRole) model.MessageBody {
			return &model.CollectorClusterRole{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				ClusterRoles: models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
