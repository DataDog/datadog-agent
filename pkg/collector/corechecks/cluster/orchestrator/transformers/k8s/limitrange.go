// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	model "github.com/DataDog/agent-payload/v5/process"
)

// ExtractLimitRange returns the protobuf model corresponding to a Kubernetes LimitRange resource.
func ExtractLimitRange(lr *corev1.LimitRange) *model.LimitRange {
	msg := &model.LimitRange{
		Metadata: extractMetadata(&lr.ObjectMeta),
		Spec:     &model.LimitRangeSpec{},
	}

	for _, item := range lr.Spec.Limits {
		limit := &model.LimitRangeItem{
			Default:              convertResourceListToMap(item.Default, convertResourceFn),
			DefaultRequest:       convertResourceListToMap(item.DefaultRequest, convertResourceFn),
			Max:                  convertResourceListToMap(item.Max, convertResourceFn),
			MaxLimitRequestRatio: convertResourceListToMap(item.MaxLimitRequestRatio, convertRatioFn),
			Min:                  convertResourceListToMap(item.Min, convertResourceFn),
			Type:                 string(item.Type),
		}
		msg.Spec.Limits = append(msg.Spec.Limits, limit)
	}

	return msg
}

type convertResourceQuantityFn func(name corev1.ResourceName, quantity resource.Quantity) int64

//nolint:revive // TODO(CAPP) Fix revive linter
func convertRatioFn(name corev1.ResourceName, quantity resource.Quantity) int64 {
	return quantity.Value()
}

func convertResourceFn(name corev1.ResourceName, quantity resource.Quantity) int64 {
	if name == corev1.ResourceCPU {
		return quantity.MilliValue()
	}
	return quantity.Value()
}

func convertResourceListToMap(list corev1.ResourceList, convertFn convertResourceQuantityFn) map[string]int64 {
	m := make(map[string]int64)
	for resource, qty := range list {
		m[string(resource)] = convertFn(resource, qty)
	}
	return m
}
