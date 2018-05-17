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
	var tailer *Tailer
	var source *config.LogSource

	// expect default identifier
	source = config.NewLogSource("", &config.LogsConfig{})
	tailer = NewTailer(source, nil)
	assert.Equal(t, "journald:default", tailer.Identifier())

	// expect identifier to be overidden
	source = config.NewLogSource("", &config.LogsConfig{Path: "any_path"})
	tailer = NewTailer(source, nil)
	assert.Equal(t, "journald:any_path", tailer.Identifier())
}
