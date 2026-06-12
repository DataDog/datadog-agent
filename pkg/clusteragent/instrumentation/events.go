// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package instrumentation

import (
	"fmt"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"
)

func classifySectionEvent(handler Handler, oldObj, newObj *datadoghq.DatadogInstrumentation) (EventType, bool) {
	oldHasSection := oldObj != nil && handler.HasSection(oldObj)
	newHasSection := newObj != nil && handler.HasSection(newObj)

	switch {
	case !oldHasSection && newHasSection:
		return EventCreate, true
	case oldHasSection && newHasSection:
		return EventUpdate, true
	case oldHasSection && !newHasSection:
		return EventDelete, true
	default:
		return "", false
	}
}

// DatadogInstrumentationFromObject converts a runtime object (typed, unstructured, or tombstone)
// into a DatadogInstrumentation. Returns a deep copy.
func DatadogInstrumentationFromObject(obj interface{}) (*datadoghq.DatadogInstrumentation, error) {
	if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tombstone.Obj
	}

	if cr, ok := obj.(*datadoghq.DatadogInstrumentation); ok {
		return cr.DeepCopy(), nil
	}

	if _, ok := obj.(*unstructured.Unstructured); !ok {
		return nil, fmt.Errorf("unexpected DatadogInstrumentation object type %T", obj)
	}

	cr := &datadoghq.DatadogInstrumentation{}
	if err := UnstructuredIntoDatadogInstrumentation(obj, cr); err != nil {
		return nil, err
	}
	return cr, nil
}
