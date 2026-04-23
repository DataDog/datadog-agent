// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"log/slog"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func BenchmarkSlogParallel(b *testing.B) {
	b.StopTimer()

	logger, levelVar := initLogger(b)
	levelVar.Set(slog.LevelDebug)
	log.SetupLoggerWithLevelVar(logger, levelVar)

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

func BenchmarkSlogLogger(b *testing.B) {
	b.StopTimer()

	logger, levelVar := initLogger(b)
	levelVar.Set(slog.LevelDebug)
	log.SetupLoggerWithLevelVar(logger, levelVar)

	runLog(b)
}

func initLogger(b *testing.B) (log.LoggerInterface, *slog.LevelVar) {
	b.Helper()
	dir := b.TempDir()

	ddCfg := pkgconfigsetup.Datadog()
	logger, levelVar, err := buildSlogLogger(
		log.DebugLvl,
		false,
		filepath.Join(dir, "test.log"), 1000, 2,
		"",
		commonFormatter("TEST", ddCfg), nil,
	)
	require.NoError(b, err)
	return logger, levelVar
}

func runLog(b *testing.B) {
	b.StartTimer()
	for range b.N {
		log.Info("Hello I am a log")
	}
	log.Flush()
}
