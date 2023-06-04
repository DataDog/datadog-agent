// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package limiter

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

var mockError = errors.New("mock")

func TestConfig(t *testing.T) {
	m := config.Mock(t)

	// no configuration, disabled by default
	l := fromConfig(1, true, func() (uint64, error) { return 0, mockError })
	assert.Nil(t, l)

	// static limit
	m.Set("dogstatsd_context_limiter.limit", 500)
	l = fromConfig(1, true, func() (uint64, error) { return 0, mockError })
	assert.Equal(t, 500, l.global)

	// fallback to static limit with error
	m.Set("dogstatsd_context_limiter.cgroup_memory_ratio", 0.5)
	l = fromConfig(1, true, func() (uint64, error) { return 0, mockError })
	assert.Equal(t, 500, l.global)

	// memory based limit
	m.Set("dogstatsd_context_limiter.bytes_per_context", 1500)
	l = fromConfig(1, true, func() (uint64, error) { return 3_000_000, nil })
	assert.Equal(t, 1000, l.global)

	// non-core agents
	l = fromConfig(1, false, func() (uint64, error) { return 3_000_000, nil })
	assert.Nil(t, l)
}
