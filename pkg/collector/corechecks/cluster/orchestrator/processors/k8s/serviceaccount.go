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

	corev1 "k8s.io/api/core/v1"
)

// ServiceAccountHandlers implements the Handlers interface for Kubernetes ServiceAccounts.
type ServiceAccountHandlers = common.ModelHandlers[*corev1.ServiceAccount, *model.ServiceAccount]

// NewServiceAccountHandlers creates a new ServiceAccountHandlers.
func NewServiceAccountHandlers(tagger tagger.Component) *ServiceAccountHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*corev1.ServiceAccount, *model.ServiceAccount]{
		Extract: k8sTransformers.ExtractServiceAccount,
		Tags:    func(m *model.ServiceAccount) *[]string { return &m.Tags },
		YAML:    func(m *model.ServiceAccount) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*corev1.ServiceAccount],
		Message: func(h common.MessageHeader, models []*model.ServiceAccount) model.MessageBody {
			return &model.CollectorServiceAccount{
				ClusterName:     h.ClusterName,
				ClusterId:       h.ClusterID,
				GroupId:         h.GroupID,
				GroupSize:       h.GroupSize,
				ServiceAccounts: models,
				Tags:            h.Tags,
				AgentVersion:    h.AgentVersion,
			}
		},
	})
}
