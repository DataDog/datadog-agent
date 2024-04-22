// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"os"
	"testing"
	"time"

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
	f.AssertNoFileExists("sds.log")
	f.AssertFileContent(file.Name()+" "+fi.Mode().String(), "logs_file_permissions.log")

    // one enabled rule, the file should exist
	fc = NewFlareController()
	f = helpers.NewFlareBuilderMock(t, false)
	fc.SetSDSInfo(nil, []string{"rule1"}, time.Now())
	fc.FillFlare(f.Fb)
	f.AssertFileExists("sds.log")

    // empty list even if the list exists, should not create the file
	fc = NewFlareController()
	f = helpers.NewFlareBuilderMock(t, false)
	fc.SetSDSInfo(nil, nil, time.Now())
	fc.FillFlare(f.Fb)
	f.AssertNoFileExists("sds.log")

    // one standard rule, the file should exist
	fc = NewFlareController()
	f = helpers.NewFlareBuilderMock(t, false)
	fc.SetSDSInfo([]string{"rule1"}, nil, time.Now())
	fc.FillFlare(f.Fb)
	f.AssertFileExists("sds.log")
    // one standard rule and one enabled rule, the file should exist
	fc.SetSDSInfo([]string{"rule1"}, []string{"user_rule1"}, time.Now())
	fc.FillFlare(f.Fb)
	f.AssertFileExists("sds.log")
}

func TestAllFiles(t *testing.T) {
	fc := NewFlareController()

	fc.SetAllFiles([]string{"file1", "file2", "file3"})
	assert.Equal(t, fc.allFiles, []string{"file1", "file2", "file3"})

	fc.SetAllFiles([]string{})
	assert.Equal(t, fc.allFiles, []string{})
}

func TestNonexistantFile(t *testing.T) {
	fc := NewFlareController()
	f := helpers.NewFlareBuilderMock(t, false)
	name := "file.log"

	fc.SetAllFiles([]string{name})
	fc.FillFlare(f.Fb)

	fi, err := os.Stat(name)
	if fi != nil {
		t.FailNow()
	}

	f.AssertFileExists("logs_file_permissions.log")
	f.AssertFileContent(err.Error(), "logs_file_permissions.log")
}
