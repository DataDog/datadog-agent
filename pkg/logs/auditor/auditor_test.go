// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package auditor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/status/health"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

var testpath = "testpath"

type AuditorTestSuite struct {
	suite.Suite
	testRunPathDir   string
	testRegistryPath string

	a      *RegistryAuditor
	source *sources.LogSource
}

func (suite *AuditorTestSuite) SetupTest() {
	suite.testRunPathDir = suite.T().TempDir()

	suite.testRegistryPath = filepath.Join(suite.testRunPathDir, "registry.json")

	suite.a = New(suite.testRunPathDir, DefaultRegistryFilename, time.Hour, health.RegisterLiveness("fake"))
	suite.source = sources.NewLogSource("", &config.LogsConfig{Path: testpath})
}

func (suite *AuditorTestSuite) TestAuditorStartStop() {
	assert.Nil(suite.T(), suite.a.Channel())
	suite.a.Start()
	assert.NotNil(suite.T(), suite.a.Channel())
	suite.a.Stop()
	assert.Nil(suite.T(), suite.a.Channel())
}

func (suite *AuditorTestSuite) TestAuditorUpdatesRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.Equal(0, len(suite.a.registry))
	suite.a.updateRegistry(suite.source.Config.Path, "42", "end", 0)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal("42", suite.a.registry[suite.source.Config.Path].Offset)
	suite.Equal("end", suite.a.registry[suite.source.Config.Path].TailingMode)
	suite.a.updateRegistry(suite.source.Config.Path, "43", "beginning", 1)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal("43", suite.a.registry[suite.source.Config.Path].Offset)
	suite.Equal("beginning", suite.a.registry[suite.source.Config.Path].TailingMode)
}

func (suite *AuditorTestSuite) TestAuditorFlushesAndRecoversRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "42",
		TailingMode: "end",
	}
	suite.NoError(suite.a.flushRegistry())
	r, err := os.ReadFile(suite.testRegistryPath)
	suite.NoError(err)
	suite.Equal("{\"Version\":2,\"Registry\":{\"testpath\":{\"LastUpdated\":\"2006-01-12T01:01:01.000000001Z\",\"Offset\":\"42\",\"TailingMode\":\"end\",\"IngestionTimestamp\":0}}}", string(r))

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry = suite.a.recoverRegistry()
	suite.Equal("42", suite.a.registry[suite.source.Config.Path].Offset)
}

func (suite *AuditorTestSuite) TestAuditorRecoversRegistryForOffset() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		Offset: "42",
	}

	offset := suite.a.GetOffset(suite.source.Config.Path)
	suite.Equal("42", offset)

	othersource := sources.NewLogSource("", &config.LogsConfig{Path: "anotherpath"})
	offset = suite.a.GetOffset(othersource.Config.Path)
	suite.Equal("", offset)
}

func (suite *AuditorTestSuite) TestAuditorCleansupRegistry() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "42",
	}

	otherpath := "otherpath"
	suite.a.registry[otherpath] = &RegistryEntry{
		LastUpdated: time.Now().UTC(),
		Offset:      "43",
	}
	suite.a.flushRegistry()
	suite.Equal(2, len(suite.a.registry))

	suite.a.cleanupRegistry()
	suite.Equal(1, len(suite.a.registry))
	suite.Equal("43", suite.a.registry[otherpath].Offset)
}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(AuditorTestSuite))
}
