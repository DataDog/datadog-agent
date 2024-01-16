// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

func TestFillFlare(t *testing.T) {
	file, err := os.Create("test.log")
	assert.Nil(t, err)
	fi, err := os.Stat(file.Name())
	assert.Nil(t, err)

	f := helpers.NewFlareBuilderMock(t, false)
	fc := NewFlareController()

	fc.SetAllFiles([]string{file.Name()})

	fc.FillFlare(f.Fb)
	f.AssertFileExists("logs_file_permissions.log")
	f.AssertFileContent(file.Name()+" "+fi.Mode().String(), "logs_file_permissions.log")
}

func TestAllFiles(t *testing.T) {
	fc := NewFlareController()

	fc.SetAllFiles([]string{"file1", "file2", "file3"})
	assert.Equal(t, fc.allFiles, []string{"file1", "file2", "file3"})

	fc.SetAllFiles([]string{})
	assert.Equal(t, fc.allFiles, []string{})
}
