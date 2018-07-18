// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

// OpenShiftApiLevel describes what level of OpenShift APIs are available on the apiserver
type OpenShiftApiLevel string

const (
	OpenShiftAPIGroup OpenShiftApiLevel = "OpenShift new API is available"
	OpenShiftOApi                       = "OpenShift legacy oapi is available"
	NotOpenShift                        = "OpenShift not detected"
	OpenShiftUnknown                    = "OpenShift status unknown"
)
