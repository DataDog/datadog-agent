// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	seelogCfg "github.com/DataDog/datadog-agent/pkg/util/log/setup/internal/seelog"
)

func TestSeelogConfig(t *testing.T) {
	cfg := seelogCfg.NewSeelogConfig("TEST", "off", "common", "", "", false, nil, nil)
	cfg.EnableConsoleLog(true)
	cfg.EnableFileLogging("/dev/null", 123, 456)

	seelogConfigStr, err := cfg.Render()
	assert.Nil(t, err)

	logger, err := seelog.LoggerFromConfigAsString(seelogConfigStr)
	assert.Nil(t, err)
	assert.NotNil(t, logger)
}

func BenchmarkSeelogParallel(b *testing.B) {
	b.StopTimer()

	cfg := initConfig(b)
	seelogConfigStr, err := cfg.Render()
	require.NoError(b, err)

	logger, err := seelog.LoggerFromConfigAsString(seelogConfigStr)
	require.NoError(b, err)
	require.NotNil(b, logger)
	log.SetupLogger(logger, "debug")

	runLogParallel(b)
}

func BenchmarkSlogParallel(b *testing.B) {
	b.StopTimer()

	cfg := initConfig(b)
	logger, err := cfg.SlogLogger()
	require.NoError(b, err)
	require.NotNil(b, logger)
	log.SetupLogger(logger, "debug")

	runLogParallel(b)
}

func runLogParallel(b *testing.B) {
	b.StartTimer()
	wg := sync.WaitGroup{}
	wg.Add(b.N)
	for range b.N {
		go func() {
			defer wg.Done()
			for range 1000 {
				log.Info("Hello I am a log")
			}
		}()
	}
	wg.Wait()
	log.Flush()
}

func BenchmarkSeelogLogger(b *testing.B) {
	b.StopTimer()

	cfg := initConfig(b)
	seelogConfigStr, err := cfg.Render()
	require.NoError(b, err)

	logger, err := seelog.LoggerFromConfigAsString(seelogConfigStr)
	require.NoError(b, err)
	require.NotNil(b, logger)
	log.SetupLogger(logger, "debug")

	runLog(b)
}

func BenchmarkSlogLogger(b *testing.B) {
	b.StopTimer()

	cfg := initConfig(b)
	logger, err := cfg.SlogLogger()
	require.NoError(b, err)
	require.NotNil(b, logger)
	log.SetupLogger(logger, "debug")

	runLog(b)
}

func initConfig(b *testing.B) *seelogCfg.Config {
	dir := b.TempDir()

	ddCfg := pkgconfigsetup.Datadog()
	cfg := seelogCfg.NewSeelogConfig("TEST", "debug", "common", "", buildCommonFormat("TEST", ddCfg), false, nil, commonFormatter("TEST", ddCfg))
	cfg.EnableConsoleLog(false)
	cfg.EnableFileLogging(filepath.Join(dir, "test.log"), 1000, 2)

	return cfg
}

func runLog(b *testing.B) {
	b.StartTimer()
	for range b.N {
		log.Info("Hello I am a log")
	}
	log.Flush()
}
