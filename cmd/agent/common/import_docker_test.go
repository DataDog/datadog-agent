// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker
// +build docker

package common

import (
	"testing"
)

func TestImportWithDockerSettings(t *testing.T) {
	integrations := []string{"cassandra", "docker", "kubelet", "mysql"}
	RunImport(t, integrations)
}
