// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/stretchr/testify/require"
)

func TestChangeChecker(t *testing.T) {
	cfg := Datadog()
	checker := NewChangeChecker(cfg)
	r := require.New(t)
	r.False(checker.HasChanged())

	assertConfigChangeDetected(r, cfg, checker, "api_key", "API_KEY")
	assertConfigChangeDetected(r, cfg, checker, "tags", []string{"tag1", "tag2"})

	m := make(map[string]string)
	m["test"] = "test"
	assertConfigChangeDetected(r, cfg, checker, "kubernetes_node_labels_as_tags", m)
}

func assertConfigChangeDetected(r *require.Assertions, cfg model.Config, checker *ChangeChecker, key string, value interface{}) {
	cfg.SetWithoutSource(key, value)
	r.True(checker.HasChanged())
	cfg.UnsetForSource(key, model.SourceUnknown)
	r.False(checker.HasChanged())
}
