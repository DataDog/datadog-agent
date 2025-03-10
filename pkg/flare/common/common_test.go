// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestIncludeSystemProbeConfig(t *testing.T) {
	configmock.NewFromFile(t, "./test/datadog-agent.yaml")
	// create system-probe.yaml file because it's in .gitignore
	_, err := os.Create("./test/system-probe.yaml")
	require.NoError(t, err, "couldn't create system-probe.yaml")
	defer os.Remove("./test/system-probe.yaml")

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	GetConfigFiles(mock, map[string]string{"": "./test/confd"})

	mock.AssertFileExists("etc", "datadog.yaml")
	mock.AssertFileExists("etc", "system-probe.yaml")
}

func TestIncludeConfigFiles(t *testing.T) {
	configmock.New(t)

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	GetConfigFiles(mock, map[string]string{"": "./test/confd"})

	mock.AssertFileExists("etc/confd/test.yaml")
	mock.AssertFileExists("etc/confd/test.Yml")
	mock.AssertNoFileExists("etc/confd/not_included.conf")
}

func TestIncludeConfigFilesWithPrefix(t *testing.T) {
	configmock.New(t)

	mock := flarehelpers.NewFlareBuilderMock(t, false)
	GetConfigFiles(mock, map[string]string{"prefix": "./test/confd"})

	mock.AssertFileExists("etc/confd/prefix/test.yaml")
	mock.AssertFileExists("etc/confd/prefix/test.Yml")
	mock.AssertNoFileExists("etc/confd/prefix/not_included.conf")
}
