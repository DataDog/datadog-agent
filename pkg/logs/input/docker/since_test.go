// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/seek"
	"github.com/DataDog/datadog-agent/pkg/logs/seek/mock"
)

func TestSince(t *testing.T) {
	now := time.Now()
	registry := mock.NewRegistry()
	seeker := seek.NewSeeker(registry)

	var since time.Time
	var err error

	since, err = Since(seeker, types.Container{Created: time.Now().Add(time.Hour).Unix()}, "")
	assert.Nil(t, err)
	assert.Equal(t, time.Time{}, since)

	since, err = Since(seeker, types.Container{Created: time.Now().Add(-time.Hour).Unix()}, "")
	assert.Nil(t, err)
	assert.True(t, since.After(now))

	registry.SetOffset("2008-01-12T01:01:01.000000001Z")
	since, err = Since(seeker, types.Container{}, "")
	assert.Nil(t, err)
	assert.Equal(t, "2008-01-12T01:01:01.000000002Z", since.Format(config.DateFormat))

	registry.SetOffset("foo")
	since, err = Since(seeker, types.Container{}, "")
	assert.NotNil(t, err)
	assert.True(t, since.After(now))
}
