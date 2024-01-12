// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

type FlareControllerTestSuite struct {
	suite.Suite

	// fc *FlareController
}

func getTestFlareController() *FlareController {
	return NewFlareController()
}

func TestFillFlare(t *testing.T) {
	f := helpers.NewFlareBuilderMock(t, false)
	fc := getTestFlareController()
	fc.SetAllFiles([]string{"file1"})

	fc.FillFlare(f.Fb)
	f.AssertFileExists("logs_file_permissions.log")
	f.AssertFileContent("file1", "logs_file_permissions.log")
}

func TestAllFiles(t *testing.T) {
	fc := getTestFlareController()

	fc.SetAllFiles([]string{"file1", "file2", "file3"})
	assert.Equal(t, fc.allFiles, []string{"file1", "file2", "file3"})

	fc.SetAllFiles([]string{})
	assert.Equal(t, fc.allFiles, []string{})
}
