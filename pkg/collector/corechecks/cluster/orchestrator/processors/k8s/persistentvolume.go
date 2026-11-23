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

// PersistentVolumeHandlers implements the Handlers interface for Kubernetes PersistentVolumes.
type PersistentVolumeHandlers = common.ModelHandlers[*corev1.PersistentVolume, *model.PersistentVolume]

// NewPersistentVolumeHandlers creates a new PersistentVolumeHandlers.
func NewPersistentVolumeHandlers(tagger tagger.Component) *PersistentVolumeHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*corev1.PersistentVolume, *model.PersistentVolume]{
		Extract: k8sTransformers.ExtractPersistentVolume,
		Tags:    func(m *model.PersistentVolume) *[]string { return &m.Tags },
		YAML:    func(m *model.PersistentVolume) *[]byte { return &m.Yaml },
		Entity:  common.ClusterMetadataEntity[*corev1.PersistentVolume],
		Message: func(h common.MessageHeader, models []*model.PersistentVolume) model.MessageBody {
			return &model.CollectorPersistentVolume{
				ClusterName:       h.ClusterName,
				ClusterId:         h.ClusterID,
				GroupId:           h.GroupID,
				GroupSize:         h.GroupSize,
				PersistentVolumes: models,
				Tags:              h.Tags,
				AgentVersion:      h.AgentVersion,
			}
		},
	})
}
