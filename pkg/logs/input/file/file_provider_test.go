// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package file

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

type ProviderTestSuite struct {
	suite.Suite
	testDir    string
	filesLimit int
}

// newLogSources returns a new log source initialized with the right path.
func (suite *ProviderTestSuite) newLogSources(path string) []*config.LogSource {
	return []*config.LogSource{config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})}
}

func (suite *ProviderTestSuite) SetupTest() {
	suite.filesLimit = 3

	// Create temporary directory
	var err error
	suite.testDir, err = ioutil.TempDir("", "log-file_provider-test-")
	suite.Nil(err)

	// Create directory tree:
	path := fmt.Sprintf("%s/1", suite.testDir)
	err = os.Mkdir(path, os.ModePerm)
	suite.Nil(err)

	path = fmt.Sprintf("%s/1/1.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/1/2.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/1/3.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/2", suite.testDir)
	err = os.Mkdir(path, os.ModePerm)
	suite.Nil(err)

	path = fmt.Sprintf("%s/2/1.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)

	path = fmt.Sprintf("%s/2/2.log", suite.testDir)
	_, err = os.Create(path)
	suite.Nil(err)
}

func (suite *ProviderTestSuite) TearDownTest() {
	os.Remove(suite.testDir)
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsSpecificFile() {
	path := fmt.Sprintf("%s/1/1.log", suite.testDir)
	fileProvider := NewProvider(suite.filesLimit)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(logSources)

	suite.Equal(1, len(files))
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[0].Path)
	suite.Equal("", logSources[0].Overview)
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsAllFilesFromDirectory() {
	path := fmt.Sprintf("%s/1/*.log", suite.testDir)
	fileProvider := NewProvider(suite.filesLimit)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(logSources)

	suite.Equal(3, len(files))
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/2.log", suite.testDir), files[1].Path)
	suite.Equal(fmt.Sprintf("%s/1/3.log", suite.testDir), files[2].Path)
	suite.Equal("3 files tailed out of 3 files matching", logSources[0].Overview)
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsAllFilesFromAnyDirectoryWithRightPermissions() {
	path := fmt.Sprintf("%s/*/*1.log", suite.testDir)
	fileProvider := NewProvider(suite.filesLimit)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(logSources)

	suite.Equal(2, len(files))
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/2/1.log", suite.testDir), files[1].Path)
	suite.Equal("2 files tailed out of 2 files matching", logSources[0].Overview)
}

func (suite *ProviderTestSuite) TestFilesToTailReturnsSpecificFileWithWildcard() {
	path := fmt.Sprintf("%s/1/?.log", suite.testDir)
	fileProvider := NewProvider(suite.filesLimit)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(logSources)

	suite.Equal(3, len(files))
	suite.Equal(fmt.Sprintf("%s/1/1.log", suite.testDir), files[0].Path)
	suite.Equal(fmt.Sprintf("%s/1/2.log", suite.testDir), files[1].Path)
	suite.Equal(fmt.Sprintf("%s/1/3.log", suite.testDir), files[2].Path)
	suite.Equal("3 files tailed out of 3 files matching", logSources[0].Overview)
}

func (suite *ProviderTestSuite) TestNumberOfFilesToTailDoesNotExceedLimit() {
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	fileProvider := NewProvider(suite.filesLimit)
	logSources := suite.newLogSources(path)
	files := fileProvider.FilesToTail(logSources)
	suite.Equal(suite.filesLimit, len(files))
	suite.Equal("3 files tailed out of 5 files matching", logSources[0].Overview)
}

func (suite *ProviderTestSuite) TestAllWildcardPathsAreUpdated() {
	filesLimit := 2
	fileProvider := NewProvider(filesLimit)
	logSources := []*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/1/*.log", suite.testDir)}),
		config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: fmt.Sprintf("%s/2/*.log", suite.testDir)}),
	}
	files := fileProvider.FilesToTail(logSources)
	suite.Equal(2, len(files))
	suite.Equal("2 files tailed out of 3 files matching", logSources[0].Overview)
	suite.Equal("0 files tailed out of 2 files matching", logSources[1].Overview)
}

func TestProviderTestSuite(t *testing.T) {
	suite.Run(t, new(ProviderTestSuite))
}
