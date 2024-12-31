// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

// Package privileged implements language detection that relies on elevated permissions.
//
// An example of privileged language detection would be binary analysis, where the binary must be
// inspected to determine the language it was compiled from.
package privileged

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
)

// MockPrivilegedDetectors is used in tests to inject mock tests. It should be called before `DetectWithPrivileges`
func MockPrivilegedDetectors(t *testing.T, newDetectors []languagemodels.Detector) {
	oldDetectors := detectorsWithPrivilege
	t.Cleanup(func() { detectorsWithPrivilege = oldDetectors })
	detectorsWithPrivilege = newDetectors
}
