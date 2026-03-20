// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGenerateDPAName(t *testing.T) {
	name := generateDPAName(NamespacedObjectReference{GroupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"}, Namespace: "prod", Name: "web-app"})
	assert.Equal(t, "web-app-9526aeb3", name)

	// Deterministic
	name2 := generateDPAName(NamespacedObjectReference{GroupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"}, Namespace: "prod", Name: "web-app"})
	assert.Equal(t, name, name2)

	// Different kind produces different name
	nameSTF := generateDPAName(NamespacedObjectReference{GroupKind: schema.GroupKind{Group: "apps", Kind: "StatefulSet"}, Namespace: "prod", Name: "web-app"})
	assert.Equal(t, "web-app-c3b1042a", nameSTF)
}
