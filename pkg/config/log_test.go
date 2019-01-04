// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019 Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractShortPathFromFullPath(t *testing.T) {
	// omnibus path
	assert.Equal(t, "pkg/collector/scheduler.go", extractShortPathFromFullPath("/go/src/github.com/DataDog/datadog-agent/.omnibus/src/datadog-agent/src/github.com/DataDog/datadog-agent/pkg/collector/scheduler.go"))
	// dev env path
	assert.Equal(t, "cmd/agent/app/start.go", extractShortPathFromFullPath("/home/vagrant/go/src/github.com/DataDog/datadog-agent/cmd/agent/app/start.go"))
	// relative path
	assert.Equal(t, "pkg/collector/scheduler.go", extractShortPathFromFullPath("pkg/collector/scheduler.go"))
	// no path
	assert.Equal(t, "main.go", extractShortPathFromFullPath("main.go"))
}
