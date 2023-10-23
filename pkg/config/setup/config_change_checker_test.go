// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestChangeChecker(t *testing.T) {
	config := SetupConf()
	checker := NewChangeChecker(config)
	r := require.New(t)
	r.False(checker.HasChanged(config))

	assertConfigChangeDetected(r, checker, "api_key", "API_KEY", config)
	assertConfigChangeDetected(r, checker, "tags", []string{"tag1", "tag2"}, config)

	m := make(map[string]string)
	m["test"] = "test"
	assertConfigChangeDetected(r, checker, "kubernetes_node_labels_as_tags", m, config)
}

func assertConfigChangeDetected(r *require.Assertions, checker *ChangeChecker, key string, value interface{}, config pkgconfigmodel.Config) {
	config.Set(key, value)
	r.True(checker.HasChanged(config))
	config.Set(key, nil)
	r.False(checker.HasChanged(config))
}
