// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package journald

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestIdentifier(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{Type: config.JournaldType})

	var tailer *Tailer
	var config JournalConfig

	// expect default identifier
	config = JournalConfig{
		Path: "",
	}
	tailer = NewTailer(config, source, nil, nil)
	assert.Equal(t, "journald:default", tailer.Identifier())

	// expect identifier to be overidden
	config = JournalConfig{
		Path: "any_path",
	}
	tailer = NewTailer(config, source, nil, nil)
	assert.Equal(t, "journald:any_path", tailer.Identifier())
}
