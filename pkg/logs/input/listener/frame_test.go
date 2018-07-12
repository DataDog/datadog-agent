// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package listener

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestMaxFrameSize(t *testing.T) {
	assert.Equal(t, defaultMaxFrameSize, getMaxFrameSize(config.NewLogSource("", &config.LogsConfig{})))
	assert.Equal(t, 65535, getMaxFrameSize(config.NewLogSource("", &config.LogsConfig{FrameSize: 65535})))
}

func TestGetContent(t *testing.T) {
	assert.Equal(t, strings.Repeat("a", 10)+"\n", string(getContent([]byte(strings.Repeat("a", 10)+"\n"), 15)))
	assert.Equal(t, strings.Repeat("a", 14)+"\n", string(getContent([]byte(strings.Repeat("a", 14)+"\n"), 15)))
	assert.Equal(t, strings.Repeat("a", 14)+"\n", string(getContent([]byte(strings.Repeat("a", 15)+"\n"), 15)))
	assert.Equal(t, strings.Repeat("a", 14)+"\n", string(getContent([]byte(strings.Repeat("a", 20)+"\n"), 15)))
}
