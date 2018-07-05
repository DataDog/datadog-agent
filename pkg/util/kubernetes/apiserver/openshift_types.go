// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

// OpenShiftApiLevel describes what level of OpenShift APIs are available on the apiserver
type OpenShiftApiLevel int

const (
	// OpenShiftAPIGroup indicates the new APIGroups are available (3.6+)
	OpenShiftAPIGroup OpenShiftApiLevel = iota
	// OpenShiftOApi  indicates the legacy oapi endpoints are available
	OpenShiftOApi
	// NotOpenShift indicates neither OShift api is available
	NotOpenShift
	// OpenShiftUnknown is set when detection is not yet done
	OpenShiftUnknown
)
