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

// ClusterRoleBindingHandlers implements the Handlers interface for Kubernetes ClusteRoleBindings.
type ClusterRoleBindingHandlers = common.ModelHandlers[*rbacv1.ClusterRoleBinding, *model.ClusterRoleBinding]

// NewClusterRoleBindingHandlers creates a new ClusterRoleBindingHandlers.
func NewClusterRoleBindingHandlers(tagger tagger.Component) *ClusterRoleBindingHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*rbacv1.ClusterRoleBinding, *model.ClusterRoleBinding]{
		Extract: k8sTransformers.ExtractClusterRoleBinding,
		Tags:    func(m *model.ClusterRoleBinding) *[]string { return &m.Tags },
		YAML:    func(m *model.ClusterRoleBinding) *[]byte { return &m.Yaml },
		Entity:  common.ClusterMetadataEntity[*rbacv1.ClusterRoleBinding],
		Message: func(h common.MessageHeader, models []*model.ClusterRoleBinding) model.MessageBody {
			return &model.CollectorClusterRoleBinding{
				ClusterName:         h.ClusterName,
				ClusterId:           h.ClusterID,
				GroupId:             h.GroupID,
				GroupSize:           h.GroupSize,
				ClusterRoleBindings: models,
				Tags:                h.Tags,
				AgentVersion:        h.AgentVersion,
			}
		},
	})
}
