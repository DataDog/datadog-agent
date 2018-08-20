// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build systemd

package journald

import (
	"testing"

	"github.com/stretchr/testify/assert"

	auditor "github.com/DataDog/datadog-agent/pkg/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	pipeline "github.com/DataDog/datadog-agent/pkg/logs/pipeline/mock"
)

func TestShouldStartOnlyOneTailerPerJournal(t *testing.T) {
	sources := config.NewLogSources([]*config.LogSource{
		config.NewLogSource("", &config.LogsConfig{Type: config.JournaldType}),
		config.NewLogSource("", &config.LogsConfig{Type: config.JournaldType}),
	})
	launcher := NewLauncher(sources, pipeline.NewMockProvider(), auditor.NewRegistry())

	// expect only one new tailer
	launcher.Start()
	assert.Equal(t, 1, len(launcher.tailers))

	// expect all tailers to be released
	launcher.Stop()
	assert.Equal(t, 0, len(launcher.tailers))
}
