// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package imageresolver provides configuration and utilities for resolving
// container image references from mutable tags to digests.
package imageresolver

func newDatadoghqRegistries(datadogRegistriesList []string) map[string]struct{} {
	datadoghqRegistries := make(map[string]struct{})
	for _, registry := range datadogRegistriesList {
		datadoghqRegistries[registry] = struct{}{}
	}
	return datadoghqRegistries
}
