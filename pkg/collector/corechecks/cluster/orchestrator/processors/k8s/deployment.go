// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors/common"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/redact"

	appsv1 "k8s.io/api/apps/v1"
)

// DeploymentHandlers implements the Handlers interface for Kubernetes Deployments.
type DeploymentHandlers = common.ModelHandlers[*appsv1.Deployment, *model.Deployment]

// NewDeploymentHandlers creates a new DeploymentHandlers.
func NewDeploymentHandlers(tagger tagger.Component) *DeploymentHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*appsv1.Deployment, *model.Deployment]{
		Extract: k8sTransformers.ExtractDeployment,
		Tags:    func(m *model.Deployment) *[]string { return &m.Tags },
		YAML:    func(m *model.Deployment) *[]byte { return &m.Yaml },
		Entity:  common.NamespacedEntity[*appsv1.Deployment](taggertypes.KubernetesDeployment),
		Scrub:   func(r *appsv1.Deployment, s *redact.DataScrubber) { redact.ScrubPodTemplateSpec(&r.Spec.Template, s) },
		Message: func(h common.MessageHeader, models []*model.Deployment) model.MessageBody {
			return &model.CollectorDeployment{
				ClusterName:  h.ClusterName,
				ClusterId:    h.ClusterID,
				GroupId:      h.GroupID,
				GroupSize:    h.GroupSize,
				Deployments:  models,
				Tags:         h.Tags,
				AgentVersion: h.AgentVersion,
			}
		},
	})
}
