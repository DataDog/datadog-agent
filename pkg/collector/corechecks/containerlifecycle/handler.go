// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package containerlifecycle

import workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"

// Handler translates a single workloadmeta event into zero or more LifecycleEvents.
type Handler interface {
	// String returns a human-readable name for the handler.
	String() string
	// CanHandle reports whether this handler processes the given event.
	CanHandle(workloadmeta.Event) bool
	// Handle builds zero or more LifecycleEvents for the given event.
	Handle(workloadmeta.Event) ([]LifecycleEvent, error)
}
