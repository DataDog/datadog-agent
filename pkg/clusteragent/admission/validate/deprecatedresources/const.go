// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package deprecatedresources is an admission controller webhook use to detect the usage of deprecated APIGroup and APIVersions.
package deprecatedresources

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var constResourceDeprecationInfo = map[schema.GroupVersionKind]deprecationInfoType{
	// TODO add resources deprecation info
}
