// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package common contains the HostInstaller struct which is used to write the agent agentConfiguration to disk
package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

func assertFileContent(t *testing.T, file, content string) {
	b, err := os.ReadFile(file)
	assert.NoError(t, err)
	assert.Equal(t, content, string(b))
}

func TestAgentConfigs(t *testing.T) {
	configDir = t.TempDir()
	datadogConfFile = filepath.Join(configDir, "datadog.yaml")
	logsConfFile = filepath.Join(configDir, "conf.d/configured_at_install_logs.yaml")
	sparkConfigFile = filepath.Join(configDir, "conf.d/spark.d/spark.yaml")

	i, err := newHostInstaller(&env.Env{APIKey: "a"}, 0, 0)
	assert.NotNil(t, i)
	assert.Nil(t, err)

	i.AddAgentConfig("key", "value")
	i.AddLogConfig(LogConfig{Type: "file", Path: "/var/log/app.log", Service: "app"})
	i.AddHostTag("k1", "v1")
	i.AddHostTag("k2", "v2")
	i.AddSparkInstance(SparkInstance{ClusterName: "cluster", SparkURL: "http://localhost:8080"})

	assert.NoError(t, i.writeConfigs())
	assertFileContent(t, datadogConfFile, `api_key: a
key: value
logs_enabled: true
tags:
- k1:v1
- k2:v2
`)

	assertFileContent(t, logsConfFile, `logs:
- type: file
  path: /var/log/app.log
  service: app
`)
	assertFileContent(t, sparkConfigFile, `instances:
- spark_url: http://localhost:8080
  cluster_name: cluster
`)
}
