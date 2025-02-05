// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package auditorimpl

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditor "github.com/DataDog/datadog-agent/comp/logs/auditor/def"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

var testpath = "testpath"

type AuditorTestSuite struct {
	suite.Suite
	testRunPathDir   string
	testRegistryPath string

	a      auditor.Component
	source *sources.LogSource
}

func (suite *AuditorTestSuite) SetupTest() {
	suite.testRunPathDir = suite.T().TempDir()

	suite.testRegistryPath = filepath.Join(suite.testRunPathDir, "registry.json")

	configComponent := configmock.NewMock(suite.T())
	logComponent := logmock.New(suite.T())

	configComponent.SetWithoutSource("logs_config.run_path", suite.testRunPathDir)

	deps := Dependencies{
		Config: configComponent,
		Log:    logComponent,
	}

	suite.a = NewAuditor(deps).Comp
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
	auditorObj := suite.a.(*registryAuditor)
	auditorObj.registry = make(map[string]*RegistryEntry)
	suite.Equal(0, len(auditorObj.registry))
	auditorObj.updateRegistry(suite.source.Config.Path, "42", "end", 0)
	suite.Equal(1, len(auditorObj.registry))
	suite.Equal("42", auditorObj.registry[suite.source.Config.Path].Offset)
	suite.Equal("end", auditorObj.registry[suite.source.Config.Path].TailingMode)
	auditorObj.updateRegistry(suite.source.Config.Path, "43", "beginning", 1)
	suite.Equal(1, len(auditorObj.registry))
	suite.Equal("43", auditorObj.registry[suite.source.Config.Path].Offset)
	suite.Equal("beginning", auditorObj.registry[suite.source.Config.Path].TailingMode)
}

func (suite *AuditorTestSuite) TestAuditorFlushesAndRecoversRegistry() {
	auditorObj := suite.a.(*registryAuditor)
	auditorObj.registry = make(map[string]*RegistryEntry)
	auditorObj.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "42",
		TailingMode: "end",
	}
	suite.NoError(auditorObj.flushRegistry())
	r, err := os.ReadFile(suite.testRegistryPath)
	suite.NoError(err)
	suite.Equal("{\"Version\":2,\"Registry\":{\"testpath\":{\"LastUpdated\":\"2006-01-12T01:01:01.000000001Z\",\"Offset\":\"42\",\"TailingMode\":\"end\",\"IngestionTimestamp\":0}}}", string(r))

	auditorObj.registry = make(map[string]*RegistryEntry)
	auditorObj.registry = auditorObj.recoverRegistry()
	suite.Equal("42", auditorObj.registry[suite.source.Config.Path].Offset)
}

func (suite *AuditorTestSuite) TestAuditorRecoversRegistryForOffset() {
	auditorObj := suite.a.(*registryAuditor)
	auditorObj.registry = make(map[string]*RegistryEntry)
	auditorObj.registry[suite.source.Config.Path] = &RegistryEntry{
		Offset: "42",
	}

	offset := auditorObj.GetOffset(suite.source.Config.Path)
	suite.Equal("42", offset)

	othersource := sources.NewLogSource("", &config.LogsConfig{Path: "anotherpath"})
	offset = auditorObj.GetOffset(othersource.Config.Path)
	suite.Equal("", offset)
}

func (suite *AuditorTestSuite) TestAuditorCleansupRegistry() {
	auditorObj := suite.a.(*registryAuditor)
	auditorObj.registry = make(map[string]*RegistryEntry)
	auditorObj.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "42",
	}

	otherpath := "otherpath"
	auditorObj.registry[otherpath] = &RegistryEntry{
		LastUpdated: time.Now().UTC(),
		Offset:      "43",
	}
	auditorObj.flushRegistry()
	suite.Equal(2, len(auditorObj.registry))

	auditorObj.cleanupRegistry()
	suite.Equal(1, len(auditorObj.registry))
	suite.Equal("43", auditorObj.registry[otherpath].Offset)
}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(AuditorTestSuite))
}
