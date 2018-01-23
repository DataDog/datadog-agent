// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package auditor

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/stretchr/testify/suite"
)

var testpath = "testpath"

type AuditorTestSuite struct {
	suite.Suite
	testDir  string
	testPath string
	testFile *os.File

	inputChan chan message.Message
	a         *Auditor
	source    *config.LogSource
}

func (suite *AuditorTestSuite) SetupTest() {
	suite.testDir = "tests/"
	os.Remove(suite.testDir)
	os.MkdirAll(suite.testDir, os.ModeDir)
	suite.testPath = fmt.Sprintf("%s/auditor.json", suite.testDir)

	_, err := os.Create(suite.testPath)
	suite.Nil(err)

	suite.inputChan = make(chan message.Message)
	suite.a = New(suite.inputChan)
	suite.a.registryPath = suite.testPath
	suite.source = config.NewLogSource("", &config.LogsConfig{Path: testpath})
}

func (suite *AuditorTestSuite) TearDownTest() {
	os.Remove(suite.testDir)
}

func (suite *AuditorTestSuite) TestAuditorUpdatesRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.Equal(0, len(suite.a.registry))
	suite.a.updateRegistry(suite.source.Config.Path, 42, "")
	suite.Equal(1, len(suite.a.registry))
	suite.Equal(int64(42), suite.a.registry[suite.source.Config.Path].Offset)
	suite.Equal("", suite.a.registry[suite.source.Config.Path].Timestamp)
	suite.a.updateRegistry(suite.source.Config.Path, 43, "")
	suite.Equal(int64(43), suite.a.registry[suite.source.Config.Path].Offset)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000000")
	suite.a.updateRegistry("containerid", 0, ts)
	suite.Equal(ts, suite.a.registry["containerid"].Timestamp)
}

func (suite *AuditorTestSuite) TestAuditorFlushesAndRecoversRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      42,
	}
	suite.a.flushRegistry(suite.a.registry, suite.testPath)
	r, err := ioutil.ReadFile(suite.testPath)
	suite.Nil(err)
	suite.Equal("{\"Version\":1,\"Registry\":{\"testpath\":{\"Timestamp\":\"\",\"Offset\":42,\"LastUpdated\":\"2006-01-12T01:01:01.000000001Z\"}}}", string(r))

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry = suite.a.recoverRegistry(suite.testPath)
	suite.Equal(int64(42), suite.a.registry[suite.source.Config.Path].Offset)
}

func (suite *AuditorTestSuite) TestAuditorRecoversRegistryForOffset() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		Offset: 42,
	}

	offset, whence := suite.a.GetLastCommittedOffset(suite.source.Config.Path)
	suite.Equal(int64(42), offset)
	suite.Equal(os.SEEK_CUR, whence)

	othersource := config.NewLogSource("", &config.LogsConfig{Path: "anotherpath"})
	offset, whence = suite.a.GetLastCommittedOffset(othersource.Config.Path)
	suite.Equal(int64(0), offset)
	suite.Equal(os.SEEK_END, whence)
}

func (suite *AuditorTestSuite) TestAuditorRecoversRegistryForTimestamp() {
	ts := time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC).Format("2006-01-02T15:04:05.000000")

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{Timestamp: ts}
	suite.Equal(ts, suite.a.GetLastCommittedTimestamp(suite.source.Config.Path))

	othersource := config.NewLogSource("", &config.LogsConfig{Path: "anotherpath"})
	suite.Equal("", suite.a.GetLastCommittedTimestamp(othersource.Config.Path))
}

func (suite *AuditorTestSuite) TestAuditorCleansupRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      42,
	}

	otherpath := "otherpath"
	suite.a.registry[otherpath] = &RegistryEntry{
		LastUpdated: time.Now().UTC(),
		Offset:      43,
	}
	suite.a.flushRegistry(suite.a.registry, suite.testPath)
	suite.Equal(2, len(suite.a.registry))

	suite.a.cleanupRegistry(suite.a.registry)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal(int64(43), suite.a.registry[otherpath].Offset)
}

func (suite *AuditorTestSuite) TestAuditorUnmarshalRegistryV0() {
	input := `{
	    "Registry": {
	        "path1.log": {
	            "Offset": 1,
	            "Path": "path1.log",
	            "Timestamp": "2006-01-12T01:01:01.000000001Z"
	        },
	        "path2.log": {
	            "Offset": 2,
	            "Path": "path2.log",
	            "Timestamp": "2006-01-12T01:01:02.000000001Z"
	        }
	    },
	    "Version": 0
	}`
	r, err := suite.a.unmarshalRegistry([]byte(input))
	suite.Nil(err)
	suite.Equal(r["file:path1.log"].Offset, int64(1))
	suite.Equal(r["file:path1.log"].LastUpdated.Second(), 1)
	suite.Equal(r["file:path2.log"].Offset, int64(2))
	suite.Equal(r["file:path2.log"].LastUpdated.Second(), 2)
}

func (suite *AuditorTestSuite) TestAuditorUnmarshalRegistryV1() {
	input := `{
	    "Registry": {
	        "path1.log": {
	            "Offset": 1,
	            "LastUpdated": "2006-01-12T01:01:01.000000001Z",
	            "Timestamp": ""
	        },
	        "path2.log": {
	            "Offset": 0,
	            "LastUpdated": "2006-01-12T01:01:02.000000001Z",
	            "Timestamp": "2006-01-12T01:01:03.000000001Z"
	        }
	    },
	    "Version": 1
	}`
	r, err := suite.a.unmarshalRegistry([]byte(input))
	suite.Nil(err)
	suite.Equal(r["path1.log"].Offset, int64(1))
	suite.Equal(r["path1.log"].LastUpdated.Second(), 1)
	suite.Equal(r["path1.log"].Timestamp, "")

	suite.Equal(r["path2.log"].Offset, int64(0))
	suite.Equal(r["path2.log"].LastUpdated.Second(), 2)
	suite.Equal(r["path2.log"].Timestamp, "2006-01-12T01:01:03.000000001Z")
}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(AuditorTestSuite))
}
