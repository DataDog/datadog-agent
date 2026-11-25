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

// RoleHandlers implements the Handlers interface for Kubernetes Roles.
type RoleHandlers = common.ModelHandlers[*rbacv1.Role, *model.Role]

// NewRoleHandlers creates a new RoleHandlers.
func NewRoleHandlers(tagger tagger.Component) *RoleHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*rbacv1.Role, *model.Role]{
		Extract: k8sTransformers.ExtractRole,
		Tags:    func(m *model.Role) *[]string { return &m.Tags },
		YAML:    func(m *model.Role) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*rbacv1.Role],
		Message: func(h common.MessageHeader, models []*model.Role) model.MessageBody {
			return &model.CollectorRole{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Roles:        models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
