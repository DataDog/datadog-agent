// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"context"
	"fmt"

	model "github.com/DataDog/agent-payload/v5/contlcycle"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
	ecsutil "github.com/DataDog/datadog-agent/pkg/util/ecs"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// LifecycleEvent is the internal representation of one lifecycle event.
// ProtoEvent must be fully populated before the event reaches the queue
type LifecycleEvent struct {
	ObjectKind string
	ProtoEvent *model.Event
}

func newPayload(le LifecycleEvent) (*model.EventsPayload, error) {
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		log.Warnf("Error getting hostname: %v", err)
	}

	var clusterID string
	if env.IsFeaturePresent(env.Kubernetes) {
		clusterID, err = clustername.GetClusterID()
	} else if env.IsFeaturePresent(env.ECSEC2) || env.IsFeaturePresent(env.ECSFargate) || env.IsFeaturePresent(env.ECSManagedInstances) {
		var meta *ecsutil.MetaECS
		meta, err = ecsutil.GetClusterMeta()
		if meta != nil {
			clusterID = meta.ECSClusterID
		}
	}
	if err != nil {
		log.Warnf("Error getting cluster id: %v", err)
	}

	kind, err := kindToModel(le.ObjectKind)
	if err != nil {
		return nil, err
	}

	return &model.EventsPayload{
		Version:    types.PayloadV1,
		Host:       hname,
		ClusterId:  clusterID,
		ObjectKind: kind,
		Events:     []*model.Event{le.ProtoEvent},
	}, nil
}

func kindToModel(kind string) (model.ObjectKind, error) {
	switch kind {
	case types.ObjectKindContainer:
		return model.ObjectKind_Container, nil
	case types.ObjectKindPod:
		return model.ObjectKind_Pod, nil
	case types.ObjectKindTask:
		return model.ObjectKind_Task, nil
	default:
		return -1, fmt.Errorf("unknown object kind %q", kind)
	}
}
