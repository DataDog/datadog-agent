// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build !windows

package tailer

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

type FileProviderTestSuite struct {
	suite.Suite
	testDir    string
	filesLimit int
}

// newFileProvider returns a new FileProvider initialized with the right configuration
func (suite *FileProviderTestSuite) newFileProvider(path string) *FileProvider {
	sources := []*config.LogSource{config.NewLogSource("", &config.LogsConfig{Type: config.FileType, Path: path})}
	return NewFileProvider(sources, suite.filesLimit)
}

func (suite *FileProviderTestSuite) SetupTest() {
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

func (suite *FileProviderTestSuite) TearDownTest() {
	os.Remove(suite.testDir)
}

func (suite *FileProviderTestSuite) TestFilesToTailReturnSpecificFile() {
	path := fmt.Sprintf("%s/1/1.log", suite.testDir)
	fileProvider := suite.newFileProvider(path)
	files := fileProvider.FilesToTail()

	suite.Equal(len(files), 1)
	suite.Equal(files[0].Path, fmt.Sprintf("%s/1/1.log", suite.testDir))
}

func (suite *FileProviderTestSuite) TestFilesToTailReturnAllFilesInDirectory() {
	path := fmt.Sprintf("%s/1/*.log", suite.testDir)
	fileProvider := suite.newFileProvider(path)
	files := fileProvider.FilesToTail()

	suite.Equal(len(files), 2)
	suite.Equal(files[0].Path, fmt.Sprintf("%s/1/1.log", suite.testDir))
	suite.Equal(files[1].Path, fmt.Sprintf("%s/1/2.log", suite.testDir))
}

func (suite *FileProviderTestSuite) TestFilesToTailReturnAllFilesInAnyDirectory() {
	path := fmt.Sprintf("%s/*/*1.log", suite.testDir)
	fileProvider := suite.newFileProvider(path)
	files := fileProvider.FilesToTail()

	suite.Equal(len(files), 2)
	suite.Equal(files[0].Path, fmt.Sprintf("%s/1/1.log", suite.testDir))
	suite.Equal(files[1].Path, fmt.Sprintf("%s/2/1.log", suite.testDir))
}

func (suite *FileProviderTestSuite) TestNumberOfFilesToTailDoesNotExceedLimit() {
	path := fmt.Sprintf("%s/*/*.log", suite.testDir)
	fileProvider := suite.newFileProvider(path)
	files := fileProvider.FilesToTail()

	suite.Equal(len(files), suite.filesLimit)
}

func TestFileProviderTestSuite(t *testing.T) {
	suite.Run(t, new(FileProviderTestSuite))
}
