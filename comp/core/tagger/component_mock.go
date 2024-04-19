// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package tagger

// Mock implements mock-specific methods for the tagger component.
type Mock interface {
	Component

	// SetTags allows to set tags in the mock fake tagger
	SetTags(entity, source string, low, orch, high, std []string)

	// ResetTagger for testing only
	ResetTagger()
}
