// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestConfigChangeChecker(t *testing.T) {
	checker := NewConfigChangeChecker()
	r := require.New(t)
	r.False(checker.HasChanged())

	assertConfigChangeDetected(r, checker, "api_key", "API_KEY")
	assertConfigChangeDetected(r, checker, "tags", []string{"tag1", "tag2"})

	m := make(map[string]string)
	m["test"] = "test"
	assertConfigChangeDetected(r, checker, "kubernetes_node_labels_as_tags", m)
}

func assertConfigChangeDetected(r *require.Assertions, checker *ConfigChangeChecker, key string, value interface{}) {
	config.Datadog.Set(key, value)
	r.True(checker.HasChanged())
	config.Datadog.Set(key, nil)
	r.False(checker.HasChanged())
}
