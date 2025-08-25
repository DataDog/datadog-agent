// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procmon

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	analysisFailureLogLimiter.SetLimit(rate.Inf)
	setupLogging()
	os.Exit(m.Run())
}

// setupLogging is used to have a consistent logging setup for all tests.
func setupLogging() {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "debug"
	}
	const defaultFormat = "%l %Date(15:04:05.000000000) @%File:%Line| %Msg%n"
	var format string
	switch formatFromEnv := os.Getenv("DD_LOG_FORMAT"); formatFromEnv {
	case "":
		format = defaultFormat
	case "json":
		format = `{"time":%Ns,"level":"%Level","msg":"%Msg","path":"%RelFile","func":"%Func","line":%Line}%n`
	case "json-short":
		format = `{"t":%Ns,"l":"%Lev","m":"%Msg"}%n`
	default:
		format = formatFromEnv
	}
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(
		os.Stderr, log.TraceLvl, format,
	)
	if err != nil {
		panic(fmt.Errorf("failed to create logger: %w", err))
	}
	log.SetupLogger(logger, logLevel)
}
