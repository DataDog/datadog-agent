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

type objectInfoType struct {
	GroupVersionKind schema.GroupVersionKind
	Name             string
	Namespace        string
}

type deprecationInfoType struct {
	isDeprecated           bool
	deprecationVersion     semVersionType
	removalVersion         semVersionType
	recommendedReplacement schema.GroupVersionKind
	infoMessage            string
}

type semVersionType struct {
	Major int
	Minor int
	Patch int
}

// apiLifecycleDeprecated is the interface that exposes the lifecycle depraction information of a runtime.Object
type apiLifecycleDeprecated interface {
	APILifecycleDeprecated() (major, minor int)
}

// apiLifecycleRemoved is the interface that exposes the lifecycle removal information of a runtime.Object
type apiLifecycleRemoved interface {
	APILifecycleRemoved() (major, minor int)
}

// apiLifecycleReplacement is the interface that exposes the lifecycle replacement information of a runtime.Object
type apiLifecycleReplacement interface {
	APILifecycleReplacement() schema.GroupVersionKind
}
