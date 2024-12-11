// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !(clusterchecks && kubeapiserver)

package listeners

// NewKubeEndpointsListener returns the kube endpoints implementation of the ServiceListener interface
var NewKubeEndpointsListener ServiceListenerFactory
