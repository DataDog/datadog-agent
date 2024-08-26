// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChangeChecker(t *testing.T) {
	checker := NewChangeChecker()
	r := require.New(t)
	r.False(checker.HasChanged())

	assertConfigChangeDetected(r, checker, "api_key", "API_KEY")
	assertConfigChangeDetected(r, checker, "tags", []string{"tag1", "tag2"})

	m := make(map[string]string)
	m["test"] = "test"
	assertConfigChangeDetected(r, checker, "kubernetes_node_labels_as_tags", m)
}

func assertConfigChangeDetected(r *require.Assertions, checker *ChangeChecker, key string, value interface{}) {
	Datadog().SetWithoutSource(key, value)
	r.True(checker.HasChanged())
	Datadog().SetWithoutSource(key, nil)
	r.False(checker.HasChanged())
}
