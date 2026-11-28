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

	storagev1 "k8s.io/api/storage/v1"
)

// StorageClassHandlers implements the Handlers interface for Kubernetes StorageClass.
type StorageClassHandlers = common.ModelHandlers[*storagev1.StorageClass, *model.StorageClass]

// NewStorageClassHandlers creates a new StorageClassHandlers.
func NewStorageClassHandlers(tagger tagger.Component) *StorageClassHandlers {
	return common.NewModelHandlers(tagger, common.ModelSpec[*storagev1.StorageClass, *model.StorageClass]{
		Extract: k8sTransformers.ExtractStorageClass,
		Tags:    func(m *model.StorageClass) *[]string { return &m.Tags },
		Entity:  common.ClusterMetadataEntity[*storagev1.StorageClass],
		Message: func(h common.MessageHeader, models []*model.StorageClass) model.MessageBody {
			return &model.CollectorStorageClass{
				ClusterName:    h.ClusterName,
				ClusterId:      h.ClusterID,
				GroupId:        h.GroupID,
				GroupSize:      h.GroupSize,
				StorageClasses: models,
				Tags:           h.Tags,
				AgentVersion:   h.AgentVersion,
			}
		},
	})
}
