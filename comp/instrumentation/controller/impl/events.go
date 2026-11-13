// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package controllerimpl

import (
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation"
)

func classifySectionEvent(handler instrumentation.Handler, oldObj, newObj *datadoghq.DatadogInstrumentation) (instrumentation.EventType, bool) {
	oldHasSection := oldObj != nil && handler.HasSection(oldObj)
	newHasSection := newObj != nil && handler.HasSection(newObj)

	switch {
	case !oldHasSection && newHasSection:
		return instrumentation.EventCreate, true
	case oldHasSection && newHasSection:
		return instrumentation.EventUpdate, true
	case oldHasSection && !newHasSection:
		return instrumentation.EventDelete, true
	default:
		return "", false
	}
}
