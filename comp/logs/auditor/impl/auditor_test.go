// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package auditorimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	kubehealthmock "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/mock"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

var testpath = "testpath"

type AuditorTestSuite struct {
	suite.Suite
	testRunPathDir   string
	testRegistryPath string

	a      *registryAuditor
	source *sources.LogSource
}

func (suite *AuditorTestSuite) SetupTest() {
	suite.testRunPathDir = suite.T().TempDir()

	suite.testRegistryPath = filepath.Join(suite.testRunPathDir, "registry.json")

	configComponent := configmock.NewMock(suite.T())
	logComponent := logmock.New(suite.T())
	kubeHealthRegistrar := kubehealthmock.NewMockRegistrar()
	configComponent.SetInTest("logs_config.run_path", suite.testRunPathDir)

	deps := Dependencies{
		Config:     configComponent,
		Log:        logComponent,
		KubeHealth: kubeHealthRegistrar,
	}

	suite.a = newAuditor(deps)
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

	// Create a fingerprint with nil config for testing
	fingerprint := types.Fingerprint{Value: 0, Config: nil}
	suite.a.updateRegistry(suite.source.Config.Path, "42", "end", 0, fingerprint)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal("42", suite.a.registry[suite.source.Config.Path].Offset)
	suite.Equal("end", suite.a.registry[suite.source.Config.Path].TailingMode)

	fingerprint = types.Fingerprint{Value: 0, Config: nil}
	suite.a.updateRegistry(suite.source.Config.Path, "43", "beginning", 1, fingerprint)
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
		Fingerprint: types.Fingerprint{Value: 0, Config: nil},
	}
	suite.NoError(suite.a.flushRegistry())
	r, err := os.ReadFile(suite.testRegistryPath)
	suite.NoError(err)
	suite.Equal("{\"Version\":2,\"Registry\":{\"testpath\":{\"LastUpdated\":\"2006-01-12T01:01:01.000000001Z\",\"Offset\":\"42\",\"TailingMode\":\"end\",\"IngestionTimestamp\":0,\"Fingerprint\":{\"Value\":0,\"Config\":null}}}}", string(r))

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry = suite.a.recoverRegistry()
	suite.Equal("42", suite.a.registry[suite.source.Config.Path].Offset)
}

func (suite *AuditorTestSuite) TestAuditorUpdatesRegistryWithFingerprint() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.Equal(0, len(suite.a.registry))

	identifier := "file:/var/log/test.log"
	fingerprintConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            1000,
	}
	fingerprint := types.Fingerprint{Value: 12345, Config: fingerprintConfig}

	suite.a.updateRegistry(identifier, "100", "end", 1, fingerprint)
	suite.Equal(1, len(suite.a.registry))
	suite.Equal("100", suite.a.registry[identifier].Offset)
	suite.Equal("end", suite.a.registry[identifier].TailingMode)
	suite.Equal(uint64(12345), suite.a.registry[identifier].Fingerprint.Value)
	suite.Equal(fingerprintConfig, suite.a.registry[identifier].Fingerprint.Config)
}

func (suite *AuditorTestSuite) TestAuditorFlushesAndRecoversRegistryWithFingerprint() {
	identifier := "file:/var/log/test.log"
	fingerprintConfig := &types.FingerprintConfig{
		FingerprintStrategy: types.FingerprintStrategyLineChecksum,
		Count:               1,
		CountToSkip:         0,
		MaxBytes:            1000,
		Source:              types.FingerprintConfigSourcePerSource,
	}
	fingerprint := types.Fingerprint{Value: 12345, Config: fingerprintConfig}

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[identifier] = &RegistryEntry{
		LastUpdated: time.Date(2024, time.July, 18, 1, 1, 1, 1, time.UTC),
		Offset:      "150",
		TailingMode: "end",
		Fingerprint: fingerprint,
	}
	suite.NoError(suite.a.flushRegistry())

	r, err := os.ReadFile(suite.testRegistryPath)
	suite.NoError(err)
	expectedJSON := `{"Version":2,"Registry":{"file:/var/log/test.log":{"LastUpdated":"2024-07-18T01:01:01.000000001Z","Offset":"150","TailingMode":"end","IngestionTimestamp":0,"Fingerprint":{"Value":12345,"Config":{"fingerprint_strategy":"line_checksum","count":1,"count_to_skip":0,"max_bytes":1000,"source":"per-source"}}}}}`
	suite.Equal(expectedJSON, string(r))

	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry = suite.a.recoverRegistry()
	suite.Equal("150", suite.a.registry[identifier].Offset)
	suite.Equal(uint64(12345), suite.a.registry[identifier].Fingerprint.Value)
	suite.Equal(fingerprintConfig.FingerprintStrategy, suite.a.registry[identifier].Fingerprint.Config.FingerprintStrategy)
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

func (suite *AuditorTestSuite) TestAuditorRecoversRegistryForFingerprint() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		Offset:      "42",
		Fingerprint: types.Fingerprint{Value: 12345, Config: nil},
	}

	fingerprint := suite.a.GetFingerprint(suite.source.Config.Path)
	suite.Equal(uint64(12345), fingerprint.Value)

	othersource := sources.NewLogSource("", &config.LogsConfig{Path: "anotherpath"})
	fingerprint = suite.a.GetFingerprint(othersource.Config.Path)
	suite.Nil(fingerprint)
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

	otherpath2 := "otherpath2"
	suite.a.registry[otherpath2] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "44",
	}

	otherpath3 := "otherpath3"
	suite.a.registry[otherpath3] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "45",
	}

	otherpath4 := "otherpath4"
	suite.a.registry[otherpath4] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "46",
	}

	suite.a.SetTailed(otherpath2, true)
	// SetTailed alters the LastUpdated field, so we need to set it back to the original value to test
	// that active tails are never removed regardless of their LastUpdated value
	suite.a.registry[otherpath2].LastUpdated = time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC)
	suite.a.SetTailed(otherpath4, false)

	suite.a.flushRegistry()
	suite.Equal(5, len(suite.a.registry))

	suite.a.cleanupRegistry()
	suite.Equal(3, len(suite.a.registry))
	suite.Equal("43", suite.a.registry[otherpath].Offset)
	suite.Equal("44", suite.a.registry[otherpath2].Offset)
	suite.Equal("46", suite.a.registry[otherpath4].Offset)
}

func (suite *AuditorTestSuite) TestAuditorLiveness() {
	suite.a.registry = make(map[string]*RegistryEntry)
	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "42",
	}

	suite.a.SetTailed(suite.source.Config.Path, false)
	suite.WithinDuration(time.Now().UTC(), suite.a.registry[suite.source.Config.Path].LastUpdated, 1*time.Second)

	suite.a.registry[suite.source.Config.Path] = &RegistryEntry{
		LastUpdated: time.Date(2006, time.January, 12, 1, 1, 1, 1, time.UTC),
		Offset:      "42",
	}

	suite.a.KeepAlive(suite.source.Config.Path)
	suite.WithinDuration(time.Now().UTC(), suite.a.registry[suite.source.Config.Path].LastUpdated, 1*time.Second)
}

func (suite *AuditorTestSuite) TestAuditorRegistryWriterSelection() {
	// Test atomic write enabled
	configComponent := configmock.NewMock(suite.T())
	logComponent := logmock.New(suite.T())
	configComponent.SetInTest("logs_config.run_path", suite.testRunPathDir)
	configComponent.SetInTest("logs_config.atomic_registry_write", true)
	deps := Dependencies{
		Config: configComponent,
		Log:    logComponent,
	}
	auditor := newAuditor(deps)
	suite.Equal("*auditorimpl.atomicRegistryWriter", fmt.Sprintf("%T", auditor.registryWriter))

	// Test atomic write disabled
	configComponent = configmock.NewMock(suite.T())
	logComponent = logmock.New(suite.T())
	configComponent.SetInTest("logs_config.run_path", suite.testRunPathDir)
	configComponent.SetInTest("logs_config.atomic_registry_write", false)
	deps = Dependencies{
		Config: configComponent,
		Log:    logComponent,
	}
	auditor = newAuditor(deps)
	suite.Equal("*auditorimpl.nonAtomicRegistryWriter", fmt.Sprintf("%T", auditor.registryWriter))
}

func TestScannerTestSuite(t *testing.T) {
	suite.Run(t, new(AuditorTestSuite))
}
