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

	netv1 "k8s.io/api/networking/v1"
)

// NetworkPolicyHandlers implements the Handlers interface for Kubernetes NetworkPolicy.
type NetworkPolicyHandlers = common.ModelHandlers[*netv1.NetworkPolicy, *model.NetworkPolicy]

// NewNetworkPolicyHandlers creates a new NetworkPolicyHandlers.
func NewNetworkPolicyHandlers(tagger tagger.Component) *NetworkPolicyHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*netv1.NetworkPolicy, *model.NetworkPolicy]{
		Extract: k8sTransformers.ExtractNetworkPolicy,
		Tags:    func(m *model.NetworkPolicy) *[]string { return &m.Tags },
		YAML:    func(m *model.NetworkPolicy) *[]byte { return &m.Yaml },
		Entity:  common.MetadataEntity[*netv1.NetworkPolicy],
		Message: func(h common.MessageHeader, models []*model.NetworkPolicy) model.MessageBody {
			return &model.CollectorNetworkPolicy{
				ClusterName:     h.ClusterName,
				ClusterId:       h.ClusterID,
				GroupId:         h.GroupID,
				GroupSize:       h.GroupSize,
				NetworkPolicies: models,
				Tags:            h.Tags,
				AgentVersion:    h.AgentVersion,
			}
		},
	})
}
