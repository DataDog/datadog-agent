// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package helmactions performs Helm operations on a Kubernetes cluster by
// launching short-lived Jobs that execute the helm CLI inside a helm container
// image. It does not link against helm's Go libraries; the helm binary in the
// Job image is the source of truth for the operation semantics.
package helmactions
