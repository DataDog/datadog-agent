// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import (
	"fmt"

	model "github.com/DataDog/agent-payload/v5/contlcycle"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	types "github.com/DataDog/datadog-agent/pkg/containerlifecycle"
)

// LifecycleEvent is the internal representation of one lifecycle event.
// ProtoEvent must be fully populated before the event reaches the queue.
type LifecycleEvent struct {
	ObjectKind string
	ProtoEvent *model.Event
}

// Handler translates a single workloadmeta event into zero or more LifecycleEvents.
type Handler interface {
	// String returns a human-readable name for the handler.
	String() string
	// CanHandle reports whether this handler processes the given event.
	CanHandle(workloadmeta.Event) bool
	// Handle builds zero or more LifecycleEvents for the given event.
	Handle(workloadmeta.Event) ([]LifecycleEvent, error)
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
