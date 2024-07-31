// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"github.com/mitchellh/mapstructure"
	"k8s.io/kube-state-metrics/v2/pkg/customresourcestate"
)

type customResourceDecoder struct {
	data customresourcestate.Metrics
}

// Decode decodes the custom resource state metrics configuration.
func (d customResourceDecoder) Decode(v interface{}) error {
	return mapstructure.Decode(d.data, v)
}
