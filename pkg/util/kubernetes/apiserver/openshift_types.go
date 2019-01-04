// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package apiserver

// OpenShiftAPILevel describes what level of OpenShift APIs are available on the apiserver
type OpenShiftAPILevel string

// Responses for DetectOpenShiftAPILevel()
const (
	OpenShiftAPIGroup OpenShiftAPILevel = "new apiGroups"
	OpenShiftOAPI                       = "legacy OAPI"
	NotOpenShift                        = "no API"
)
