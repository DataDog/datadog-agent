// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build cpython

// NOTICE: See TestMain function in `utils_test.go` for Python initialization
package py

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

type ContainerFilterSuite struct {
	suite.Suite
}

func (s *ContainerFilterSuite) TearDownTest() {
	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{})
	config.Datadog.SetDefault("ac_exclude", []string{})
	containers.ResetSharedFilter()
	initContainerFilter()
}

func (s *ContainerFilterSuite) TestCheckRun() {
	config.Datadog.SetDefault("exclude_pause_container", true)
	config.Datadog.SetDefault("ac_include", []string{"image:apache.*"})
	config.Datadog.SetDefault("ac_exclude", []string{"name:dd-.*"})
	containers.ResetSharedFilter()
	initContainerFilter()

	check, _ := getCheckInstance("testcontainers", "TestCheck")
	err := check.Run()
	assert.NoError(s.T(), err)

	warnings := check.GetWarnings()
	require.Len(s.T(), warnings, 0)
}

func TestContainerFilterSuite(t *testing.T) {
	suite.Run(t, new(ContainerFilterSuite))
}
