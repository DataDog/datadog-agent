// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package file

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/seek"
	"github.com/DataDog/datadog-agent/pkg/logs/seek/mock"
)

func TestPosition(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	registry := mock.NewRegistry()
	seeker := seek.NewSeeker(registry)
	end := time.Now().Add(time.Hour)

	var err error
	var offset int64
	var whence int

	offset, whence, err = Position(seeker, start, "")
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)

	offset, whence, err = Position(seeker, end, "")
	assert.Nil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("123456789")
	offset, whence, err = Position(seeker, end, "")
	assert.Nil(t, err)
	assert.Equal(t, int64(123456789), offset)
	assert.Equal(t, io.SeekStart, whence)

	registry.SetOffset("foo")
	offset, whence, err = Position(seeker, end, "")
	assert.NotNil(t, err)
	assert.Equal(t, int64(0), offset)
	assert.Equal(t, io.SeekEnd, whence)
}
