// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/stretchr/testify/assert"
)

func TestTailerNextLogSinceDate(t *testing.T) {
	tailer := &Tailer{}
	var date time.Time
	var err error
	date, err = tailer.nextLogSinceDate("2008-01-12T01:01:01.000000000Z")
	assert.Equal(t, "2008-01-12T01:01:01.000000001Z", date.Format(config.DateFormat))
	assert.Nil(t, err)

	_, err = tailer.nextLogSinceDate("2008-01-12T01:01:01.anything")
	assert.NotNil(t, err)

	_, err = tailer.nextLogSinceDate("")
	assert.NotNil(t, err)
}

func TestTailerNextLogDate(t *testing.T) {
	tailer := &Tailer{}
	assert.Equal(t, "2008-01-12T01:01:01.000000001Z", tailer.nextLogDate("2008-01-12T01:01:01.000000000Z", true))
	assert.Equal(t, "0001-01-01T00:00:00.000000000Z", tailer.nextLogDate("", true))
	assert.NotEqual(t, "0001-01-01T00:00:00.000000000Z", tailer.nextLogDate("", false))
}

func TestTailerIdentifier(t *testing.T) {
	tailer := &Tailer{ContainerID: "test"}
	assert.Equal(t, "docker:test", tailer.Identifier())
}
