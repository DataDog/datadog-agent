// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
)

func TestSince(t *testing.T) {
	now := time.Now()
	registry := mock.NewRegistry()

	var since time.Time
	var err error

	since, err = Since(registry, "", service.Before)
	assert.Nil(t, err)
	assert.True(t, since.Equal(now) || since.After(now))

	since, err = Since(registry, "", service.After)
	assert.Nil(t, err)
	assert.Equal(t, time.Time{}, since)

	registry.SetOffset("2008-01-12T01:01:01.000000001Z")
	since, err = Since(registry, "", service.Before)
	assert.Nil(t, err)
	assert.Equal(t, "2008-01-12T01:01:01.000000001Z", since.Format(config.DateFormat))

	// Not properly formated
	registry.SetOffset("2008-01-12T01:01.000000001Z")
	since, err = Since(registry, "", service.Before)
	assert.NotNil(t, err)
	assert.True(t, since.After(now))

	registry.SetOffset("foo")
	since, err = Since(registry, "", service.Before)
	assert.NotNil(t, err)
	assert.True(t, since.After(now))
}
