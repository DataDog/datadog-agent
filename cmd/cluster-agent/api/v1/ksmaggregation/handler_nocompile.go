// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubeapiserver

// Package ksmaggregation is a no-op stub on builds without kubeapiserver support;
// the real KSM node-aggregate endpoint is only compiled with the kubeapiserver tag.
package ksmaggregation

import (
	"context"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

// InstallKSMAggregationEndpoints is a no-op on builds without kubeapiserver support.
func InstallKSMAggregationEndpoints(_ context.Context, _ *http.ServeMux, _ config.Component) {}
