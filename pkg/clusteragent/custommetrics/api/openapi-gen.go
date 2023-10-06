// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build codegen

// This is a stub to ensure that k8s.io/kube-openapi/cmd/openapi-gen is tracked by go.mod
// It is a dependency needed in the build process of the cluster agent to update the
// generated code for the external metrics server's openapi.
package api

import _ "k8s.io/kube-openapi/cmd/openapi-gen"
