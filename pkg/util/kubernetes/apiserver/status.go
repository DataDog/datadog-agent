// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package apiserver

import (
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/autoscalers"
)

var dogCl autoscalers.DatadogClient

// GetStatus returns the status of the autoscalers
func GetStatus() map[string]interface{} {
	return autoscalers.GetStatus(dogCl)
}
